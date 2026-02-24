package integration_test

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMandatoryAcceptanceSuite(t *testing.T) {
	env := setupIntegrationEnv(t)

	now := time.Now().UnixNano()
	ownerAEmail := fmt.Sprintf("owner-a-%d@example.com", now)
	ownerBEmail := fmt.Sprintf("owner-b-%d@example.com", now)
	ownerPassword := "Owner#Password1"

	env.createUser(ownerAEmail, ownerPassword, "owner")
	env.createUser(ownerBEmail, ownerPassword, "owner")

	ownerATokenDeviceA := env.login(ownerAEmail, ownerPassword, "owner-a-device-a")
	ownerATokenDeviceB := env.login(ownerAEmail, ownerPassword, "owner-a-device-b")
	ownerBToken := env.login(ownerBEmail, ownerPassword, "owner-b-device-a")
	adminToken := env.adminToken

	t.Run("Immutability", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Immutability", "original-body")
		experimentID := getString(t, exp, "experimentId")
		originalEntryID := getString(t, exp, "originalEntryId")

		status, _, _, addendumResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerATokenDeviceA, map[string]any{
			"baseEntryId": originalEntryID,
			"body":        "addendum-body",
		})
		if status != http.StatusCreated {
			t.Fatalf("create addendum failed: status=%d body=%v", status, addendumResp)
		}
		addendumEntryID := getString(t, asMap(t, addendumResp), "entryId")

		if _, err := env.db.Exec(`UPDATE experiment_entries SET body = 'tampered' WHERE id = $1::uuid`, originalEntryID); err == nil {
			t.Fatal("expected SQL UPDATE on immutable experiment_entries row to fail")
		}
		if _, err := env.db.Exec(`DELETE FROM experiment_entries WHERE id = $1::uuid`, addendumEntryID); err == nil {
			t.Fatal("expected SQL DELETE on immutable experiment_entries row to fail")
		}

		status, _, _, _ = env.doJSON(http.MethodDelete, "/v1/experiments/"+experimentID, ownerATokenDeviceA, nil)
		if status >= 200 && status < 300 {
			t.Fatalf("expected API mutation attempt to fail, got status=%d", status)
		}

		status, _, _, histResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID+"/history", ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("get history failed: status=%d body=%v", status, histResp)
		}
		entries := asSlice(t, asMap(t, histResp)["entries"])
		if len(entries) != 2 {
			t.Fatalf("expected 2 immutable history entries, got %d", len(entries))
		}
	})

	t.Run("AddendumSupersedence", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Supersedence", "original-v1")
		experimentID := getString(t, exp, "experimentId")
		entryBase := getString(t, exp, "originalEntryId")

		status, _, _, add1Resp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerATokenDeviceA, map[string]any{
			"baseEntryId": entryBase,
			"body":        "v2-addendum",
		})
		if status != http.StatusCreated {
			t.Fatalf("addendum v2 failed: status=%d body=%v", status, add1Resp)
		}
		entryV2 := getString(t, asMap(t, add1Resp), "entryId")

		status, _, _, add2Resp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerATokenDeviceA, map[string]any{
			"baseEntryId": entryV2,
			"body":        "v3-addendum",
		})
		if status != http.StatusCreated {
			t.Fatalf("addendum v3 failed: status=%d body=%v", status, add2Resp)
		}
		entryV3 := getString(t, asMap(t, add2Resp), "entryId")

		status, _, _, effectiveResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID, ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("get effective view failed: status=%d body=%v", status, effectiveResp)
		}
		effective := asMap(t, effectiveResp)
		if got := getString(t, effective, "effectiveBody"); got != "v3-addendum" {
			t.Fatalf("expected effective body to be latest addendum, got %q", got)
		}
		if got := getString(t, effective, "effectiveEntryId"); got != entryV3 {
			t.Fatalf("expected effective entry to be latest addendum id %s, got %s", entryV3, got)
		}

		status, _, _, historyResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID+"/history", ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("get history failed: status=%d body=%v", status, historyResp)
		}
		entries := asSlice(t, asMap(t, historyResp)["entries"])
		if len(entries) != 3 {
			t.Fatalf("expected 3 immutable history entries, got %d", len(entries))
		}
		if getString(t, asMap(t, entries[0]), "entryType") != "original" {
			t.Fatalf("first history entry should be original, got %v", entries[0])
		}
		if getString(t, asMap(t, entries[1]), "entryType") != "addendum" || getString(t, asMap(t, entries[2]), "entryType") != "addendum" {
			t.Fatalf("expected trailing entries to be addendums, got %v", entries)
		}
	})

	t.Run("RoleEnforcement", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Role enforcement", "draft-body")
		experimentID := getString(t, exp, "experimentId")
		originalEntryID := getString(t, exp, "originalEntryId")

		status, _, _, createResp := env.doJSON(http.MethodPost, "/v1/experiments", adminToken, map[string]any{
			"title":        "admin-should-fail",
			"originalBody": "x",
		})
		if status != http.StatusForbidden {
			t.Fatalf("admin create experiment should be forbidden, got status=%d body=%v", status, createResp)
		}

		status, _, _, addResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", adminToken, map[string]any{
			"baseEntryId": originalEntryID,
			"body":        "admin-addendum",
		})
		if status != http.StatusForbidden {
			t.Fatalf("admin add addendum should be forbidden, got status=%d body=%v", status, addResp)
		}

		status, _, _, commentBeforeComplete := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/comments", adminToken, map[string]any{"body": "admin comment"})
		if status != http.StatusForbidden {
			t.Fatalf("admin comment on draft should be forbidden, got status=%d body=%v", status, commentBeforeComplete)
		}

		status, _, _, completeResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/complete", ownerATokenDeviceA, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("owner complete experiment failed: status=%d body=%v", status, completeResp)
		}

		status, _, _, commentResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/comments", adminToken, map[string]any{"body": "admin comment after complete"})
		if status != http.StatusCreated {
			t.Fatalf("admin comment on completed experiment should succeed, got status=%d body=%v", status, commentResp)
		}

		status, _, _, proposalResp := env.doJSON(http.MethodPost, "/v1/proposals", adminToken, map[string]any{
			"sourceExperimentId": experimentID,
			"title":              "admin proposal",
			"body":               "proposed follow-up",
		})
		if status != http.StatusCreated {
			t.Fatalf("admin proposal on completed experiment should succeed, got status=%d body=%v", status, proposalResp)
		}
	})

	t.Run("CompletedOnlyAdminAccess", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Completed only", "draft")
		experimentID := getString(t, exp, "experimentId")

		status, _, _, draftReadResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID, adminToken, nil)
		if status != http.StatusForbidden {
			t.Fatalf("admin should not read draft experiment, got status=%d body=%v", status, draftReadResp)
		}

		status, _, _, completeResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/complete", ownerATokenDeviceA, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("owner complete experiment failed: status=%d body=%v", status, completeResp)
		}

		status, _, _, completedReadResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID, adminToken, nil)
		if status != http.StatusOK {
			t.Fatalf("admin should read completed experiment, got status=%d body=%v", status, completedReadResp)
		}

		status, _, _, historyResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID+"/history", adminToken, nil)
		if status != http.StatusOK {
			t.Fatalf("admin should read completed history, got status=%d body=%v", status, historyResp)
		}
	})

	t.Run("SignatureVerifyRoute", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Signature verify", "sign-body")
		experimentID := getString(t, exp, "experimentId")

		status, _, _, completeResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/complete", ownerATokenDeviceA, map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("owner complete experiment failed: status=%d body=%v", status, completeResp)
		}

		status, _, _, signResp := env.doJSON(http.MethodPost, "/v1/signatures", ownerATokenDeviceA, map[string]any{
			"experimentId":  experimentID,
			"password":      ownerPassword,
			"signatureType": "author",
		})
		if status != http.StatusCreated {
			t.Fatalf("author signature failed: status=%d body=%v", status, signResp)
		}

		status, _, _, verifyResp := env.doJSON(http.MethodGet, "/v1/experiments/"+experimentID+"/signatures/verify", ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("signature verify route failed: status=%d body=%v", status, verifyResp)
		}
		verify := asMap(t, verifyResp)
		if !getBool(t, verify, "integrityValid") {
			t.Fatalf("expected signature verification integrityValid=true, got %v", verify)
		}
	})

	t.Run("OwnerOnlyWrite", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Owner-only", "owner-a-original")
		experimentID := getString(t, exp, "experimentId")
		baseEntryID := getString(t, exp, "originalEntryId")

		status, _, _, addendumResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerBToken, map[string]any{
			"baseEntryId": baseEntryID,
			"body":        "owner-b attempt",
		})
		if status != http.StatusForbidden {
			t.Fatalf("non-owner addendum should be forbidden, got status=%d body=%v", status, addendumResp)
		}

		status, _, _, attachResp := env.doJSON(http.MethodPost, "/v1/attachments/initiate", ownerBToken, map[string]any{
			"experimentId": experimentID,
			"objectKey":    "owner-b/forbidden.txt",
			"sizeBytes":    42,
			"mimeType":     "text/plain",
		})
		if status != http.StatusForbidden {
			t.Fatalf("non-owner attachment initiate should be forbidden, got status=%d body=%v", status, attachResp)
		}
	})

	t.Run("SyncSafetyStaleConflict", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Sync stale conflict", "original")
		experimentID := getString(t, exp, "experimentId")
		baseEntryID := getString(t, exp, "originalEntryId")

		status, _, _, addendumAResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerATokenDeviceA, map[string]any{
			"baseEntryId": baseEntryID,
			"body":        "device-a addendum",
		})
		if status != http.StatusCreated {
			t.Fatalf("device A addendum should succeed, got status=%d body=%v", status, addendumAResp)
		}

		status, _, _, addendumBResp := env.doJSON(http.MethodPost, "/v1/experiments/"+experimentID+"/addendums", ownerATokenDeviceB, map[string]any{
			"baseEntryId": baseEntryID,
			"body":        "device-b stale addendum",
		})
		if status != http.StatusConflict {
			t.Fatalf("stale write should conflict, got status=%d body=%v", status, addendumBResp)
		}
		conflictBody := asMap(t, addendumBResp)
		conflictID := getString(t, conflictBody, "conflictArtifactId")
		if conflictID == "" {
			t.Fatalf("expected conflictArtifactId in conflict response: %v", conflictBody)
		}

		status, _, _, conflictsResp := env.doJSON(http.MethodGet, "/v1/sync/conflicts?limit=50", ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("list conflicts failed: status=%d body=%v", status, conflictsResp)
		}
		conflicts := asSlice(t, asMap(t, conflictsResp)["conflicts"])
		found := false
		for _, item := range conflicts {
			m := asMap(t, item)
			if getString(t, m, "conflictArtifactId") == conflictID {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected conflict id %s in conflict list: %v", conflictID, conflicts)
		}
	})

	t.Run("AttachmentMetadataPipeline", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Attachment pipeline", "original")
		experimentID := getString(t, exp, "experimentId")

		status, headers, raw, initiateResp := env.doJSON(http.MethodPost, "/v1/attachments/initiate", ownerATokenDeviceA, map[string]any{
			"experimentId": experimentID,
			"objectKey":    "project-x/results.csv",
			"sizeBytes":    123,
			"mimeType":     "text/csv",
		})
		if status != http.StatusCreated {
			t.Fatalf("attachment initiate failed: status=%d body=%v", status, initiateResp)
		}
		if ct := headers.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("expected JSON response, got content-type=%q", ct)
		}
		if strings.Contains(strings.ToLower(string(raw)), "filebytes") {
			t.Fatalf("unexpected file-bytes payload in initiate response: %s", string(raw))
		}

		initiate := asMap(t, initiateResp)
		attachmentID := getString(t, initiate, "attachmentId")
		uploadURL := getString(t, initiate, "uploadUrl")
		if !strings.Contains(uploadURL, "op=put") || !strings.Contains(uploadURL, "sig=") {
			t.Fatalf("expected signed upload URL, got %q", uploadURL)
		}

		status, _, _, completeResp := env.doJSON(http.MethodPost, "/v1/attachments/"+attachmentID+"/complete", ownerATokenDeviceA, map[string]any{
			"checksum":  "abc123",
			"sizeBytes": 123,
		})
		if status != http.StatusOK {
			t.Fatalf("attachment complete failed: status=%d body=%v", status, completeResp)
		}

		status, headers, raw, downloadResp := env.doJSON(http.MethodGet, "/v1/attachments/"+attachmentID+"/download", ownerATokenDeviceA, nil)
		if status != http.StatusOK {
			t.Fatalf("attachment download metadata failed: status=%d body=%v", status, downloadResp)
		}
		if ct := headers.Get("Content-Type"); !strings.Contains(ct, "application/json") {
			t.Fatalf("expected JSON response, got content-type=%q", ct)
		}
		if strings.Contains(strings.ToLower(string(raw)), "filebytes") {
			t.Fatalf("unexpected file-bytes payload in download response: %s", string(raw))
		}
		downloadURL := getString(t, asMap(t, downloadResp), "downloadUrl")
		if !strings.Contains(downloadURL, "op=get") || !strings.Contains(downloadURL, "sig=") {
			t.Fatalf("expected signed download URL, got %q", downloadURL)
		}
	})

	t.Run("AttachmentReconcileObjectDrift", func(t *testing.T) {
		exp := env.createExperiment(ownerATokenDeviceA, "Attachment drift", "original")
		experimentID := getString(t, exp, "experimentId")

		createCompletedAttachment := func(objectKey, checksum string, sizeBytes int64) string {
			t.Helper()
			status, _, _, initiateResp := env.doJSON(http.MethodPost, "/v1/attachments/initiate", ownerATokenDeviceA, map[string]any{
				"experimentId": experimentID,
				"objectKey":    objectKey,
				"sizeBytes":    sizeBytes,
				"mimeType":     "application/octet-stream",
			})
			if status != http.StatusCreated {
				t.Fatalf("attachment initiate failed: status=%d body=%v", status, initiateResp)
			}
			attachmentID := getString(t, asMap(t, initiateResp), "attachmentId")

			status, _, _, completeResp := env.doJSON(http.MethodPost, "/v1/attachments/"+attachmentID+"/complete", ownerATokenDeviceA, map[string]any{
				"checksum":  checksum,
				"sizeBytes": sizeBytes,
			})
			if status != http.StatusOK {
				t.Fatalf("attachment complete failed: status=%d body=%v", status, completeResp)
			}
			return attachmentID
		}

		_ = createCompletedAttachment("drift/missing-object.bin", "missing-expected", 5)
		_ = createCompletedAttachment("drift/integrity-mismatch.bin", "expected-checksum", 3)
		env.objectStore.putObject("drift/integrity-mismatch.bin", []byte("mismatch-object"), "observed-checksum")
		env.objectStore.putObject("drift/orphan-object.bin", []byte("orphan"), "orphan-checksum")

		status, _, _, reconcileResp := env.doJSON(http.MethodPost, "/v1/ops/attachments/reconcile", adminToken, map[string]any{
			"scanLimit": 100,
		})
		if status != http.StatusOK {
			t.Fatalf("attachment reconcile failed: status=%d body=%v", status, reconcileResp)
		}
		reconcile := asMap(t, reconcileResp)

		missingObjectCount := int(reconcile["missingObjectCount"].(float64))
		orphanObjectCount := int(reconcile["orphanObjectCount"].(float64))
		integrityMismatchCount := int(reconcile["integrityMismatchCount"].(float64))
		if missingObjectCount < 1 {
			t.Fatalf("expected missing object findings, got %v", reconcile)
		}
		if orphanObjectCount < 1 {
			t.Fatalf("expected orphan object findings, got %v", reconcile)
		}
		if integrityMismatchCount < 1 {
			t.Fatalf("expected integrity mismatch findings, got %v", reconcile)
		}

		status, _, _, dashboardResp := env.doJSON(http.MethodGet, "/v1/ops/dashboard", adminToken, nil)
		if status != http.StatusOK {
			t.Fatalf("ops dashboard failed: status=%d body=%v", status, dashboardResp)
		}
		dashboard := asMap(t, dashboardResp)
		if dashboard["reconcileMissingObjectUnresolved"].(float64) < 1 {
			t.Fatalf("expected missing-object metric in dashboard, got %v", dashboard)
		}
		if dashboard["reconcileOrphanObjectUnresolved"].(float64) < 1 {
			t.Fatalf("expected orphan-object metric in dashboard, got %v", dashboard)
		}
		if dashboard["reconcileIntegrityMismatchUnresolved"].(float64) < 1 {
			t.Fatalf("expected integrity-mismatch metric in dashboard, got %v", dashboard)
		}
	})

	t.Run("ForensicAuditHashChain", func(t *testing.T) {
		status, _, _, verifyResp := env.doJSON(http.MethodGet, "/v1/ops/audit/verify", adminToken, nil)
		if status != http.StatusOK {
			t.Fatalf("audit verify failed: status=%d body=%v", status, verifyResp)
		}
		verify := asMap(t, verifyResp)
		if !getBool(t, verify, "valid") {
			t.Fatalf("expected valid audit chain, got %v", verify)
		}

		var count int
		if err := env.db.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count); err != nil {
			t.Fatalf("count audit rows: %v", err)
		}
		if count == 0 {
			t.Fatal("expected audit_log to contain events after write operations")
		}

		var immutableErr error
		_, immutableErr = env.db.Exec(`UPDATE audit_log SET event_type = event_type WHERE id = 1`)
		if immutableErr == nil {
			t.Fatal("expected audit_log update to be rejected by immutability trigger")
		}
		if immutableErr == nil || immutableErr == sql.ErrNoRows {
			t.Fatalf("expected immutable audit_log update failure, got %v", immutableErr)
		}
	})
}
