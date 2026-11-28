package services

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/meerkatapifake"
	"mycorrhizal/meerkatapi"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() { meerkatapi.DisableRateLimitForTesting() }

func apiCfg(t *testing.T) *config.Config { return &config.Config{ProfilePhotoDir: t.TempDir()} }

func reasonOf(t *testing.T, e *apperrors.AppError) string {
	t.Helper()
	requireNoReasonProse(t, e.ReasonCode, e.ReasonParams)
	r, _ := e.Details["reason"].(string)
	return r
}

func tokenReq(srv *meerkatapifake.Server) models.MeerkatConnectRequest {
	return models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: meerkatapifake.Token}
}

func passwordReq(srv *meerkatapifake.Server) models.MeerkatConnectRequest {
	return models.MeerkatConnectRequest{BaseURL: srv.URL, Username: meerkatapifake.Username, Password: meerkatapifake.Password}
}

// fakeWithContacts serves n contacts; contact 1 has a relationship to 2, a
// note, and a profile picture.
func fakeWithContacts(t *testing.T, n int) *meerkatapifake.Server {
	t.Helper()
	srv := meerkatapifake.New(t)
	for i := 1; i <= n; i++ {
		srv.Contacts = append(srv.Contacts, meerkatapifake.Contact(i, "Person"+string(rune('A'+i-1))))
	}
	srv.Contacts[0]["photo"] = "abc.png"
	srv.Contacts[0]["relationships"] = []map[string]any{{"ID": 5, "name": "PersonB", "type": "Friend", "contact_id": 1, "related_contact_id": 2}}
	srv.Contacts[0]["notes"] = []map[string]any{{"ID": 9, "content": "met at the lake", "date": "2024-05-06T07:08:09Z", "contact_id": 1}}
	srv.Pictures[1] = meerkatapifake.PNG()
	return srv
}

func TestMeerkatAPI_FullFlow_TokenWithPhotos(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 3)
	cfg := apiCfg(t)
	mgr := NewMeerkatImportManager()
	log := meerkatTestLogger()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), cfg)
	require.Nil(t, appErr)
	assert.True(t, resp.Version.Supported)
	assert.Equal(t, meerkatapi.GenerationV15toV17, resp.Version.Generation)
	assert.Equal(t, 3, resp.Totals.Contacts)
	assert.Equal(t, "ada", resp.SourceUser.Username)
	assert.True(t, resp.InsecureTransport == false)

	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, log))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)

	preview, appErr := mgr.Preview(user.ID, resp.SessionID)
	require.Nil(t, appErr)
	assert.Len(t, preview.Rows, 3)
	for _, iss := range preview.LossReport {
		assert.False(t, iss.Field == "photo" && iss.Category == "unsupported",
			"the API path can carry photos, so they are not reported unsupported: %+v", iss)
	}
	assert.True(t, preview.Rows[0].HasPhoto, "contact 1 is flagged as having a photo")

	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	st := confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
	assert.Equal(t, 3, st.Result.Created)
	assert.Equal(t, 1, st.Result.RelationshipsCreated)
	assert.Equal(t, 1, st.Result.NotesCreated)
	assert.Equal(t, 1, st.Result.PhotosQueued)
	assert.Equal(t, 1, st.Result.PhotosSaved)
	assert.Zero(t, st.Result.PhotosFailed)

	var withPhoto models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "PersonA").First(&withPhoto).Error)
	require.NotEmpty(t, withPhoto.Photo)
	_, err := os.Stat(filepath.Join(cfg.ProfilePhotoDir, withPhoto.Photo))
	assert.NoError(t, err, "the re-encoded picture is on disk")
	assert.NotEmpty(t, withPhoto.PhotoThumbnail)

	assert.Zero(t, srv.Logouts(), "an API token belongs to the user; it is never logged out")
}

// TestMeerkatAPI_FullFlow_PreV15Shape is issue #2084's done-when: a Meerkat
// serving the v1.0-v1.4 contact shape is supported (no "Try anyway") and a
// full import round-trips, the scalar email/phone/address included.
func TestMeerkatAPI_FullFlow_PreV15Shape(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := meerkatapifake.New(t)
	srv.Contacts = []map[string]any{meerkatapifake.LegacyContact(1, "Olga"), meerkatapifake.LegacyContact(2, "Pavel")}
	srv.Contacts[0]["relationships"] = []map[string]any{{"ID": 5, "name": "Pavel", "type": "Friend", "contact_id": 1, "related_contact_id": 2}}
	srv.Contacts[0]["notes"] = []map[string]any{{"ID": 9, "content": "met in 2019", "date": "2019-05-06T07:08:09Z", "contact_id": 1}}
	for _, c := range srv.Contacts {
		for _, k := range []string{"emails", "phones", "addresses", "urls", "impps", "organization", "anniversary"} {
			require.NotContains(t, c, k, "LegacyContact serves the pre-v1.5 shape")
		}
	}
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	assert.True(t, resp.Version.Supported)
	assert.Equal(t, meerkatapi.GenerationV10toV14, resp.Version.Generation)
	assert.Empty(t, resp.Version.ReasonCode)
	assert.Equal(t, 2, resp.Totals.Contacts)

	// Supported: the fetch starts without accept_untested.
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	preview, appErr := mgr.Preview(user.ID, resp.SessionID)
	require.Nil(t, appErr)
	require.Len(t, preview.Rows, 2)

	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	st := confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
	assert.Equal(t, 2, st.Result.Created)
	assert.Equal(t, 1, st.Result.RelationshipsCreated)
	assert.Equal(t, 1, st.Result.NotesCreated)

	var olga models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "Olga").First(&olga).Error)
	assert.Equal(t, "olga@example.com", olga.Email, "the scalar email lands (mapper fallback)")
	assert.NotEmpty(t, olga.Phone, "the scalar phone lands (mapper fallback)")
	assert.Equal(t, meerkatapifake.LegacyAddress, olga.Address, "the scalar address lands (mapper fallback)")
}

func TestMeerkatAPI_PasswordLoginIsLoggedOutWhenDone(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	srv.Pictures = map[int64][]byte{} // no photos: done immediately after the commit
	srv.Contacts[0]["photo"] = ""
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, passwordReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	preview, _ := mgr.Preview(user.ID, resp.SessionID)
	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)

	assert.Eventually(t, func() bool { return srv.Logouts() == 1 }, 3*time.Second, 10*time.Millisecond,
		"the Meerkat session is logged out when the import finishes")
}

func TestMeerkatAPI_CancelDropsSessionAndLogsOut(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, passwordReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)

	require.Nil(t, mgr.Cancel(user.ID, resp.SessionID))
	_, appErr = mgr.Status(user.ID, resp.SessionID)
	require.NotNil(t, appErr, "session is gone")
	assert.Eventually(t, func() bool { return srv.Logouts() == 1 }, 3*time.Second, 10*time.Millisecond)
}

func TestMeerkatAPI_RerunDoesNotDuplicate(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 3)
	srv.Pictures = map[int64][]byte{}
	srv.Contacts[0]["photo"] = ""
	mgr := NewMeerkatImportManager()
	cfg := apiCfg(t)

	run := func() *models.SourceImportPreviewResponse {
		resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), cfg)
		require.Nil(t, appErr)
		require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
		waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
		preview, appErr := mgr.Preview(user.ID, resp.SessionID)
		require.Nil(t, appErr)
		actions := make([]models.RowImportAction, len(preview.Rows))
		for i, row := range preview.Rows {
			act := "add"
			if row.DuplicateMatch != nil {
				act = "skip"
			}
			actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: act}
		}
		confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)
		return preview
	}
	first := run()
	assert.Zero(t, first.DuplicateCount)
	second := run()
	assert.Equal(t, 3, second.DuplicateCount, "a re-run sees every contact as already imported")

	var contacts, rels int64
	db.Model(&models.Contact{}).Where("user_id = ? AND firstname LIKE ?", user.ID, "Person%").Count(&contacts)
	assert.EqualValues(t, 3, contacts, "no duplicate contacts")
	db.Model(&models.RelationshipEdge{}).Count(&rels)
	assert.EqualValues(t, 1, rels, "no duplicate relationship edge")
}

func TestMeerkatAPI_ConnectRejections(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	_ = db
	mgr := NewMeerkatImportManager()
	ctx := context.Background()

	t.Run("no credentials", func(t *testing.T) {
		srv := fakeWithContacts(t, 1)
		_, appErr := mgr.Connect(ctx, user.ID, models.MeerkatConnectRequest{BaseURL: srv.URL}, apiCfg(t))
		require.NotNil(t, appErr)
		assert.Equal(t, 0, mgr.CountActive(user.ID))
	})
	t.Run("wrong password never reaches the response", func(t *testing.T) {
		srv := fakeWithContacts(t, 1)
		_, appErr := mgr.Connect(ctx, user.ID, models.MeerkatConnectRequest{BaseURL: srv.URL, Username: "ada", Password: "SECRET-WRONG-PW"}, apiCfg(t))
		require.NotNil(t, appErr)
		assert.NotContains(t, reasonOf(t, appErr), "SECRET-WRONG-PW")
		assert.Equal(t, 0, mgr.CountActive(user.ID), "a failed connect leaves no credential-holding session")
	})
	t.Run("bad token", func(t *testing.T) {
		srv := fakeWithContacts(t, 1)
		_, appErr := mgr.Connect(ctx, user.ID, models.MeerkatConnectRequest{BaseURL: srv.URL, APIToken: "SECRET-BAD-TOKEN"}, apiCfg(t))
		require.NotNil(t, appErr)
		assert.NotContains(t, reasonOf(t, appErr), "SECRET-BAD-TOKEN")
		assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)
	})
	t.Run("non-Meerkat host never sees the token", func(t *testing.T) {
		srv := fakeWithContacts(t, 1)
		srv.BreakSignature = true
		_, appErr := mgr.Connect(ctx, user.ID, tokenReq(srv), apiCfg(t))
		require.NotNil(t, appErr)
		assert.Empty(t, srv.AuthSeen, "no authenticated request may reach a host that fails the /health signature")
	})
	t.Run("invalid URL", func(t *testing.T) {
		_, appErr := mgr.Connect(ctx, user.ID, models.MeerkatConnectRequest{BaseURL: "ftp://x", APIToken: "t"}, apiCfg(t))
		require.NotNil(t, appErr)
	})
	t.Run("private address blocked when configured", func(t *testing.T) {
		srv := fakeWithContacts(t, 1) // listens on loopback
		cfg := apiCfg(t)
		cfg.MeerkatBlockPrivateURLs = true
		_, appErr := mgr.Connect(ctx, user.ID, tokenReq(srv), cfg)
		require.NotNil(t, appErr)
		assert.Contains(t, reasonOf(t, appErr), "private address")
		assert.Empty(t, srv.Requests, "the guarded dialer refuses before any request is made")
	})
	t.Run("too many contacts", func(t *testing.T) {
		srv := meerkatapifake.New(t)
		for i := 0; i < MaxMeerkatContacts+1; i++ {
			srv.Contacts = append(srv.Contacts, map[string]any{"ID": i})
		}
		for _, c := range srv.Contacts {
			for k, v := range meerkatapifake.Contact(1, "x") {
				if _, ok := c[k]; !ok {
					c[k] = v
				}
			}
		}
		_, appErr := mgr.Connect(ctx, user.ID, tokenReq(srv), apiCfg(t))
		require.NotNil(t, appErr)
		assert.Contains(t, reasonOf(t, appErr), "at most")
	})
}

func TestMeerkatAPI_UntestedVersionNeedsAcceptance(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	srv.Reshape = func(c map[string]any) { c["pronouns"] = "they/them" } // a newer Meerkat adds a field
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	assert.False(t, resp.Version.Supported)
	assert.Contains(t, resp.Version.Reason, "newer")
	assert.Equal(t, "meerkat_version_newer_than_tested", resp.Version.ReasonCode, "clients translate the code; reason is the English fallback")

	appErr = mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger())
	require.NotNil(t, appErr, "an untested version must be accepted explicitly")
	st, _ := mgr.Status(user.ID, resp.SessionID)
	assert.Equal(t, models.SourceImportPhaseConnecting, st.Phase, "nothing was fetched")

	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID, AcceptUntested: true}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	var n int64
	db.Model(&models.Contact{}).Where("user_id = ? AND firstname LIKE ?", user.ID, "Person%").Count(&n)
	assert.Zero(t, n, "preview only: nothing is written until confirm")
}

func TestMeerkatAPI_FetchFailureIsRetryableAndSanitized(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	srv.Close() // the server disappears between connect and fetch
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	st := waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseFailed)
	assert.Contains(t, st.Error, "could not be reached")
	assert.NotContains(t, st.Error, meerkatapifake.Token)
}

func TestMeerkatAPI_NoCredentialInAnyResponse(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	mgr := NewMeerkatImportManager()
	req := passwordReq(srv)

	resp, appErr := mgr.Connect(context.Background(), user.ID, req, apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	status, _ := mgr.Status(user.ID, resp.SessionID)
	preview, _ := mgr.Preview(user.ID, resp.SessionID)

	for name, v := range map[string]any{"connect": resp, "status": status, "preview": preview} {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		for _, secret := range []string{req.Password, meerkatapifake.Token} {
			assert.False(t, strings.Contains(string(b), secret), "%s response leaked a credential", name)
		}
	}
}

func TestMeerkatAPI_SessionOwnershipAndExpiryWipeClient(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 1)
	mgr := NewMeerkatImportManager()
	resp, appErr := mgr.Connect(context.Background(), user.ID, passwordReq(srv), apiCfg(t))
	require.Nil(t, appErr)

	_, appErr = mgr.Status(user.ID+1, resp.SessionID)
	require.NotNil(t, appErr, "another user cannot touch the session")

	mgr.mu.Lock()
	mgr.sessions[resp.SessionID].expiresAt = Now().Add(-time.Minute)
	mgr.mu.Unlock()
	mgr.CleanupExpired()
	assert.Eventually(t, func() bool { return srv.Logouts() == 1 }, 3*time.Second, 10*time.Millisecond,
		"an expired session logs its Meerkat login out")
}

func TestMeerkatAPIErrorMapping(t *testing.T) {
	cases := map[error]string{
		meerkatapi.ErrUnauthorized:   "credentials",
		meerkatapi.ErrLocked:         "credentials",
		meerkatapi.ErrInvalidURL:     "base_url",
		meerkatapi.ErrPrivateAddress: "base_url",
		meerkatapi.ErrNotMeerkat:     "base_url",
		meerkatapi.ErrTooLarge:       "base_url",
	}
	for err, field := range cases {
		appErr := meerkatAPIAppError(err)
		assert.Equal(t, field, appErr.Details["field"], err.Error())
		assert.NotEmpty(t, reasonOf(t, appErr))
	}
	for _, err := range []error{meerkatapi.ErrInvalidData, meerkatapi.ErrUnreachable, context.DeadlineExceeded} {
		appErr := meerkatAPIAppError(err)
		// Issue #2912: the default branch used meerkat_upstream_error with the
		// English sentence as a free-text reason param. This guard is what
		// fails until it uses a localizable code (#2927).
		requireNoReasonProse(t, appErr.ReasonCode, appErr.ReasonParams)
		assert.Equal(t, 503, appErr.HTTPStatus, err.Error())
		assert.NotEmpty(t, userSafeMeerkatAPIError(err))
	}
}

// Issue #2912: a connect that cannot reach the host must carry the
// localizable meerkat_fetch_unreachable code, matching the fetch path. The old
// default branch returned meerkat_upstream_error with the English sentence as
// a free-text {reason} param, so every locale showed English.
func TestMeerkatAPI_ConnectUnreachableUsesTheLocalizableReason(t *testing.T) {
	mgr := NewMeerkatImportManager()
	// Port 9 is the discard service: nothing listens, so the dial is refused
	// immediately instead of hanging until the connect timeout.
	_, appErr := mgr.Connect(context.Background(), 1, models.MeerkatConnectRequest{
		BaseURL: "http://127.0.0.1:9", APIToken: "token",
	}, apiCfg(t))
	require.NotNil(t, appErr)
	assert.Equal(t, string(apperrors.ReasonMeerkatFetchUnreachable), appErr.ReasonCode)
	assert.Nil(t, appErr.ReasonParams, "the localized template takes no free-text param")
	assert.Equal(t, "The Meerkat instance could not be reached", appErr.Message)
}

func TestMeerkatAPI_PhotoFailuresAreCountedNotFatal(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 3)
	srv.Contacts[1]["photo"] = "missing.png" // 404 on download
	srv.Contacts[2]["photo"] = "garbage.png" // not an image
	srv.Pictures[3] = []byte("this is not an image")
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	preview, _ := mgr.Preview(user.ID, resp.SessionID)
	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	st := confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)
	assert.Equal(t, 3, st.Result.Created, "photo trouble never fails the import")
	assert.Equal(t, 3, st.Result.PhotosQueued)
	assert.Equal(t, 1, st.Result.PhotosSaved)
	assert.Equal(t, 2, st.Result.PhotosFailed)
}

func TestMeerkatAPI_FetchRejectsReuseWhileRunningAndUploadSessionsStillWork(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 1)
	mgr := NewMeerkatImportManager()
	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	appErr = mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger())
	require.NotNil(t, appErr, "a prepared session cannot be re-fetched")
	assert.Equal(t, 409, appErr.HTTPStatus)
}

func TestMeerkatAPI_ConnectFailsWhenUserLookupFails(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 1)
	srv.FailMe = true
	mgr := NewMeerkatImportManager()
	_, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.NotNil(t, appErr)
	assert.Equal(t, 0, mgr.CountActive(user.ID))
}

func TestMeerkatAPI_PhotoTasksSkipUnmappedAndMalformedRefs(t *testing.T) {
	client, err := meerkatapi.NewClient("http://meerkat.invalid", false)
	require.NoError(t, err)
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	malformed, ok := models.Contact{UserID: user.ID, Firstname: "M"}, models.Contact{UserID: user.ID, Firstname: "OK"}
	require.NoError(t, db.Create(&malformed).Error)
	require.NoError(t, db.Create(&ok).Error)
	s := &meerkatImportSession{client: client, photoDir: "x", userID: user.ID}
	plan := &ImportSourcePlan{Contacts: []MappedContact{
		{Ref: SourceRef{System: "meerkat", ExternalID: "contact/1"}},                  // no photo
		{Ref: SourceRef{System: "meerkat", ExternalID: "contact/2"}, PhotoURL: "p"},   // not imported
		{Ref: SourceRef{System: "meerkat", ExternalID: "contact/abc"}, PhotoURL: "p"}, // malformed
		{Ref: SourceRef{System: "meerkat", ExternalID: "contact/4"}, PhotoURL: "p"},   // ok
	}}
	tasks := NewMeerkatImportManager().photoTasks(db, s, plan, map[string]uint{"contact/abc": malformed.ID, "contact/4": ok.ID}, meerkatTestLogger())
	require.Len(t, tasks, 1)
	assert.EqualValues(t, 4, tasks[0].sourceID)
	assert.Equal(t, ok.ID, tasks[0].contactID)
}

// A Meerkat contact with a picture that the user merges into an existing CRM
// contact must not replace the destination's own photo (the engine's merge path
// never replaces one either).
func TestMeerkatAPI_MergeNeverReplacesDestinationPhoto(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 1)
	cfg := apiCfg(t)
	mgr := NewMeerkatImportManager()

	local := models.Contact{UserID: user.ID, Firstname: "PersonA", Lastname: "Test", Photo: "keep-me.jpg", PhotoThumbnail: "keep-me-thumb"}
	require.NoError(t, db.Create(&local).Error)

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), cfg)
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)
	preview, appErr := mgr.Preview(user.ID, resp.SessionID)
	require.Nil(t, appErr)
	require.Len(t, preview.Rows, 1)
	require.NotNil(t, preview.Rows[0].DuplicateMatch)
	require.Equal(t, local.ID, preview.Rows[0].DuplicateMatch.ExistingContactID)

	actions := []models.RowImportAction{{RowIndex: preview.Rows[0].RowIndex, Action: "update"}}
	st := confirmMeerkatAndWait(t, mgr, db, user.ID, resp.SessionID, actions, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
	assert.Equal(t, 1, st.Result.Updated)
	assert.Zero(t, st.Result.PhotosQueued, "a destination that already has a photo gets no download task")
	assert.Zero(t, st.Result.PhotosSaved)

	var after models.Contact
	require.NoError(t, db.First(&after, local.ID).Error)
	assert.Equal(t, "keep-me.jpg", after.Photo)
	assert.Equal(t, "keep-me-thumb", after.PhotoThumbnail)
	entries, err := os.ReadDir(cfg.ProfilePhotoDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "nothing was written to the photo directory")
}

// The write itself is guarded too: if the destination gains a photo between
// the task being queued and the download finishing, the new file is dropped.
func TestMeerkatAPI_ProcessPhotosDoesNotOverwriteLatePhoto(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 1)
	cfg := apiCfg(t)
	local := models.Contact{UserID: user.ID, Firstname: "PersonA", Lastname: "Test", Photo: "late.jpg"}
	require.NoError(t, db.Create(&local).Error)

	client, err := meerkatapi.NewClient(srv.URL, false)
	require.NoError(t, err)
	client.SetToken(meerkatapifake.Token)
	s := &meerkatImportSession{client: client, photoDir: cfg.ProfilePhotoDir, userID: user.ID, result: &models.SourceImportResult{}}
	NewMeerkatImportManager().processPhotos(db, s, []meerkatPhotoTask{{contactID: local.ID, sourceID: 1}}, meerkatTestLogger())

	var after models.Contact
	require.NoError(t, db.First(&after, local.ID).Error)
	assert.Equal(t, "late.jpg", after.Photo)
	assert.Equal(t, 1, s.result.PhotosFailed)
	entries, err := os.ReadDir(cfg.ProfilePhotoDir)
	require.NoError(t, err)
	assert.Empty(t, entries, "the orphaned download is removed")
}

func TestMeerkatAPI_PhotoTasksFailSafeWhenLookupFails(t *testing.T) {
	client, err := meerkatapi.NewClient("http://meerkat.invalid", false)
	require.NoError(t, err)
	db := setupSourceImportTestDB(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	s := &meerkatImportSession{client: client, photoDir: "x", userID: 1}
	plan := &ImportSourcePlan{Contacts: []MappedContact{{Ref: SourceRef{System: "meerkat", ExternalID: "contact/4"}, PhotoURL: "p"}}}
	assert.Empty(t, NewMeerkatImportManager().photoTasks(db, s, plan, map[string]uint{"contact/4": 40}, meerkatTestLogger()),
		"an unanswerable ownership/photo query queues nothing rather than risking an overwrite")
}

func TestMeerkatAPI_PhotoTasksSkipOtherUsersContacts(t *testing.T) {
	client, err := meerkatapi.NewClient("http://meerkat.invalid", false)
	require.NoError(t, err)
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	otherUser := models.User{Username: "someone-else", Password: "password123!A", Email: "else@example.com"}
	require.NoError(t, db.Create(&otherUser).Error)
	other := models.Contact{UserID: otherUser.ID, Firstname: "Other"}
	require.NoError(t, db.Create(&other).Error)
	s := &meerkatImportSession{client: client, photoDir: "x", userID: user.ID}
	plan := &ImportSourcePlan{Contacts: []MappedContact{{Ref: SourceRef{System: "meerkat", ExternalID: "contact/4"}, PhotoURL: "p"}}}
	assert.Empty(t, NewMeerkatImportManager().photoTasks(db, s, plan, map[string]uint{"contact/4": other.ID}, meerkatTestLogger()))
}

// Issue #2727: a failed Meerkat fetch job is reason-coded so clients can
// localize it and drop the "use the database upload" advice where no upload
// exists. The English string stays as the fallback.
func TestMeerkatAPI_FetchFailureCarriesReasonCode(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	srv := fakeWithContacts(t, 2)
	mgr := NewMeerkatImportManager()

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	srv.Close()
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	st := waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseFailed)

	assert.Equal(t, "meerkat_fetch_unreachable", st.ErrorReasonCode)
	assert.Contains(t, st.Error, "use the database upload", "the English fallback is unchanged")

	raw, err := json.Marshal(st)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"error_reason_code":"meerkat_fetch_unreachable"`)
	assert.NotContains(t, string(raw), "error_reason_params", "no params when the code has none")
}

func TestMeerkatFetchFailure_MapsEveryCause(t *testing.T) {
	cases := []struct {
		err    error
		code   string
		params map[string]any
	}{
		{meerkatapi.ErrInvalidData, "meerkat_fetch_unreadable", nil},
		{meerkatapi.ErrUnreachable, "meerkat_fetch_unreachable", nil},
		{context.DeadlineExceeded, "meerkat_fetch_unreachable", nil},
		{meerkatapi.ErrUnauthorized, "meerkat_api_error", map[string]any{"reason": "Meerkat rejected the credentials"}},
		{meerkatapi.ErrLocked, "meerkat_api_error", map[string]any{"reason": userSafeMeerkatAPIError(meerkatapi.ErrLocked)}},
		{meerkatapi.ErrInvalidURL, "meerkat_api_error", map[string]any{"reason": "The Meerkat URL is invalid"}},
		{meerkatapi.ErrPrivateAddress, "meerkat_api_error", map[string]any{"reason": userSafeMeerkatAPIError(meerkatapi.ErrPrivateAddress)}},
		{meerkatapi.ErrNotMeerkat, "meerkat_api_error", map[string]any{"reason": userSafeMeerkatAPIError(meerkatapi.ErrNotMeerkat)}},
		{meerkatapi.ErrTooLarge, "meerkat_api_error", map[string]any{"reason": "This Meerkat account is larger than the import supports"}},
	}
	for _, tc := range cases {
		msg, code, params := meerkatFetchFailure(tc.err)
		assert.Equal(t, userSafeMeerkatAPIError(tc.err), msg, tc.err.Error())
		assert.Equal(t, tc.code, string(code), tc.err.Error())
		assert.Equal(t, tc.params, params, tc.err.Error())
	}
}

// Issue #2727: a later plain fail() clears a stale reason code, and a coded
// failure echoes its params through Status.
func TestMeerkatSession_FailClearsReasonAndCodedFailureEchoesParams(t *testing.T) {
	mgr := NewMeerkatImportManager()
	s := &meerkatImportSession{userID: 7, phase: models.SourceImportPhaseMapping, expiresAt: time.Now().Add(time.Hour), hardExpiry: time.Now().Add(time.Hour)}
	mgr.sessions = map[string]*meerkatImportSession{"s": s}

	s.failCoded("bad", apperrors.ReasonCode("meerkat_api_error"), map[string]any{"reason": "r"})
	st, appErr := mgr.Status(7, "s")
	require.Nil(t, appErr)
	assert.Equal(t, "meerkat_api_error", st.ErrorReasonCode)
	assert.Equal(t, map[string]any{"reason": "r"}, st.ErrorReasonParams)

	s.fail("plain")
	st, appErr = mgr.Status(7, "s")
	require.Nil(t, appErr)
	assert.Equal(t, "plain", st.Error)
	assert.Empty(t, st.ErrorReasonCode)
	assert.Nil(t, st.ErrorReasonParams)
}
