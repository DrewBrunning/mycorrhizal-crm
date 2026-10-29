package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"
	"mycorrhizal/monica"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2103 (ADR 0038): the import assistants (CSV/vCard, Monica, account
// bundle) attach a reason code to each user-facing error. These tests assert
// the code on the wire shape (the marshalled envelope), so a dropped
// .WithReason fails them.

func requireImportReason(t *testing.T, appErr *apperrors.AppError, want apperrors.ReasonCode, wantParams map[string]any) {
	t.Helper()
	require.NotNil(t, appErr)
	raw, err := json.Marshal(appErr)
	require.NoError(t, err)
	var body struct {
		ReasonCode   string         `json:"reason_code"`
		ReasonParams map[string]any `json:"reason_params"`
	}
	require.NoError(t, json.Unmarshal(raw, &body), string(raw))
	assert.Equal(t, string(want), body.ReasonCode, "marshalled: %s", raw)
	if wantParams != nil {
		for k, v := range wantParams {
			assert.EqualValues(t, v, body.ReasonParams[k], "param %s in %s", k, raw)
		}
	}
}

func TestImportSession_CSVVCardReasons(t *testing.T) {
	m := NewImportSessionManager()

	_, appErr := m.get("missing", 1)
	requireImportReason(t, appErr, apperrors.ReasonImportSessionNotFound, nil)

	csvID := m.CreateCSVSession(1, []string{"First"}, [][]string{{"Alice"}})
	// A foreign session is indistinguishable from an unknown one (#2203).
	_, appErr = m.get(csvID, 2)
	requireImportReason(t, appErr, apperrors.ReasonImportSessionNotFound, nil)

	m.mu.Lock()
	m.sessions[csvID].session.ExpiresAt = Now().Add(-time.Minute)
	m.mu.Unlock()
	_, appErr = m.get(csvID, 1)
	requireImportReason(t, appErr, apperrors.ReasonImportSessionExpired, nil)

	log := meerkatTestLogger()
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	t.Run("csv confirm before preview", func(t *testing.T) {
		id := m.CreateCSVSession(user.ID, []string{"First"}, [][]string{{"Alice"}})
		_, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{SessionID: id}, log)
		requireImportReason(t, appErr, apperrors.ReasonImportPreviewRequired, nil)
	})
	t.Run("records session on the csv endpoint", func(t *testing.T) {
		id := m.CreateRecordsSession(user.ID, nil, nil)
		_, appErr := m.Confirm(db, user.ID, models.ImportConfirmRequest{SessionID: id}, log)
		requireImportReason(t, appErr, apperrors.ReasonImportEndpointCSVVCFOnly, nil)
	})
	t.Run("csv session on the vcf endpoint", func(t *testing.T) {
		id := m.CreateCSVSession(user.ID, []string{"First"}, [][]string{{"Alice"}})
		_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{SessionID: id}, nil, log)
		requireImportReason(t, appErr, apperrors.ReasonImportEndpointVCFRecordsOnly, nil)
	})
	t.Run("vcf confirm before preview", func(t *testing.T) {
		id := m.CreateVCFSession(user.ID, nil, nil)
		m.mu.Lock()
		m.sessions[id].session.PreviewCached = false
		m.mu.Unlock()
		_, appErr := m.ConfirmVCF(db, user.ID, models.ImportConfirmRequest{SessionID: id}, nil, log)
		requireImportReason(t, appErr, apperrors.ReasonImportPreviewRequired, nil)
	})
}

func TestMonicaImport_SessionLookupReasons(t *testing.T) {
	m := NewMonicaImportManager()
	_, appErr := m.get("missing", 1)
	requireImportReason(t, appErr, apperrors.ReasonMonicaSessionNotFound, nil)

	id := generateSessionID()
	m.sessions[id] = &monicaImportSession{id: id, userID: 1, expiresAt: Now().Add(time.Hour), hardExpiry: Now().Add(time.Hour)}
	_, appErr = m.get(id, 2)
	requireImportReason(t, appErr, apperrors.ReasonMonicaSessionNotFound, nil)

	m.sessions[id].expiresAt = Now().Add(-time.Minute)
	_, appErr = m.get(id, 1)
	requireImportReason(t, appErr, apperrors.ReasonMonicaSessionExpired, nil)
}

func TestMonicaAppError_ReasonPerSentinel(t *testing.T) {
	cases := []struct {
		err  error
		want apperrors.ReasonCode
	}{
		{monica.ErrUnauthorized, apperrors.ReasonMonicaTokenRejected},
		{monica.ErrInvalidURL, apperrors.ReasonMonicaUrlInvalid},
		{monica.ErrPrivateAddress, apperrors.ReasonMonicaUrlPrivate},
		{monica.ErrInvalidData, apperrors.ReasonMonicaNotMonicaAPI},
		{monica.ErrUnreachable, apperrors.ReasonMonicaUnreachable},
		{errors.New("boom"), apperrors.ReasonMonicaUnreachable},
	}
	for _, tc := range cases {
		t.Run(string(tc.want), func(t *testing.T) {
			appErr := monicaAppError(tc.err)
			requireImportReason(t, appErr, tc.want, nil)
			// The reason sentence carries the same user-safe text.
			assert.Contains(t, appErr.Message, userSafeMonicaError(tc.err))
		})
	}
}

func TestMonicaImport_ConnectSizeLimitReasons(t *testing.T) {
	monica.DisableRateLimitForTesting()
	serve := func(contacts, notes int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			total := 0
			switch {
			case strings.HasSuffix(r.URL.Path, "/contacts"):
				total = contacts
			case strings.HasSuffix(r.URL.Path, "/notes"):
				total = notes
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []any{},
				"meta": map[string]int{"current_page": 1, "last_page": 1, "total": total},
			})
		}))
	}
	mgr := NewMonicaImportManager()

	srv := serve(MaxMonicaContacts+1, 0)
	defer srv.Close()
	_, appErr := mgr.Connect(context.Background(), 1, models.MonicaConnectRequest{BaseURL: srv.URL, APIToken: "t"}, false)
	requireImportReason(t, appErr, apperrors.ReasonMonicaTooManyContacts,
		map[string]any{"total": MaxMonicaContacts + 1, "max": MaxMonicaContacts})

	srv2 := serve(1, MaxMonicaEntities+1)
	defer srv2.Close()
	_, appErr = mgr.Connect(context.Background(), 1, models.MonicaConnectRequest{BaseURL: srv2.URL, APIToken: "t"}, false)
	requireImportReason(t, appErr, apperrors.ReasonMonicaTooManyRecords,
		map[string]any{"total": MaxMonicaEntities + 2, "max": MaxMonicaEntities})
}

func TestMonicaImport_PhaseReasons(t *testing.T) {
	mgr := NewMonicaImportManager()
	id := generateSessionID()
	mgr.sessions[id] = &monicaImportSession{
		id: id, userID: 1, expiresAt: Now().Add(time.Hour), hardExpiry: Now().Add(time.Hour),
		phase: models.MonicaPhaseFetchingContacts,
	}

	appErr := mgr.StartFetch(setupSourceImportTestDB(t), 1, models.MonicaFetchRequest{SessionID: id}, meerkatTestLogger())
	requireImportReason(t, appErr, apperrors.ReasonMonicaFetchInProgress, nil)

	_, appErr = mgr.Preview(1, id)
	requireImportReason(t, appErr, apperrors.ReasonMonicaNotFetched, nil)
	appErr = mgr.Confirm(setupSourceImportTestDB(t), 1, models.MonicaConfirmRequest{SessionID: id}, nil, meerkatTestLogger())
	requireImportReason(t, appErr, apperrors.ReasonMonicaNotFetched, nil)
}

func bundleJSON(contacts, notes int, format string, version int) []byte {
	return []byte(fmt.Sprintf(`{"format":%q,"version":%d,"plan":{"contacts":[%s],"notes":[%s]}}`,
		format, version, strings.TrimSuffix(strings.Repeat("{},", contacts), ","), strings.TrimSuffix(strings.Repeat("{},", notes), ",")))
}

func TestMycorrhizalImport_UploadReasons(t *testing.T) {
	mgr := NewMycorrhizalImportManager()
	ok := models.AccountBundleFormat

	t.Run("empty", func(t *testing.T) {
		h := fileHeaderFrom(t, "b.json", []byte("{}"))
		h.Size = 0
		_, appErr := mgr.Upload(1, h)
		requireImportReason(t, appErr, apperrors.ReasonBundleSize, nil)
	})
	t.Run("upload cannot be opened", func(t *testing.T) {
		_, appErr := mgr.Upload(1, &multipart.FileHeader{Filename: "gone.json", Size: 10})
		requireImportReason(t, appErr, apperrors.ReasonBundleUploadUnreadable, nil)
	})
	t.Run("not json", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "b.json", []byte("not json")))
		requireImportReason(t, appErr, apperrors.ReasonBundleNotBundle, nil)
	})
	t.Run("wrong format", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "b.json", bundleJSON(0, 0, "something-else", 1)))
		requireImportReason(t, appErr, apperrors.ReasonBundleNotBundle, nil)
	})
	t.Run("too many contacts", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "b.json", bundleJSON(MaxMycorrhizalContacts+1, 0, ok, models.AccountBundleVersion)))
		requireImportReason(t, appErr, apperrors.ReasonBundleTooManyContacts, nil)
	})
	t.Run("too many records", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "b.json", bundleJSON(0, MaxMycorrhizalEntities+1, ok, models.AccountBundleVersion)))
		requireImportReason(t, appErr, apperrors.ReasonBundleTooManyRecords, nil)
	})
}

func TestMycorrhizalImport_SessionReasons(t *testing.T) {
	db := setupSourceImportTestDB(t)
	mgr := NewMycorrhizalImportManager()

	_, appErr := mgr.Status(1, "missing")
	requireImportReason(t, appErr, apperrors.ReasonBundleSessionNotFound, nil)

	id := generateSessionID()
	mgr.sessions[id] = &mycorrhizalImportSession{id: id, userID: 1, expiresAt: Now().Add(time.Hour), hardExpiry: Now().Add(time.Hour)}
	_, appErr = mgr.Status(2, id)
	requireImportReason(t, appErr, apperrors.ReasonBundleSessionNotFound, nil)

	// Not yet prepared: preview and confirm are refused.
	_, appErr = mgr.Preview(1, id)
	requireImportReason(t, appErr, apperrors.ReasonBundleNotPrepared, nil)
	appErr = mgr.Confirm(db, 1, models.SourceImportConfirmRequest{SessionID: id}, meerkatTestLogger())
	requireImportReason(t, appErr, apperrors.ReasonBundleNotPrepared, nil)

	mgr.sessions[id].phase = models.SourceImportPhaseFetchingContacts
	appErr = mgr.StartFetch(db, 1, models.MycorrhizalFetchRequest{SessionID: id}, meerkatTestLogger())
	requireImportReason(t, appErr, apperrors.ReasonBundleImportInProgress, nil)

	mgr.sessions[id].expiresAt = Now().Add(-time.Minute)
	mgr.sessions[id].hardExpiry = Now().Add(-time.Minute)
	_, appErr = mgr.Status(1, id)
	requireImportReason(t, appErr, apperrors.ReasonBundleSessionExpired, nil)
}
