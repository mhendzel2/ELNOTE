package app

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/mjhen/elnote/server/internal/admin"
	"github.com/mjhen/elnote/server/internal/attachments"
	"github.com/mjhen/elnote/server/internal/auth"
	"github.com/mjhen/elnote/server/internal/config"
	"github.com/mjhen/elnote/server/internal/datavis"
	internaldb "github.com/mjhen/elnote/server/internal/db"
	"github.com/mjhen/elnote/server/internal/experiments"
	"github.com/mjhen/elnote/server/internal/httpx"
	"github.com/mjhen/elnote/server/internal/middleware"
	"github.com/mjhen/elnote/server/internal/notifications"
	"github.com/mjhen/elnote/server/internal/ops"
	"github.com/mjhen/elnote/server/internal/previews"
	"github.com/mjhen/elnote/server/internal/protocols"
	"github.com/mjhen/elnote/server/internal/reagents"
	"github.com/mjhen/elnote/server/internal/search"
	"github.com/mjhen/elnote/server/internal/signatures"
	"github.com/mjhen/elnote/server/internal/syncer"
	"github.com/mjhen/elnote/server/internal/templates"
	"github.com/mjhen/elnote/server/internal/users"
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
	protocolService   *protocols.Service
	searchService     *search.Service
	userService       *users.Service
	signatureService  *signatures.Service
	notifService      *notifications.Service
	datavisService    *datavis.Service
	templateService   *templates.Service
	previewService    *previews.Service
	reagentService    *reagents.Service
}

func New(cfg config.Config, db *sql.DB) (*App, error) {
	tokenManager := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTIssuer, cfg.AccessTokenTTL, cfg.RefreshTokenTTL)
	syncService := syncer.NewService(db)
	signer, err := attachments.NewHMACURLSigner(cfg.ObjectStorePublicBaseURL, cfg.ObjectStoreBucket, cfg.ObjectStoreSignSecret)
	if err != nil {
		return nil, fmt.Errorf("build attachment signer: %w", err)
	}
	objectInspector := attachments.NewSignedURLObjectInspector(signer, cfg.ObjectStoreInventoryURL, cfg.ObjectStoreProbeTimeout)

	return &App{
		cfg:               cfg,
		db:                db,
		tokens:            tokenManager,
		authService:       auth.NewService(db, tokenManager),
		expService:        experiments.NewService(db, syncService),
		adminService:      admin.NewService(db, syncService),
		syncService:       syncService,
		attachmentService: attachments.NewService(db, syncService, signer, objectInspector, cfg.AttachmentUploadURLTTL, cfg.AttachmentDownloadURLTTL),
		opsService:        ops.NewService(db),
		protocolService:   protocols.NewService(db, syncService),
		searchService:     search.NewService(db),
		userService:       users.NewService(db),
		signatureService:  signatures.NewService(db, syncService),
		notifService:      notifications.NewService(db),
		datavisService:    datavis.NewService(db, syncService),
		templateService:   templates.NewService(db, syncService),
		previewService:    previews.NewService(db),
		reagentService:    reagents.NewService(db),
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
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
	}
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

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
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/request-account":
		a.handleCreateAccountRequest(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/refresh":
		a.handleRefresh(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/auth/logout":
		a.handleLogout(w, r)
		return

	// --- Projects ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/projects":
		a.handleCreateProject(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/projects":
		a.handleListProjects(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/projects/"):
		a.routeProjectScope(w, r)
		return

	case r.Method == http.MethodPost && r.URL.Path == "/v1/experiments":
		a.handleCreateExperiment(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/experiments/clone":
		a.handleCloneExperiment(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/experiments/from-template":
		a.handleCreateFromTemplate(w, r)
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

	// --- Protocols ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/protocols":
		a.handleCreateProtocol(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/protocols":
		a.handleListProtocols(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/protocols/"):
		a.routeProtocolScope(w, r)
		return

	// --- Search ---
	case r.Method == http.MethodGet && r.URL.Path == "/v1/search":
		a.handleSearch(w, r)
		return

	// --- Users (admin) ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/users":
		a.handleCreateUser(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/users":
		a.handleListUsers(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/account-requests":
		a.handleListAccountRequests(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/account-requests/"):
		a.routeAccountRequestScope(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/admin/reset-default":
		a.handleResetDefaultAdmin(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/users/"):
		a.routeUserScope(w, r)
		return

	// --- Signatures ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/signatures":
		a.handleSignExperiment(w, r)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/experiments/") && strings.HasSuffix(r.URL.Path, "/signatures"):
		a.routeExperimentScope(w, r)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/experiments/") && strings.HasSuffix(r.URL.Path, "/signatures/verify"):
		a.routeExperimentScope(w, r)
		return

	// --- Notifications ---
	case r.Method == http.MethodGet && r.URL.Path == "/v1/notifications":
		a.handleListNotifications(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/notifications/read-all":
		a.handleMarkAllNotificationsRead(w, r)
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/notifications/") && strings.HasSuffix(r.URL.Path, "/read"):
		a.handleMarkNotificationRead(w, r)
		return

	// --- Data Visualization ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/data/parse-csv":
		a.handleParseCSV(w, r)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/data/extracts/"):
		a.handleGetDataExtract(w, r)
		return
	case r.Method == http.MethodPost && r.URL.Path == "/v1/charts":
		a.handleCreateChart(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/charts":
		a.handleListCharts(w, r)
		return

	// --- Templates ---
	case r.Method == http.MethodPost && r.URL.Path == "/v1/templates":
		a.handleCreateTemplate(w, r)
		return
	case r.Method == http.MethodGet && r.URL.Path == "/v1/templates":
		a.handleListTemplates(w, r)
		return
	case strings.HasPrefix(r.URL.Path, "/v1/templates/"):
		a.routeTemplateScope(w, r)
		return

	// --- Previews / Thumbnails ---
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/attachments/") && strings.HasSuffix(r.URL.Path, "/preview"):
		a.handleGetAttachmentPreview(w, r)
		return
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/attachments/") && strings.HasSuffix(r.URL.Path, "/generate-preview"):
		a.handleGeneratePreview(w, r)
		return

	// --- Tags ---
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/v1/experiments/") && strings.HasSuffix(r.URL.Path, "/tags"):
		a.routeExperimentScope(w, r)
		return
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/experiments/") && strings.HasSuffix(r.URL.Path, "/tags"):
		a.routeExperimentScope(w, r)
		return

	// --- Reagents (mutable inventory, all authenticated users) ---
	case strings.HasPrefix(r.URL.Path, "/v1/reagents/"):
		a.routeReagentScope(w, r)
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
	case r.Method == http.MethodGet && action == "signatures":
		a.handleListSignatures(w, r, experimentID)
	case action == "signatures" && r.Method == http.MethodGet:
		a.handleListSignatures(w, r, experimentID)
	case r.Method == http.MethodGet && strings.HasPrefix(action, "signatures/verify"):
		a.handleVerifySignatures(w, r, experimentID)
	case r.Method == http.MethodPost && action == "tags":
		a.handleAddTag(w, r, experimentID)
	case r.Method == http.MethodGet && action == "tags":
		a.handleListTags(w, r, experimentID)
	case r.Method == http.MethodGet && action == "data-extracts":
		a.handleListDataExtracts(w, r, experimentID)
	case r.Method == http.MethodGet && action == "previews":
		a.handleListExperimentPreviews(w, r, experimentID)
	case r.Method == http.MethodPost && action == "protocols":
		a.handleLinkProtocol(w, r, experimentID)
	case r.Method == http.MethodPost && action == "deviations":
		a.handleRecordDeviation(w, r, experimentID)
	case r.Method == http.MethodGet && action == "deviations":
		a.handleListDeviations(w, r, experimentID)
	case r.Method == http.MethodGet && action == "attachments":
		a.handleListExperimentAttachments(w, r, experimentID)
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

func (a *App) authenticateSyncWebSocket(r *http.Request) (middleware.AuthUser, error) {
	user, err := a.authenticate(r)
	if err == nil {
		return user, nil
	}

	token := strings.TrimSpace(r.URL.Query().Get("access_token"))
	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("accessToken"))
	}
	if token == "" {
		return middleware.AuthUser{}, err
	}

	claims, parseErr := a.tokens.ParseAccessToken(token)
	if parseErr != nil {
		return middleware.AuthUser{}, parseErr
	}

	return middleware.AuthUser{
		ID:       claims.Sub,
		Role:     claims.Role,
		DeviceID: claims.DeviceID,
	}, nil
}

func (a *App) requireAdmin(r *http.Request) (middleware.AuthUser, bool) {
	user, err := a.authenticate(r)
	if err != nil {
		return middleware.AuthUser{}, false
	}
	if user.Role != "admin" && user.Role != "owner" {
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

func (a *App) handleCreateAccountRequest(w http.ResponseWriter, r *http.Request) {
	type request struct {
		RequestType string `json:"requestType"`
		Username    string `json:"username"`
		Email       string `json:"email"`
		Note        string `json:"note"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := a.userService.CreateAccountRequest(r.Context(), users.CreateAccountRequestInput{
		RequestType: req.RequestType,
		Username:    req.Username,
		Email:       req.Email,
		Note:        req.Note,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	admins, err := a.userService.ListAdminUsers(r.Context())
	if err != nil {
		log.Printf("list admins for account request notification failed: %v", err)
	} else {
		title := "New account request"
		if req.RequestType == "password_recovery" {
			title = "Password recovery request"
		}
		body := fmt.Sprintf("Username: %s\nEmail: %s\nNote: %s", created.Username, created.Email, strings.TrimSpace(created.Note))
		for _, admin := range admins {
			if notifyErr := a.notifService.Create(r.Context(), admin.ID, "auth.account_request", title, body, "account_request", &created.ID); notifyErr != nil {
				log.Printf("notify admin %s for account request failed: %v", admin.Email, notifyErr)
			}
		}
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]any{
		"requestId": created.ID,
		"status":    created.Status,
		"message":   "Request submitted. It now appears in the admin approval queue.",
	})
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

// ---------------------------------------------------------------------------
// Project handlers
// ---------------------------------------------------------------------------

func (a *App) routeProjectScope(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	// /v1/projects/{id} => ["v1","projects","{id}"]
	// /v1/projects/{id}/experiments => ["v1","projects","{id}","experiments"]
	if len(parts) < 3 || parts[0] != "v1" || parts[1] != "projects" || parts[2] == "" {
		http.NotFound(w, r)
		return
	}
	projectID := parts[2]
	action := ""
	if len(parts) == 4 {
		action = parts[3]
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a.handleGetProject(w, r, projectID)
	case r.Method == http.MethodPut && action == "":
		a.handleUpdateProject(w, r, projectID)
	case r.Method == http.MethodDelete && action == "":
		a.handleDeleteProject(w, r, projectID)
	case r.Method == http.MethodGet && action == "experiments":
		a.handleListProjectExperiments(w, r, projectID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(req.Title) == "" {
		httpx.WriteError(w, http.StatusBadRequest, "title is required")
		return
	}

	var project struct {
		ID          string    `json:"id"`
		OwnerUserID string    `json:"ownerUserId"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}
	err = a.db.QueryRowContext(r.Context(),
		`INSERT INTO projects (owner_user_id, title, description)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, owner_user_id::text, title, description, status, created_at, updated_at`,
		user.ID, strings.TrimSpace(req.Title), strings.TrimSpace(req.Description),
	).Scan(&project.ID, &project.OwnerUserID, &project.Title, &project.Description,
		&project.Status, &project.CreatedAt, &project.UpdatedAt)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "create project failed")
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, project)
}

func (a *App) handleListProjects(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var query string
	var args []any
	if user.Role == "admin" || user.Role == "owner" {
		query = `SELECT id::text, owner_user_id::text, title, description, status, created_at, updated_at
				 FROM projects ORDER BY updated_at DESC`
	} else {
		query = `SELECT id::text, owner_user_id::text, title, description, status, created_at, updated_at
				 FROM projects WHERE owner_user_id = $1 ORDER BY updated_at DESC`
		args = append(args, user.ID)
	}

	rows, err := a.db.QueryContext(r.Context(), query, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list projects failed")
		return
	}
	defer rows.Close()

	type project struct {
		ID          string    `json:"id"`
		OwnerUserID string    `json:"ownerUserId"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}
	var projects []project
	for rows.Next() {
		var p project
		if err := rows.Scan(&p.ID, &p.OwnerUserID, &p.Title, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "scan project failed")
			return
		}
		projects = append(projects, p)
	}
	if projects == nil {
		projects = []project{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (a *App) handleGetProject(w http.ResponseWriter, r *http.Request, projectID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type project struct {
		ID          string    `json:"id"`
		OwnerUserID string    `json:"ownerUserId"`
		Title       string    `json:"title"`
		Description string    `json:"description"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
	}
	var p project
	err = a.db.QueryRowContext(r.Context(),
		`SELECT id::text, owner_user_id::text, title, description, status, created_at, updated_at
		 FROM projects WHERE id = $1::uuid`, projectID,
	).Scan(&p.ID, &p.OwnerUserID, &p.Title, &p.Description, &p.Status, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "project not found")
		return
	}

	// Access check: owner, admin, or owner can see
	if p.OwnerUserID != user.ID && user.Role != "admin" && user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	httpx.WriteJSON(w, http.StatusOK, p)
}

func (a *App) handleUpdateProject(w http.ResponseWriter, r *http.Request, projectID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Status      *string `json:"status"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Build dynamic SET clause
	sets := []string{}
	args := []any{}
	idx := 1
	if req.Title != nil {
		sets = append(sets, fmt.Sprintf("title = $%d", idx))
		args = append(args, strings.TrimSpace(*req.Title))
		idx++
	}
	if req.Description != nil {
		sets = append(sets, fmt.Sprintf("description = $%d", idx))
		args = append(args, strings.TrimSpace(*req.Description))
		idx++
	}
	if req.Status != nil {
		if *req.Status != "active" && *req.Status != "archived" {
			httpx.WriteError(w, http.StatusBadRequest, "status must be active or archived")
			return
		}
		sets = append(sets, fmt.Sprintf("status = $%d", idx))
		args = append(args, *req.Status)
		idx++
	}
	if len(sets) == 0 {
		httpx.WriteError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	sets = append(sets, "updated_at = NOW()")
	args = append(args, projectID)

	query := fmt.Sprintf("UPDATE projects SET %s WHERE id = $%d::uuid", strings.Join(sets, ", "), idx)
	if user.Role != "admin" && user.Role != "owner" {
		idx++
		query += fmt.Sprintf(" AND owner_user_id = $%d::uuid", idx)
		args = append(args, user.ID)
	}
	result, err := a.db.ExecContext(r.Context(), query, args...)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "update failed")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteProject(w http.ResponseWriter, r *http.Request, projectID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	// Allow project owner OR admin/owner roles.
	var ownerUserID string
	err = a.db.QueryRowContext(r.Context(), `SELECT owner_user_id::text FROM projects WHERE id = $1::uuid`, projectID).Scan(&ownerUserID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "project not found")
		return
	}
	if user.ID != ownerUserID && user.Role != "admin" && user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	// Un-link experiments first, then delete
	if _, err := a.db.ExecContext(r.Context(), `UPDATE experiments SET project_id = NULL WHERE project_id = $1::uuid`, projectID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "unlink experiments failed")
		return
	}
	result, err := a.db.ExecContext(r.Context(), `DELETE FROM projects WHERE id = $1::uuid`, projectID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "delete project failed")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		httpx.WriteError(w, http.StatusNotFound, "project not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleListProjectExperiments(w http.ResponseWriter, r *http.Request, projectID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var ownerUserID string
	err = a.db.QueryRowContext(r.Context(), `SELECT owner_user_id::text FROM projects WHERE id = $1::uuid`, projectID).Scan(&ownerUserID)
	if err != nil {
		httpx.WriteError(w, http.StatusNotFound, "project not found")
		return
	}
	if user.ID != ownerUserID && user.Role != "admin" && user.Role != "owner" {
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	rows, err := a.db.QueryContext(r.Context(), `
		SELECT e.id::text, e.owner_user_id::text, e.title, e.status, e.created_at, e.updated_at,
			COALESCE((
				SELECT ee.body FROM experiment_entries ee
				WHERE ee.experiment_id = e.id
				ORDER BY ee.created_at DESC LIMIT 1
			), '') AS effective_body
		FROM experiments e
		WHERE e.project_id = $1::uuid
		ORDER BY e.updated_at DESC
	`, projectID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, "list experiments failed")
		return
	}
	defer rows.Close()

	type expSummary struct {
		ID          string    `json:"id"`
		OwnerUserID string    `json:"ownerUserId"`
		Title       string    `json:"title"`
		Status      string    `json:"status"`
		CreatedAt   time.Time `json:"createdAt"`
		UpdatedAt   time.Time `json:"updatedAt"`
		Body        string    `json:"effectiveBody"`
	}
	var exps []expSummary
	for rows.Next() {
		var e expSummary
		if err := rows.Scan(&e.ID, &e.OwnerUserID, &e.Title, &e.Status, &e.CreatedAt, &e.UpdatedAt, &e.Body); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "scan experiment failed")
			return
		}
		exps = append(exps, e)
	}
	if exps == nil {
		exps = []expSummary{}
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"experiments": exps})
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
		Title        string  `json:"title"`
		OriginalBody string  `json:"originalBody"`
		ProjectID    *string `json:"projectId"`
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

	// Link to project if provided
	if req.ProjectID != nil && *req.ProjectID != "" {
		if _, err := a.db.ExecContext(r.Context(),
			`UPDATE experiments SET project_id = $1 WHERE id = $2`,
			*req.ProjectID, resp.ExperimentID,
		); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, "link to project failed")
			return
		}
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
	user, err := a.authenticateSyncWebSocket(r)
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
		Checksum  string `json:"checksum"`
		SizeBytes int64  `json:"sizeBytes"`
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

func (a *App) handleListExperimentAttachments(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	items, err := a.attachmentService.ListByExperiment(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeAttachmentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"attachments": items})
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

// ---------------------------------------------------------------------------
// Protocol handlers
// ---------------------------------------------------------------------------

func parseSubResourcePath(path, prefix string) (resourceID string, action string, ok bool) {
	trimmed := strings.TrimPrefix(path, prefix)
	parts := strings.SplitN(strings.Trim(trimmed, "/"), "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return "", "", false
	}
	resourceID = parts[0]
	if len(parts) == 2 {
		action = parts[1]
	}
	return resourceID, action, true
}

func (a *App) routeProtocolScope(w http.ResponseWriter, r *http.Request) {
	protocolID, action, ok := parseSubResourcePath(r.URL.Path, "/v1/protocols/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a.handleGetProtocol(w, r, protocolID)
	case r.Method == http.MethodPost && action == "publish":
		a.handlePublishProtocolVersion(w, r, protocolID)
	case r.Method == http.MethodGet && action == "versions":
		a.handleListProtocolVersions(w, r, protocolID)
	case r.Method == http.MethodPost && action == "status":
		a.handleUpdateProtocolStatus(w, r, protocolID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleCreateProtocol(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		InitialBody string `json:"initialBody"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.protocolService.CreateProtocol(r.Context(), protocols.CreateProtocolInput{
		OwnerUserID: user.ID,
		Title:       req.Title,
		Description: req.Description,
		InitialBody: req.InitialBody,
	})
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleGetProtocol(w http.ResponseWriter, r *http.Request, protocolID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.protocolService.GetProtocol(r.Context(), protocolID, user.ID, user.Role)
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleListProtocols(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.protocolService.ListProtocols(r.Context(), user.ID, user.Role)
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"protocols": resp})
}

func (a *App) handlePublishProtocolVersion(w http.ResponseWriter, r *http.Request, protocolID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Body      string `json:"body"`
		ChangeLog string `json:"changeLog"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.protocolService.PublishVersion(r.Context(), protocols.PublishVersionInput{
		ProtocolID:    protocolID,
		AuthorUserID:  user.ID,
		Body:          req.Body,
		ChangeSummary: req.ChangeLog,
	})
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListProtocolVersions(w http.ResponseWriter, r *http.Request, protocolID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.protocolService.ListVersions(r.Context(), protocolID, user.ID, user.Role)
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"versions": resp})
}

func (a *App) handleUpdateProtocolStatus(w http.ResponseWriter, r *http.Request, protocolID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if user.Role != "admin" {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	type request struct {
		Status string `json:"status"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.protocolService.UpdateStatus(r.Context(), protocolID, user.ID, req.Status); err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": req.Status})
}

func (a *App) handleLinkProtocol(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		ProtocolID string `json:"protocolId"`
		VersionNum int    `json:"versionNum"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.protocolService.LinkToExperiment(r.Context(), protocols.LinkProtocolInput{
		ExperimentID:      experimentID,
		ProtocolID:        req.ProtocolID,
		ProtocolVersionID: fmt.Sprintf("%d", req.VersionNum),
		ActorUserID:       user.ID,
		DeviceID:          user.DeviceID,
	})
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleRecordDeviation(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		ProtocolID  string `json:"protocolId"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.protocolService.RecordDeviation(r.Context(), protocols.RecordDeviationInput{
		ExperimentID:  experimentID,
		DeviationType: req.Severity,
		Rationale:     req.Description,
		ActorUserID:   user.ID,
		DeviceID:      user.DeviceID,
	})
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListDeviations(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.protocolService.ListDeviations(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeProtocolError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"deviations": resp})
}

// ---------------------------------------------------------------------------
// Search handler
// ---------------------------------------------------------------------------

func (a *App) handleSearch(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httpx.WriteError(w, http.StatusBadRequest, "q is required")
		return
	}

	tag := strings.TrimSpace(r.URL.Query().Get("tag"))
	var tags []string
	if tag != "" {
		tags = []string{tag}
	}

	resp, err := a.searchService.Search(r.Context(), search.SearchInput{
		Query:  q,
		UserID: user.ID,
		Role:   user.Role,
		Tags:   tags,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// User management handlers
// ---------------------------------------------------------------------------

func (a *App) routeUserScope(w http.ResponseWriter, r *http.Request) {
	userID, action, ok := parseSubResourcePath(r.URL.Path, "/v1/users/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a.handleGetUser(w, r, userID)
	case r.Method == http.MethodPut && action == "":
		a.handleUpdateUser(w, r, userID)
	case r.Method == http.MethodDelete && action == "":
		a.handleDeleteUser(w, r, userID)
	case r.Method == http.MethodPost && action == "change-password":
		a.handleChangePassword(w, r, userID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) routeAccountRequestScope(w http.ResponseWriter, r *http.Request) {
	requestID, action, ok := parseSubResourcePath(r.URL.Path, "/v1/account-requests/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodPost && action == "approve":
		a.handleApproveAccountRequest(w, r, requestID)
	case r.Method == http.MethodPost && action == "dismiss":
		a.handleDismissAccountRequest(w, r, requestID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	admin, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	type request struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.userService.CreateUser(r.Context(), users.CreateUserInput{
		Email:       req.Email,
		Password:    req.Password,
		Role:        req.Role,
		AdminUserID: admin.ID,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	_ = a.notifService.Create(
		r.Context(),
		resp.ID,
		"user.account_created",
		"Your account is ready",
		"Your account was created by an administrator. Sign in with your temporary password and change it immediately.",
		"user",
		&resp.ID,
	)

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListUsers(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	resp, err := a.userService.ListUsers(r.Context())
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"users": resp})
}

func (a *App) handleListAccountRequests(w http.ResponseWriter, r *http.Request) {
	_, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	if status == "" {
		status = "pending"
	}
	limit, _ := parseIntQuery(r, "limit", 100)

	resp, err := a.userService.ListAccountRequests(r.Context(), users.ListAccountRequestsInput{
		Status: status,
		Limit:  limit,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"requests": resp})
}

func (a *App) handleApproveAccountRequest(w http.ResponseWriter, r *http.Request, requestID string) {
	admin, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	type request struct {
		Role              string `json:"role"`
		TemporaryPassword string `json:"temporaryPassword"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := a.userService.ApproveAccountRequest(r.Context(), users.ApproveAccountRequestInput{
		RequestID:         requestID,
		AdminUserID:       admin.ID,
		Role:              req.Role,
		TemporaryPassword: req.TemporaryPassword,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (a *App) handleDismissAccountRequest(w http.ResponseWriter, r *http.Request, requestID string) {
	admin, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	updated, err := a.userService.DismissAccountRequest(r.Context(), users.DismissAccountRequestInput{
		RequestID:   requestID,
		AdminUserID: admin.ID,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, updated)
}

func (a *App) handleGetUser(w http.ResponseWriter, r *http.Request, userID string) {
	caller, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if caller.ID != userID && caller.Role != "admin" {
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
		return
	}

	resp, err := a.userService.GetUser(r.Context(), userID)
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request, userID string) {
	admin, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	type request struct {
		Role string `json:"role"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.userService.UpdateUser(r.Context(), users.UpdateUserInput{
		TargetID:    userID,
		AdminUserID: admin.ID,
		Role:        req.Role,
	})
	if err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleChangePassword(w http.ResponseWriter, r *http.Request, userID string) {
	caller, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if caller.ID != userID {
		httpx.WriteError(w, http.StatusForbidden, "can only change own password")
		return
	}

	type request struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.userService.ChangePassword(r.Context(), users.ChangePasswordInput{
		UserID:      userID,
		OldPassword: req.CurrentPassword,
		NewPassword: req.NewPassword,
	}); err != nil {
		a.writeUserError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteUser(w http.ResponseWriter, r *http.Request, userID string) {
	admin, ok := a.requireAdmin(r)
	if !ok {
		httpx.WriteError(w, http.StatusForbidden, "admin role required")
		return
	}

	if err := a.userService.DeleteUser(r.Context(), admin.ID, userID); err != nil {
		a.writeUserError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleResetDefaultAdmin(w http.ResponseWriter, r *http.Request) {
	// This is intentionally unauthenticated so a locked-out admin can recover.
	if err := a.userService.ResetDefaultAdmin(r.Context()); err != nil {
		a.writeUserError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"message": "LabAdmin password has been reset to default",
	})
}

// SeedDefaultAdmin seeds the LabAdmin account at startup.
func (a *App) SeedDefaultAdmin(ctx context.Context) error {
	return a.userService.SeedDefaultAdmin(ctx)
}

// SeedDefaultProtocols inserts the standard protocol library on first run.
func (a *App) SeedDefaultProtocols(ctx context.Context) error {
	// Use the LabAdmin user ID as the protocol owner.
	var adminID string
	err := a.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = 'labadmin' LIMIT 1`).Scan(&adminID)
	if err != nil {
		return fmt.Errorf("find admin user for protocol seeding: %w", err)
	}
	return a.protocolService.SeedDefaultProtocols(ctx, adminID)
}

// ---------------------------------------------------------------------------
// Signature handlers
// ---------------------------------------------------------------------------

func (a *App) handleSignExperiment(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		ExperimentID  string `json:"experimentId"`
		Password      string `json:"password"`
		SignatureType string `json:"signatureType"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.signatureService.Sign(r.Context(), signatures.SignInput{
		ExperimentID:  req.ExperimentID,
		SignerUserID:  user.ID,
		SignatureType: req.SignatureType,
		Password:      req.Password,
		DeviceID:      user.DeviceID,
	})
	if err != nil {
		a.writeSignatureError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListSignatures(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.signatureService.ListSignatures(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"signatures": resp})
}

func (a *App) handleVerifySignatures(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.signatureService.VerifySignatures(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

// ---------------------------------------------------------------------------
// Notification handlers
// ---------------------------------------------------------------------------

func (a *App) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	unreadOnly := strings.TrimSpace(r.URL.Query().Get("unreadOnly")) == "true"
	limit, err2 := parseIntQuery(r, "limit", 50)
	if err2 != nil {
		httpx.WriteError(w, http.StatusBadRequest, err2.Error())
		return
	}

	resp, err := a.notifService.List(r.Context(), notifications.ListInput{
		UserID:     user.ID,
		UnreadOnly: unreadOnly,
		Limit:      limit,
	})
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": resp})
}

func (a *App) handleMarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// Extract notification ID from path: /v1/notifications/{id}/read
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	notifID := parts[2]

	if err := a.notifService.MarkRead(r.Context(), notifID, user.ID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if _, err := a.notifService.MarkAllRead(r.Context(), user.ID); err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Data Visualization handlers
// ---------------------------------------------------------------------------

func (a *App) handleParseCSV(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		AttachmentID string `json:"attachmentId"`
		ExperimentID string `json:"experimentId"`
		CsvData      string `json:"csvData"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.datavisService.ParseCSV(r.Context(), datavis.ParseCSVInput{
		AttachmentID: req.AttachmentID,
		ExperimentID: req.ExperimentID,
		CSVData:      []byte(req.CsvData),
		ActorUserID:  user.ID,
	})
	if err != nil {
		a.writeDatavisError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleGetDataExtract(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	extractID := strings.TrimPrefix(r.URL.Path, "/v1/data/extracts/")
	extractID = strings.TrimSuffix(extractID, "/")

	resp, err := a.datavisService.GetDataExtract(r.Context(), extractID, user.ID, user.Role)
	if err != nil {
		a.writeDatavisError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleListDataExtracts(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.datavisService.ListDataExtracts(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeDatavisError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dataExtracts": resp})
}

func (a *App) handleCreateChart(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		ExperimentID  string         `json:"experimentId"`
		DataExtractID string         `json:"dataExtractId"`
		ChartType     string         `json:"chartType"`
		Title         string         `json:"title"`
		XColumn       string         `json:"xColumn"`
		YColumns      []string       `json:"yColumns"`
		Options       map[string]any `json:"options"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.datavisService.CreateChartConfig(r.Context(), datavis.CreateChartInput{
		ExperimentID:  req.ExperimentID,
		DataExtractID: req.DataExtractID,
		CreatorUserID: user.ID,
		DeviceID:      user.DeviceID,
		ChartType:     req.ChartType,
		Title:         req.Title,
		XColumn:       req.XColumn,
		YColumns:      req.YColumns,
		Options:       req.Options,
	})
	if err != nil {
		a.writeDatavisError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListCharts(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	experimentID := strings.TrimSpace(r.URL.Query().Get("experimentId"))
	if experimentID == "" {
		httpx.WriteError(w, http.StatusBadRequest, "experimentId is required")
		return
	}

	resp, err := a.datavisService.ListChartConfigs(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writeDatavisError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"charts": resp})
}

// ---------------------------------------------------------------------------
// Template handlers
// ---------------------------------------------------------------------------

func (a *App) routeTemplateScope(w http.ResponseWriter, r *http.Request) {
	templateID, action, ok := parseSubResourcePath(r.URL.Path, "/v1/templates/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	switch {
	case r.Method == http.MethodGet && action == "":
		a.handleGetTemplate(w, r, templateID)
	case r.Method == http.MethodPut && action == "":
		a.handleUpdateTemplate(w, r, templateID)
	case r.Method == http.MethodDelete && action == "":
		a.handleDeleteTemplate(w, r, templateID)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) handleCreateTemplate(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Title        string              `json:"title"`
		Description  string              `json:"description"`
		BodyTemplate string              `json:"bodyTemplate"`
		Sections     []templates.Section `json:"sections"`
		ProtocolID   *string             `json:"protocolId"`
		Tags         []string            `json:"tags"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.templateService.CreateTemplate(r.Context(), templates.CreateTemplateInput{
		OwnerUserID:  user.ID,
		Title:        req.Title,
		Description:  req.Description,
		BodyTemplate: req.BodyTemplate,
		Sections:     req.Sections,
		ProtocolID:   req.ProtocolID,
		Tags:         req.Tags,
	})
	if err != nil {
		a.writeTemplateError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.templateService.ListTemplates(r.Context(), user.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"templates": resp})
}

func (a *App) handleGetTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.templateService.GetTemplate(r.Context(), templateID, user.ID)
	if err != nil {
		a.writeTemplateError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleUpdateTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Description  string              `json:"description"`
		BodyTemplate string              `json:"bodyTemplate"`
		Sections     []templates.Section `json:"sections"`
		Tags         []string            `json:"tags"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.templateService.UpdateTemplate(r.Context(), templates.UpdateTemplateInput{
		TemplateID:   templateID,
		OwnerUserID:  user.ID,
		Description:  req.Description,
		BodyTemplate: req.BodyTemplate,
		Sections:     req.Sections,
		Tags:         req.Tags,
	})
	if err != nil {
		a.writeTemplateError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleDeleteTemplate(w http.ResponseWriter, r *http.Request, templateID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	if err := a.templateService.DeleteTemplate(r.Context(), templateID, user.ID); err != nil {
		a.writeTemplateError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleCloneExperiment(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		SourceExperimentID string `json:"sourceExperimentId"`
		NewTitle           string `json:"newTitle"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.templateService.CloneExperiment(r.Context(), templates.CloneExperimentInput{
		SourceExperimentID: req.SourceExperimentID,
		OwnerUserID:        user.ID,
		DeviceID:           user.DeviceID,
		NewTitle:           req.NewTitle,
	})
	if err != nil {
		a.writeTemplateError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleCreateFromTemplate(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		TemplateID string `json:"templateId"`
		Title      string `json:"title"`
		Body       string `json:"body"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	resp, err := a.templateService.CreateFromTemplate(r.Context(), templates.CreateFromTemplateInput{
		TemplateID:  req.TemplateID,
		OwnerUserID: user.ID,
		DeviceID:    user.DeviceID,
		Title:       req.Title,
		Body:        req.Body,
	})
	if err != nil {
		a.writeTemplateError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

// ---------------------------------------------------------------------------
// Preview / Thumbnail handlers
// ---------------------------------------------------------------------------

func (a *App) handleGetAttachmentPreview(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// /v1/attachments/{id}/preview
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	attachmentID := parts[2]

	resp, err := a.previewService.GetPreviewForAttachment(r.Context(), attachmentID, user.ID, user.Role)
	if err != nil {
		a.writePreviewError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, resp)
}

func (a *App) handleGeneratePreview(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	// /v1/attachments/{id}/generate-preview
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 {
		http.NotFound(w, r)
		return
	}
	attachmentID := parts[2]

	// Read raw image data from multipart or base64 JSON body
	var imageData []byte
	var sourceMime string

	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		if err := r.ParseMultipartForm(10 << 20); err != nil { // 10 MB max
			httpx.WriteError(w, http.StatusBadRequest, "invalid multipart data")
			return
		}
		file, header, err := r.FormFile("image")
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "image field is required")
			return
		}
		defer file.Close()
		imageData, err = io.ReadAll(file)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "cannot read image data")
			return
		}
		sourceMime = header.Header.Get("Content-Type")
	} else {
		type request struct {
			ImageBase64 string `json:"imageBase64"`
			MimeType    string `json:"mimeType"`
		}
		var req request
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		decoded, err := decodeBase64(req.ImageBase64)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid base64 image data")
			return
		}
		imageData = decoded
		sourceMime = req.MimeType
	}

	resp, err := a.previewService.GenerateThumbnail(r.Context(), previews.GenerateInput{
		AttachmentID: attachmentID,
		ImageData:    imageData,
		SourceMime:   sourceMime,
		ActorUserID:  user.ID,
	})
	if err != nil {
		a.writePreviewError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (a *App) handleListExperimentPreviews(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	resp, err := a.previewService.ListPreviewsForExperiment(r.Context(), experimentID, user.ID, user.Role)
	if err != nil {
		a.writePreviewError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"previews": resp})
}

// ---------------------------------------------------------------------------
// Tag handlers
// ---------------------------------------------------------------------------

func (a *App) handleAddTag(w http.ResponseWriter, r *http.Request, experimentID string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	type request struct {
		Tag string `json:"tag"`
	}
	var req request
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	tag := strings.TrimSpace(strings.ToLower(req.Tag))
	if tag == "" {
		httpx.WriteError(w, http.StatusBadRequest, "tag is required")
		return
	}

	// Upsert tag, then link
	_, err = a.db.ExecContext(r.Context(),
		`INSERT INTO tags (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`, tag)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var tagID string
	err = a.db.QueryRowContext(r.Context(), `SELECT id FROM tags WHERE name = $1`, tag).Scan(&tagID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	_, err = a.db.ExecContext(r.Context(),
		`INSERT INTO experiment_tags (experiment_id, tag_id, added_by) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		experimentID, tagID, user.ID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	httpx.WriteJSON(w, http.StatusCreated, map[string]string{"tagId": tagID, "tag": tag})
}

func (a *App) handleListTags(w http.ResponseWriter, r *http.Request, experimentID string) {
	_, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	rows, err := a.db.QueryContext(r.Context(),
		`SELECT t.id, t.name FROM tags t JOIN experiment_tags et ON et.tag_id = t.id WHERE et.experiment_id = $1 ORDER BY t.name`,
		experimentID)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type tagResult struct {
		ID   string `json:"tagId"`
		Name string `json:"name"`
	}
	var tags []tagResult
	for rows.Next() {
		var t tagResult
		if err := rows.Scan(&t.ID, &t.Name); err != nil {
			httpx.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		tags = append(tags, t)
	}
	if tags == nil {
		tags = []tagResult{}
	}

	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tags": tags})
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// Ensure json and io imports are used
var (
	_ = json.Unmarshal
	_ = io.ReadAll
)

// ---------------------------------------------------------------------------
// Error writers for new services
// ---------------------------------------------------------------------------

func (a *App) writeProtocolError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, protocols.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, protocols.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, protocols.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeUserError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, users.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, users.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, users.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, users.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, users.ErrDuplicateEmail):
		httpx.WriteError(w, http.StatusConflict, "email already in use")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeSignatureError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, signatures.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, signatures.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, signatures.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, signatures.ErrInvalidPassword):
		httpx.WriteError(w, http.StatusUnauthorized, "invalid password for signing")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeDatavisError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, datavis.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, datavis.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, datavis.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writeTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, templates.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, templates.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, templates.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}

func (a *App) writePreviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, previews.ErrForbidden):
		httpx.WriteError(w, http.StatusForbidden, "forbidden")
	case errors.Is(err, previews.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, previews.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
	}
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

// ===========================================================================
// Reagent handlers — mutable CRUD for lab inventory
// ===========================================================================

func (a *App) routeReagentScope(w http.ResponseWriter, r *http.Request) {
	// Strip prefix to get the sub-path: /v1/reagents/antibodies -> antibodies
	// /v1/reagents/antibodies/42 -> antibodies/42
	sub := strings.TrimPrefix(r.URL.Path, "/v1/reagents/")
	sub = strings.TrimSuffix(sub, "/")

	parts := strings.SplitN(sub, "/", 2)
	resource := parts[0]
	var idStr string
	if len(parts) > 1 {
		idStr = parts[1]
	}

	switch {
	case resource == "import-access" && idStr == "" && r.Method == http.MethodPost:
		a.handleReagentAccessImport(w, r)

	// --- Storage ---
	case resource == "storage" && idStr == "" && r.Method == http.MethodGet:
		a.handleListStorage(w, r)
	case resource == "storage" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateStorage(w, r)
	case resource == "storage" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateStorage(w, r, idStr)
	case resource == "storage" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteStorage(w, r, idStr)

	// --- Boxes ---
	case resource == "boxes" && idStr == "" && r.Method == http.MethodGet:
		a.handleListBoxes(w, r)
	case resource == "boxes" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateBox(w, r)
	case resource == "boxes" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateBox(w, r, idStr)
	case resource == "boxes" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteBox(w, r, idStr)

	// --- Antibodies ---
	case resource == "antibodies" && idStr == "" && r.Method == http.MethodGet:
		a.handleListAntibodies(w, r)
	case resource == "antibodies" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateAntibody(w, r)
	case resource == "antibodies" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateAntibody(w, r, idStr)
	case resource == "antibodies" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteAntibody(w, r, idStr)

	// --- Cell Lines ---
	case resource == "cell-lines" && idStr == "" && r.Method == http.MethodGet:
		a.handleListCellLines(w, r)
	case resource == "cell-lines" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateCellLine(w, r)
	case resource == "cell-lines" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateCellLine(w, r, idStr)
	case resource == "cell-lines" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteCellLine(w, r, idStr)

	// --- Viruses ---
	case resource == "viruses" && idStr == "" && r.Method == http.MethodGet:
		a.handleListViruses(w, r)
	case resource == "viruses" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateVirus(w, r)
	case resource == "viruses" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateVirus(w, r, idStr)
	case resource == "viruses" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteVirus(w, r, idStr)

	// --- DNA ---
	case resource == "dna" && idStr == "" && r.Method == http.MethodGet:
		a.handleListDNA(w, r)
	case resource == "dna" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateDNA(w, r)
	case resource == "dna" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateDNA(w, r, idStr)
	case resource == "dna" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteDNA(w, r, idStr)

	// --- Oligos ---
	case resource == "oligos" && idStr == "" && r.Method == http.MethodGet:
		a.handleListOligos(w, r)
	case resource == "oligos" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateOligo(w, r)
	case resource == "oligos" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateOligo(w, r, idStr)
	case resource == "oligos" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteOligo(w, r, idStr)

	// --- Chemicals ---
	case resource == "chemicals" && idStr == "" && r.Method == http.MethodGet:
		a.handleListChemicals(w, r)
	case resource == "chemicals" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateChemical(w, r)
	case resource == "chemicals" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateChemical(w, r, idStr)
	case resource == "chemicals" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteChemical(w, r, idStr)

	// --- Molecular ---
	case resource == "molecular" && idStr == "" && r.Method == http.MethodGet:
		a.handleListMolecular(w, r)
	case resource == "molecular" && idStr == "" && r.Method == http.MethodPost:
		a.handleCreateMolecular(w, r)
	case resource == "molecular" && idStr != "" && r.Method == http.MethodPut:
		a.handleUpdateMolecular(w, r, idStr)
	case resource == "molecular" && idStr != "" && r.Method == http.MethodDelete:
		a.handleDeleteMolecular(w, r, idStr)

	// --- Cross-type search ---
	case resource == "search" && r.Method == http.MethodGet:
		a.handleReagentSearch(w, r)

	// --- Bulk import ---
	case idStr == "import" && r.Method == http.MethodPost:
		a.handleReagentBulkImport(w, r, resource)

	default:
		http.NotFound(w, r)
	}
}

func (a *App) writeReagentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, reagents.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not found")
	case errors.Is(err, reagents.ErrInvalidInput):
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal error")
	}
}

// --- Storage handlers ---

func (a *App) handleListStorage(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	items, err := a.reagentService.ListStorage(r.Context())
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"storage": items})
}

func (a *App) handleCreateStorage(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Storage
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateStorage(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateStorage(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Storage
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateStorage(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteStorage(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.DeleteStorage(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Box handlers ---

func (a *App) handleListBoxes(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	items, err := a.reagentService.ListBoxes(r.Context(), q)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"boxes": items})
}

func (a *App) handleCreateBox(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Box
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateBox(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateBox(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Box
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateBox(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteBox(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.DeleteBox(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Antibody handlers ---

func (a *App) handleListAntibodies(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListAntibodies(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"antibodies": items})
}

func (a *App) handleCreateAntibody(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Antibody
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateAntibody(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateAntibody(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Antibody
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateAntibody(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteAntibody(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteAntibody(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Cell Line handlers ---

func (a *App) handleListCellLines(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListCellLines(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"cellLines": items})
}

func (a *App) handleCreateCellLine(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.CellLine
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateCellLine(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateCellLine(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.CellLine
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateCellLine(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteCellLine(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteCellLine(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Virus handlers ---

func (a *App) handleListViruses(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListViruses(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"viruses": items})
}

func (a *App) handleCreateVirus(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Virus
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateVirus(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateVirus(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Virus
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateVirus(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteVirus(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteVirus(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- DNA handlers ---

func (a *App) handleListDNA(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListDNA(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"dna": items})
}

func (a *App) handleCreateDNA(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.DNA
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateDNA(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateDNA(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.DNA
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateDNA(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteDNA(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteDNA(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Oligo handlers ---

func (a *App) handleListOligos(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListOligos(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"oligos": items})
}

func (a *App) handleCreateOligo(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Oligo
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateOligo(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateOligo(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Oligo
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateOligo(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteOligo(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteOligo(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Chemical handlers ---

func (a *App) handleListChemicals(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListChemicals(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"chemicals": items})
}

func (a *App) handleCreateChemical(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Chemical
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateChemical(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateChemical(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Chemical
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateChemical(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteChemical(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteChemical(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Molecular handlers ---

func (a *App) handleListMolecular(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	dep := strings.TrimSpace(r.URL.Query().Get("depleted")) == "true"
	items, err := a.reagentService.ListMolecular(r.Context(), q, dep)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"molecular": items})
}

func (a *App) handleCreateMolecular(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	var req reagents.Molecular
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := a.reagentService.CreateMolecular(r.Context(), req, user.ID)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (a *App) handleUpdateMolecular(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req reagents.Molecular
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.reagentService.UpdateMolecular(r.Context(), id, req, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleDeleteMolecular(w http.ResponseWriter, r *http.Request, idStr string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := a.reagentService.SoftDeleteMolecular(r.Context(), id, user.ID); err != nil {
		a.writeReagentError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Cross-type reagent search ---

func (a *App) handleReagentSearch(w http.ResponseWriter, r *http.Request) {
	if _, err := a.authenticate(r); err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	results, err := a.reagentService.SearchAll(r.Context(), q)
	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (a *App) handleReagentAccessImport(w http.ResponseWriter, r *http.Request) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	const maxUploadBytes = 100 << 20 // 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	if err := r.ParseMultipartForm(maxUploadBytes); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid multipart form: "+err.Error())
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer file.Close()

	payload, err := io.ReadAll(file)
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "read file: "+err.Error())
		return
	}

	result, err := a.reagentService.ImportAccessDatabase(r.Context(), reagents.AccessImportInput{
		Filename:  header.Filename,
		FileBytes: payload,
		UserID:    user.ID,
	})
	if err != nil {
		a.writeReagentError(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *App) handleReagentBulkImport(w http.ResponseWriter, r *http.Request, reagentType string) {
	user, err := a.authenticate(r)
	if err != nil {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10 MB max
	if err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}

	var result *reagents.BulkImportResult

	switch reagentType {
	case "storage":
		var req struct {
			Items []reagents.Storage `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportStorage(r.Context(), req.Items, user.ID)
	case "boxes":
		var req struct {
			Items []reagents.Box `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportBoxes(r.Context(), req.Items, user.ID)
	case "antibodies":
		var req struct {
			Items []reagents.Antibody `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportAntibodies(r.Context(), req.Items, user.ID)
	case "cell-lines":
		var req struct {
			Items []reagents.CellLine `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportCellLines(r.Context(), req.Items, user.ID)
	case "viruses":
		var req struct {
			Items []reagents.Virus `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportViruses(r.Context(), req.Items, user.ID)
	case "dna":
		var req struct {
			Items []reagents.DNA `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportDNA(r.Context(), req.Items, user.ID)
	case "oligos":
		var req struct {
			Items []reagents.Oligo `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportOligos(r.Context(), req.Items, user.ID)
	case "chemicals":
		var req struct {
			Items []reagents.Chemical `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportChemicals(r.Context(), req.Items, user.ID)
	case "molecular":
		var req struct {
			Items []reagents.Molecular `json:"items"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		result, err = a.reagentService.BulkImportMolecular(r.Context(), req.Items, user.ID)
	default:
		httpx.WriteError(w, http.StatusNotFound, "unknown reagent type")
		return
	}

	if err != nil {
		a.writeReagentError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (a *App) Run(ctx context.Context) error {
	if a.cfg.ReconcileScheduleEnabled {
		go a.runReconcileScheduler(ctx)
	}

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

func (a *App) runReconcileScheduler(ctx context.Context) {
	interval := a.cfg.ReconcileScheduleInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}

	if a.cfg.ReconcileScheduleRunOnStart {
		a.runReconcileSchedulerTick(ctx)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runReconcileSchedulerTick(ctx)
		}
	}
}

func (a *App) runReconcileSchedulerTick(ctx context.Context) {
	actorUserID, err := a.resolveReconcileSchedulerActorUserID(ctx)
	if err != nil {
		log.Printf("WARN: reconcile scheduler actor lookup failed: %v", err)
		_ = internaldb.AppendAuditEvent(ctx, a.db, "", "attachment.reconcile.scheduler_failed", "attachment_reconcile_run", "", map[string]any{
			"error":         err.Error(),
			"reason":        "actor_lookup_failed",
			"actorEmail":    a.cfg.ReconcileScheduleActorEmail,
			"scanLimit":     a.cfg.DefaultReconcileScanLimit,
			"staleAfterSec": int64(a.cfg.DefaultReconcileStaleAfter.Seconds()),
		})
		return
	}

	out, err := a.attachmentService.Reconcile(ctx, attachments.ReconcileInput{
		ActorUserID: actorUserID,
		StaleAfter:  a.cfg.DefaultReconcileStaleAfter,
		Limit:       a.cfg.DefaultReconcileScanLimit,
	})
	if err != nil {
		log.Printf("WARN: reconcile scheduler run failed: %v", err)
		_ = internaldb.AppendAuditEvent(ctx, a.db, actorUserID, "attachment.reconcile.scheduler_failed", "attachment_reconcile_run", "", map[string]any{
			"error":         err.Error(),
			"scanLimit":     a.cfg.DefaultReconcileScanLimit,
			"staleAfterSec": int64(a.cfg.DefaultReconcileStaleAfter.Seconds()),
		})
		return
	}

	_ = internaldb.AppendAuditEvent(ctx, a.db, actorUserID, "attachment.reconcile.scheduler_succeeded", "attachment_reconcile_run", out.RunID, map[string]any{
		"runId":                   out.RunID,
		"totalFindingsCreated":    out.TotalFindingsCreated,
		"staleInitiatedCount":     out.StaleInitiatedCount,
		"missingChecksumCount":    out.MissingChecksumCount,
		"missingObjectCount":      out.MissingObjectCount,
		"orphanObjectCount":       out.OrphanObjectCount,
		"integrityMismatchCount":  out.IntegrityMismatchCount,
		"objectProbeErrorCount":   out.ObjectProbeErrorCount,
		"objectListingErrorCount": out.ObjectListingErrorCount,
		"scanLimit":               a.cfg.DefaultReconcileScanLimit,
		"staleAfterSec":           int64(a.cfg.DefaultReconcileStaleAfter.Seconds()),
	})
}

func (a *App) resolveReconcileSchedulerActorUserID(ctx context.Context) (string, error) {
	actorEmail := strings.TrimSpace(a.cfg.ReconcileScheduleActorEmail)
	if actorEmail == "" {
		actorEmail = users.DefaultAdminEmail
	}

	var userID string
	if err := a.db.QueryRowContext(ctx, `
		SELECT id::text
		FROM users
		WHERE email = $1
		LIMIT 1
	`, actorEmail).Scan(&userID); err != nil {
		return "", err
	}
	return userID, nil
}
