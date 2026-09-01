package services

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/monica"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const monicaTestToken = "s3cr3t-monica-token"

// mockMonica serves the slice of the Monica list API the assistant fetches.
// Contact 1 is flagged is_dead with no deceased date, which the mapper reports
// as a named loss (so the preview's loss report is non-trivial).
func mockMonica(t *testing.T) *httptest.Server {
	t.Helper()
	list := func(w http.ResponseWriter, data []map[string]any) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": data,
			"meta": map[string]int{"current_page": 1, "last_page": 1, "total": len(data)},
		})
	}
	contacts := []map[string]any{
		{"id": 1, "first_name": "Ada", "last_name": "Lovelace", "is_dead": true},
		{"id": 2, "first_name": "Grace", "last_name": "Hopper", "contactFields": []map[string]any{
			{"content": "grace@example.com", "contact_field_type": map[string]any{"name": "Email", "type": "email"}},
		}},
	}
	notes := []map[string]any{
		{"id": 10, "body": "met at the analytical engine demo", "created_at": "2023-01-02T00:00:00Z",
			"contact": map[string]any{"id": 1, "first_name": "Ada"}},
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+monicaTestToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/contacts":
			list(w, contacts)
		case "/api/notes":
			list(w, notes)
		case "/api/activities", "/api/reminders", "/api/calls", "/api/tasks", "/api/gifts", "/api/debts":
			list(w, nil)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func monicaTestLogger(buf *bytes.Buffer) *zerolog.Logger {
	l := zerolog.New(buf)
	return &l
}

// waitForPhase polls Status until the session reaches want (or "failed"),
// failing the test on timeout.
func waitForPhase(t *testing.T, m *MonicaImportManager, userID uint, sessionID, want string) *models.MonicaImportStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, appErr := m.Status(userID, sessionID)
		require.Nil(t, appErr)
		if st.Phase == want {
			return st
		}
		if st.Phase == models.MonicaPhaseFailed {
			t.Fatalf("fetch failed: %s", st.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for phase %q", want)
	return nil
}

func TestMonicaImportSession_FullFlow_AddAndMerge(t *testing.T) {
	monica.DisableRateLimitForTesting()
	srv := mockMonica(t)
	defer srv.Close()

	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	// A pre-existing local contact matching Grace by name (no email), so the
	// review step detects the duplicate and can merge the Monica email in.
	grace := models.Contact{UserID: user.ID, Firstname: "Grace", Lastname: "Hopper"}
	require.NoError(t, db.Create(&grace).Error)

	var logBuf bytes.Buffer
	log := monicaTestLogger(&logBuf)
	mgr := NewMonicaImportManager()

	// Connect.
	connectResp, appErr := mgr.Connect(context.Background(), user.ID,
		models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: monicaTestToken}, false)
	require.Nil(t, appErr)
	assert.Equal(t, 2, connectResp.Totals.Contacts)
	require.NotEmpty(t, connectResp.SessionID)
	sid := connectResp.SessionID

	// Fetch.
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MonicaFetchRequest{SessionID: sid}, log))
	waitForPhase(t, mgr, user.ID, sid, models.MonicaPhaseReady)

	// Preview: loss report carries the bare-is_dead loss; row 2 is a duplicate.
	preview, appErr := mgr.Preview(user.ID, sid)
	require.Nil(t, appErr)
	require.Len(t, preview.Rows, 2)
	assert.NotNil(t, preview.LossReport)
	var sawDeadLoss bool
	for _, iss := range preview.LossReport {
		if strings.Contains(strings.ToLower(iss.Message), "is_dead") || strings.Contains(strings.ToLower(iss.Field), "dead") {
			sawDeadLoss = true
		}
	}
	assert.True(t, sawDeadLoss, "the bare is_dead loss must be in the pre-confirm loss report: %+v", preview.LossReport)

	adaRowIdx, graceRowIdx := -1, -1
	for _, row := range preview.Rows {
		switch row.ParsedContact["firstname"] {
		case "Ada":
			adaRowIdx = row.RowIndex
			assert.Equal(t, 1, row.Related.Notes, "Ada carries the imported note")
		case "Grace":
			graceRowIdx = row.RowIndex
			require.NotNil(t, row.DuplicateMatch, "Grace must be detected as a duplicate of the seeded contact")
		}
	}
	require.NotEqual(t, -1, adaRowIdx)
	require.NotEqual(t, -1, graceRowIdx)
	result, appErr := mgr.Confirm(db, user.ID, models.MonicaConfirmRequest{
		SessionID: sid,
		Actions: []models.RowImportAction{
			{RowIndex: adaRowIdx, Action: "add"},
			{RowIndex: graceRowIdx, Action: "update"},
		},
	}, &config.Config{ProfilePhotoDir: t.TempDir()}, log)
	require.Nil(t, appErr)
	assert.Equal(t, 1, result.Created)
	assert.Equal(t, 1, result.Updated)
	assert.GreaterOrEqual(t, result.NotesCreated, 1)

	// Grace merged in place; Ada created new.
	var contactCount int64
	db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contactCount)
	assert.Equal(t, int64(2), contactCount)
	var mergedGrace models.Contact
	require.NoError(t, db.First(&mergedGrace, grace.ID).Error)
	assert.Equal(t, "grace@example.com", mergedGrace.Email, "the Monica email was merged into the existing contact")

	// A history row was recorded for the Monica import.
	var runs int64
	db.Model(&models.ImportRun{}).Where("user_id = ? AND format = ?", user.ID, models.ImportFormatMonica).Count(&runs)
	assert.Equal(t, int64(1), runs)

	// The API token never reached the logs.
	assert.NotContains(t, logBuf.String(), monicaTestToken)
}

func TestMonicaImportSession_ReRunIsIdempotent(t *testing.T) {
	monica.DisableRateLimitForTesting()
	srv := mockMonica(t)
	defer srv.Close()

	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	log := monicaTestLogger(&bytes.Buffer{})
	mgr := NewMonicaImportManager()

	run := func() models.MonicaImportResult {
		resp, appErr := mgr.Connect(context.Background(), user.ID,
			models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: monicaTestToken}, false)
		require.Nil(t, appErr)
		require.Nil(t, mgr.StartFetch(db, user.ID, models.MonicaFetchRequest{SessionID: resp.SessionID}, log))
		waitForPhase(t, mgr, user.ID, resp.SessionID, models.MonicaPhaseReady)
		preview, appErr := mgr.Preview(user.ID, resp.SessionID)
		require.Nil(t, appErr)
		actions := make([]models.RowImportAction, len(preview.Rows))
		for i, row := range preview.Rows {
			actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
		}
		res, appErr := mgr.Confirm(db, user.ID,
			models.MonicaConfirmRequest{SessionID: resp.SessionID, Actions: actions},
			&config.Config{ProfilePhotoDir: t.TempDir()}, log)
		require.Nil(t, appErr)
		return *res
	}

	first := run()
	assert.Equal(t, 2, first.Created)

	second := run()
	assert.Equal(t, 0, second.Created, "a re-run creates nothing")
	assert.Equal(t, 2, second.Skipped, "both contacts are skipped as already imported")

	var contactCount int64
	db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contactCount)
	assert.Equal(t, int64(2), contactCount)
}

func TestMonicaImportSession_ConnectRejectsBadToken(t *testing.T) {
	monica.DisableRateLimitForTesting()
	srv := mockMonica(t)
	defer srv.Close()

	mgr := NewMonicaImportManager()
	_, appErr := mgr.Connect(context.Background(), 1,
		models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: "wrong"}, false)
	require.NotNil(t, appErr)
	assert.Equal(t, "api_token", appErr.Details["field"])
}

func TestMonicaImportSession_SessionOwnershipEnforced(t *testing.T) {
	monica.DisableRateLimitForTesting()
	srv := mockMonica(t)
	defer srv.Close()

	db := setupSourceImportTestDB(t)
	owner := createSourceImportUser(t, db)
	mgr := NewMonicaImportManager()

	resp, appErr := mgr.Connect(context.Background(), owner.ID,
		models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: monicaTestToken}, false)
	require.Nil(t, appErr)

	_, appErr = mgr.Status(owner.ID+999, resp.SessionID)
	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusUnauthorized, appErr.HTTPStatus)
}
