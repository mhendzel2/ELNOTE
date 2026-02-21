package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mjhen/elnote/server/internal/admin"
	"github.com/mjhen/elnote/server/internal/attachments"
	"github.com/mjhen/elnote/server/internal/auth"
	"github.com/mjhen/elnote/server/internal/config"
	"github.com/mjhen/elnote/server/internal/experiments"
	"github.com/mjhen/elnote/server/internal/httpx"
	"github.com/mjhen/elnote/server/internal/middleware"
	"github.com/mjhen/elnote/server/internal/ops"
	"github.com/mjhen/elnote/server/internal/syncer"
)

type App struct {
	cfg               config.Config
	db                *sql.DB
	tokens            *auth.TokenManager
	authService       *auth.Service
	expService        *experiments.Service
	adminService      *admin.Service
	syncService       *syncer.Service
	attachmentService *attachments.Service
	opsService        *ops.Service
}

func New(cfg config.Config, db *sql.DB) (*App, error) {
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	syncService := syncer.NewService(db)
	signer, err := attachments.NewHMACURLSigner(cfg.ObjectStorePublicBaseURL, cfg.ObjectStoreBucket, cfg.ObjectStoreSignSecret)
	if err != nil {
		return nil, fmt.Errorf("build attachment signer: %w", err)
	}

	return &App{
		cfg:               cfg,
		db:                db,
		tokens:            tokenManager,
		authService:       auth.NewService(db, tokenManager),
		expService:        experiments.NewService(db, syncService),
		adminService:      admin.NewService(db, syncService),
		syncService:       syncService,
		attachmentService: attachments.NewService(db, syncService, signer, cfg.AttachmentUploadURLTTL, cfg.AttachmentDownloadURLTTL),
		opsService:        ops.NewService(db),
	}, nil
}

func (a *App) Close() error {
	if a.db != nil {
		return a.db.Close()
	}
	return nil
}

func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if a.cfg.RequireTLS && r.URL.Path != "/healthz" && !isTLSRequest(r) {
		httpx.WriteError(w, http.StatusUpgradeRequired, "tls is required")
		return
	}

	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		a.handleHealth(w)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/login":
		a.handleLogin(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/refresh":
		a.handleRefresh(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/logout":
		a.handleLogout(w, r)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/v1/experiments":
		a.handleCreateExperiment(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/experiments/"):
		a.routeExperimentScope(w, r)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/v1/proposals":
		a.handleCreateProposal(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/proposals":
		a.handleListProposals(w, r)
		return

	case r.Method == http.MethodGet && r.URL.Path == "/v1/sync/pull":
		a.handleSyncPull(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sync/conflicts":
		a.handleSyncConflicts(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/sync/ws":
		a.handleSyncWS(w, r)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/v1/attachments/initiate":
		a.handleAttachmentInitiate(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/attachments/"):
		a.routeAttachmentScope(w, r)
		return

	case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/dashboard":
		a.handleOpsDashboard(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/audit/verify":
		a.handleOpsAuditVerify(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/ops/attachments/reconcile":
		a.handleOpsAttachmentReconcile(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/ops/forensic/export":
		a.handleOpsForensicExport(w, r)
		return
	default:
		http.NotFound(w, r)
		return
	}
}

func isTLSRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		return true
	}
	return false
}

func (a *App) routeExperimentScope(w http.ResponseWriter, r *http.Request) {
	experimentID, action, ok := parseExperimentPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a.handleGetExperiment(w, r, experimentID)
	case r.Method == http.MethodGet && action == "history":
		a.handleGetExperimentHistory(w, r, experimentID)
	case r.Method == http.MethodPost && action == "addendums":
		a.handleCreateAddendum(w, r, experimentID)
	case r.Method == http.MethodPost && action == "complete":
		a.handleMarkCompleted(w, r, experimentID)
	case r.Method == http.MethodPost && action == "comments":
		a.handleCreateComment(w, r, experimentID)
	case r.Method == http.MethodGet && action == "comments":
		a.handleListComments(w, r, experimentID)
	default:
		http.NotFound(w, r)
	}
}

func routeAttachmentPath(path string) (attachmentID string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 4 || parts[0] != "v1" || parts[1] != "attachments" {
		return "", "", false
	}
	if parts[2] == "" || parts[3] == "" {
		return "", "", false
	}
	return parts[2], parts[3], true
}

func (a *App) routeAttachmentScope(w http.ResponseWriter, r *http.Request) {
	attachmentID, action, ok := routeAttachmentPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodPost && action == "complete":
		a.handleAttachmentComplete(w, r, attachmentID)
	case r.Method == http.MethodGet && action == "download":
		a.handleAttachmentDownload(w, r, attachmentID)
	default:
		http.NotFound(w, r)
	}
}

func parseExperimentPath(path string) (experimentID string, action string, ok bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "experiments" {
		return "", "", false
	}
	if parts[2] == "" {
		return "", "", false
	}

	experimentID = parts[2]
	if len(parts) == 3 {
		return experimentID, "", true
	}
	if len(parts) == 4 {
		return experimentID, parts[3], true
	}
	return "", "", false
}

func (a *App) authenticate(r *http.Request) (middleware.AuthUser, error) {
	return middleware.AuthenticateRequest(r, a.tokens)
}

func (a *App) requireAdmin(r *http.Request) (middleware.AuthUser, bool) {
	user, err := a.authenticate(r)
	if err != nil {
		return middleware.AuthUser{}, false
	}
	if user.Role != "admin" {
		return middleware.AuthUser{}, false
	}
	return user, true
}

func (a *App) handleHealth(w http.ResponseWriter) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	type request struct {
		Email      string `json:"email"`
		Password   string `json:"password"`
		DeviceName string `json:"deviceName"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.authService.Login(r.Context(), auth.LoginInput{
		Email:      req.Email,
		Password:   req.Password,
		DeviceName: req.DeviceName,
	})
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleRefresh(w http.ResponseWriter, r *http.Request) {
	type request struct {
		RefreshToken string `json:"refreshToken"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.authService.Refresh(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidRefreshToken) {
			httpx.WriteError(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	type request struct {
		RefreshToken string `json:"refreshToken"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.authService.Logout(r.Context(), req.RefreshToken); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCreateExperiment(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "only owner role can create experiments")
		return
	}

	type request struct {
		Title        string `json:"title"`
		OriginalBody string `json:"originalBody"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.expService.CreateExperiment(r.Context(), experiments.CreateExperimentInput{
		OwnerUserID:  user.ID,
		DeviceID:     user.DeviceID,
		Title:        req.Title,
		OriginalBody: req.OriginalBody,
	})
	if err != nil {
		a.writeExperimentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleCreateAddendum(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "only owner role can add addendums")
		return
	}

	type request struct {
		BaseEntryID string `json:"baseEntryId"`
		Body        string `json:"body"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.expService.AddAddendum(r.Context(), experiments.AddAddendumInput{
		ExperimentID: experimentID,
		OwnerUserID:  user.ID,
		DeviceID:     user.DeviceID,
		BaseEntryID:  req.BaseEntryID,
		Body:         req.Body,
	})
	if err != nil {
		a.writeExperimentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleMarkCompleted(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "only owner role can complete experiments")
		return
	}

	resp, err := a.expService.MarkCompleted(r.Context(), experimentID, user.ID, user.DeviceID)
	if err != nil {
		a.writeExperimentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleGetExperiment(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.expService.GetEffectiveView(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeExperimentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleGetExperimentHistory(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.expService.GetHistory(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeExperimentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleCreateComment(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "admin" {
		httpx.WriteError(w, http.StatusForbidden, "only admin role can add comments")
		return
	}

	type request struct {
		Body string `json:"body"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.adminService.CreateComment(r.Context(), admin.CreateCommentInput{
		ExperimentID: experimentID,
		AdminUserID:  user.ID,
		DeviceID:     user.DeviceID,
		Body:         req.Body,
	})
	if err != nil {
		a.writeAdminError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListComments(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.adminService.ListComments(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeAdminError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"experimentId": experimentID,
		"comments":     resp,
	})
}

func (a *App) handleCreateProposal(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "admin" {
		httpx.WriteError(w, http.StatusForbidden, "only admin role can create proposals")
		return
	}

	type request struct {
		SourceExperimentID string `json:"sourceExperimentId"`
		Title              string `json:"title"`
		Body               string `json:"body"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.adminService.CreateProposal(r.Context(), admin.CreateProposalInput{
		SourceExperimentID: req.SourceExperimentID,
		AdminUserID:        user.ID,
		DeviceID:           user.DeviceID,
		Title:              req.Title,
		Body:               req.Body,
	})
	if err != nil {
		a.writeAdminError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListProposals(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sourceExperimentID := strings.TrimSpace(r.URL.Query().Get("sourceExperimentId"))
	if sourceExperimentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "sourceExperimentId is required")
		return
	}

	resp, err := a.adminService.ListProposals(r.Context(), sourceExperimentID, user.ID, user.Role)
	if err != nil {
		a.writeAdminError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"sourceExperimentId": sourceExperimentID,
		"proposals":          resp,
	})
}

func (a *App) handleSyncPull(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cursor, err := parseInt64Query(r, "cursor", 0)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, err := parseIntQuery(r, "limit", 100)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.syncService.Pull(r.Context(), user.ID, cursor, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleSyncConflicts(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	limit, err := parseIntQuery(r, "limit", 100)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.syncService.ListConflicts(r.Context(), user.ID, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"conflicts": resp})
}

func (a *App) handleSyncWS(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	cursor, err := parseInt64Query(r, "cursor", 0)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.syncService.ServeWS(w, r, user.ID, cursor); err != nil {
		return
	}
}

func (a *App) handleAttachmentInitiate(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "only owner role can upload attachments")
		return
	}

	type request struct {
		ExperimentID string `json:"experimentId"`
		ObjectKey    string `json:"objectKey"`
		SizeBytes    int64  `json:"sizeBytes"`
		MimeType     string `json:"mimeType"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.attachmentService.Initiate(r.Context(), attachments.InitiateInput{
		ExperimentID: req.ExperimentID,
		OwnerUserID:  user.ID,
		DeviceID:     user.DeviceID,
		ObjectKey:    req.ObjectKey,
		SizeBytes:    req.SizeBytes,
		MimeType:     req.MimeType,
	})
	if err != nil {
		a.writeAttachmentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleAttachmentComplete(w http.ResponseWriter, r *http.Request, attachmentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "only owner role can complete attachments")
		return
	}

	type request struct {
		Checksum string `json:"checksum"`
		SizeBytes int64 `json:"sizeBytes"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.attachmentService.Complete(r.Context(), attachments.CompleteInput{
		AttachmentID: attachmentID,
		OwnerUserID:  user.ID,
		DeviceID:     user.DeviceID,
		Checksum:     req.Checksum,
		SizeBytes:    req.SizeBytes,
	})
	if err != nil {
		a.writeAttachmentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleAttachmentDownload(w http.ResponseWriter, r *http.Request, attachmentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.attachmentService.Download(r.Context(), attachments.DownloadInput{
		AttachmentID: attachmentID,
		ViewerUserID: user.ID,
		ViewerRole:   user.Role,
	})
	if err != nil {
		a.writeAttachmentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleOpsDashboard(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	resp, err := a.opsService.Dashboard(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleOpsAuditVerify(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	resp, err := a.opsService.VerifyAuditHashChain(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !resp.Valid {
		httpx.WriteJSON(w, http.StatusConflict, resp)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleOpsAttachmentReconcile(w http.ResponseWriter, r *http.Request) {
	adminUser, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	type request struct {
		StaleAfterSeconds int64 `json:"staleAfterSeconds"`
		ScanLimit         int   `json:"scanLimit"`
	}
	req := request{}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	staleAfter := a.cfg.DefaultReconcileStaleAfter
	if req.StaleAfterSeconds > 0 {
		staleAfter = time.Duration(req.StaleAfterSeconds) * time.Second
	}
	limit := a.cfg.DefaultReconcileScanLimit
	if req.ScanLimit > 0 {
		limit = req.ScanLimit
	}

	resp, err := a.attachmentService.Reconcile(r.Context(), attachments.ReconcileInput{
		ActorUserID: adminUser.ID,
		StaleAfter:  staleAfter,
		Limit:       limit,
	})
	if err != nil {
		a.writeAttachmentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleOpsForensicExport(w http.ResponseWriter, r *http.Request) {
	user, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	experimentID := strings.TrimSpace(r.URL.Query().Get("experimentId"))
	if experimentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "experimentId is required")
		return
	}

	resp, err := a.opsService.ForensicExport(r.Context(), experimentID)
	if err != nil {
		a.writeOpsError(w, err)
		return
	}

	if err := a.opsService.LogForensicExport(r.Context(), user.ID, experimentID); err != nil {
		a.writeOpsError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) writeExperimentError(w http.ResponseWriter, err error) {
	var conflictErr *experiments.ConflictError
	switch {
	case errors.As(err, &conflictErr):
		httpx.WriteJSON(w, http.StatusConflict, map[string]any{
			"error":               conflictErr.Error(),
			"conflictArtifactId":  conflictErr.ConflictArtifactID,
			"experimentId":        conflictErr.ExperimentID,
			"clientBaseEntryId":   conflictErr.ClientBaseEntryID,
			"serverLatestEntryId": conflictErr.ServerLatestEntryID,
		})
	case errors.Is(err, experiments.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, experiments.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, experiments.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeAdminError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, admin.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, admin.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, admin.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeAttachmentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, attachments.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, attachments.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, attachments.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeOpsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ops.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, ops.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, ops.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, "invalid input")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func parseInt64Query(r *http.Request, key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func parseIntQuery(r *http.Request, key string, fallback int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid %s", key)
	}
	if value < 0 {
		return 0, fmt.Errorf("invalid %s", key)
	}
	return value, nil
}

func (a *App) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              a.cfg.HTTPAddr,
		Handler:           a,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server failed: %w", err)
	}
}
