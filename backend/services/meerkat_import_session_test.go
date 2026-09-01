package services

import (
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/meerkatfixture"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fileHeaderFrom builds a *multipart.FileHeader carrying data, so a session
// test can call Upload directly without a full HTTP request.
func fileHeaderFrom(t *testing.T, filename string, data []byte) *multipart.FileHeader {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	part, err := w.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = part.Write(data)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	r := multipart.NewReader(&body, w.Boundary())
	form, err := r.ReadForm(int64(len(data)) + 4096)
	require.NoError(t, err)
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["file"][0]
}

// meerkatFixtureBytes builds the checked-in fixture into a temp Meerkat SQLite
// file and returns its raw bytes.
func meerkatFixtureBytes(t *testing.T) []byte {
	t.Helper()
	m, err := meerkatfixture.Read()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "fixture.sqlite")
	require.NoError(t, meerkatfixture.Populate(path, m))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func meerkatTestLogger() *zerolog.Logger {
	l := zerolog.New(&bytes.Buffer{})
	return &l
}

func waitMeerkatPhase(t *testing.T, m *MeerkatImportManager, userID uint, sid, want string) *models.SourceImportStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, appErr := m.Status(userID, sid)
		require.Nil(t, appErr)
		if st.Phase == want {
			return st
		}
		if st.Phase == models.SourceImportPhaseFailed && want != models.SourceImportPhaseFailed {
			t.Fatalf("meerkat session failed: %s", st.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for meerkat phase %q", want)
	return nil
}

func confirmMeerkatAndWait(t *testing.T, m *MeerkatImportManager, db *gorm.DB, userID uint, sid string, actions []models.RowImportAction, want string) *models.SourceImportStatus {
	t.Helper()
	require.Nil(t, m.Confirm(db, userID, models.SourceImportConfirmRequest{SessionID: sid, Actions: actions}, meerkatTestLogger()))
	return waitMeerkatPhase(t, m, userID, sid, want)
}

func TestMeerkatImportSession_FullFlow_SourceUserPickerAndImport(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	mgr := NewMeerkatImportManager()
	log := meerkatTestLogger()

	upload, appErr := mgr.Upload(user.ID, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)

	// The picker reflects the fixture's two source users with contact counts.
	require.Len(t, upload.SourceUsers, 2)
	byID := map[int64]models.MeerkatSourceUser{}
	for _, u := range upload.SourceUsers {
		byID[u.ID] = u
	}
	assert.Equal(t, "fixture", byID[1].Username)
	assert.Equal(t, 4, byID[1].Contacts)
	assert.Equal(t, 1, byID[2].Contacts)
	require.NotNil(t, upload.DefaultSourceUserID)
	assert.Equal(t, int64(1), *upload.DefaultSourceUserID)

	// Fetch scoped to user 1.
	require.Nil(t, mgr.StartFetch(db, user.ID,
		models.MeerkatFetchRequest{SessionID: upload.SessionID, SourceUserID: upload.DefaultSourceUserID}, log))
	waitMeerkatPhase(t, mgr, user.ID, upload.SessionID, models.SourceImportPhaseReady)

	preview, appErr := mgr.Preview(user.ID, upload.SessionID)
	require.Nil(t, appErr)
	assert.NotNil(t, preview.LossReport)
	// Only user 1's contacts are in the plan (the "other-user" contact is user 2's).
	for _, row := range preview.Rows {
		assert.NotEqual(t, "other-user", row.ParsedContact["firstname"])
	}
	// The fixture's known losses (photo unsupported; a dangling relationship)
	// are named, not silent.
	var sawUnsupported bool
	for _, iss := range preview.LossReport {
		if iss.Category == "unsupported" {
			sawUnsupported = true
		}
	}
	assert.True(t, sawUnsupported, "the fixture's unsupported losses are named: %+v", preview.LossReport)

	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	st := confirmMeerkatAndWait(t, mgr, db, user.ID, upload.SessionID, actions, models.SourceImportPhaseDone)
	require.NotNil(t, st.Result)
	assert.Greater(t, st.Result.Created, 0)

	var runs int64
	db.Model(&models.ImportRun{}).Where("user_id = ? AND format = ?", user.ID, models.ImportFormatMeerkat).Count(&runs)
	assert.Equal(t, int64(1), runs)

	// Re-running the same import is idempotent.
	upload2, appErr := mgr.Upload(user.ID, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID,
		models.MeerkatFetchRequest{SessionID: upload2.SessionID, SourceUserID: upload2.DefaultSourceUserID}, log))
	waitMeerkatPhase(t, mgr, user.ID, upload2.SessionID, models.SourceImportPhaseReady)
	preview2, _ := mgr.Preview(user.ID, upload2.SessionID)
	actions2 := make([]models.RowImportAction, len(preview2.Rows))
	for i, row := range preview2.Rows {
		actions2[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	st2 := confirmMeerkatAndWait(t, mgr, db, user.ID, upload2.SessionID, actions2, models.SourceImportPhaseDone)
	assert.Equal(t, 0, st2.Result.Created, "a re-run creates nothing")
}

func TestMeerkatImportSession_RejectsNonSQLite(t *testing.T) {
	mgr := NewMeerkatImportManager()
	_, appErr := mgr.Upload(1, fileHeaderFrom(t, "notes.db", []byte("this is not a sqlite database at all")))
	require.NotNil(t, appErr)
	assert.Equal(t, "file", appErr.Details["field"])
}

func TestMeerkatImportSession_RejectsWrongExtension(t *testing.T) {
	mgr := NewMeerkatImportManager()
	_, appErr := mgr.Upload(1, fileHeaderFrom(t, "contacts.csv", meerkatFixtureBytes(t)))
	require.NotNil(t, appErr)
	assert.Equal(t, "file", appErr.Details["field"])
}

func TestMeerkatImportSession_RejectsOversized(t *testing.T) {
	mgr := NewMeerkatImportManager()
	h := fileHeaderFrom(t, "big.db", []byte("SQLite format 3\x00padding"))
	h.Size = MaxMeerkatDBSize + 1
	_, appErr := mgr.Upload(1, h)
	require.NotNil(t, appErr)
}

func TestMeerkatImportSession_CancelRemovesTempDir(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	mgr := NewMeerkatImportManager()

	upload, appErr := mgr.Upload(user.ID, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)

	mgr.mu.RLock()
	tempDir := mgr.sessions[upload.SessionID].tempDir
	mgr.mu.RUnlock()
	require.DirExists(t, tempDir)

	require.Nil(t, mgr.Cancel(user.ID, upload.SessionID))
	_, appErr = mgr.Status(user.ID, upload.SessionID)
	require.NotNil(t, appErr, "the session is gone")
	assert.NoDirExists(t, tempDir, "the uploaded file's temp dir is deleted on cancel")
}

func TestMeerkatImportSession_OwnershipEnforced(t *testing.T) {
	db := setupSourceImportTestDB(t)
	owner := createSourceImportUser(t, db)
	mgr := NewMeerkatImportManager()

	upload, appErr := mgr.Upload(owner.ID, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)
	_, appErr = mgr.Status(owner.ID+999, upload.SessionID)
	require.NotNil(t, appErr)
}

func TestMeerkatImportSession_CleanupExpiredRemovesTempDir(t *testing.T) {
	mgr := NewMeerkatImportManager()
	upload, appErr := mgr.Upload(7, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)

	mgr.mu.Lock()
	s := mgr.sessions[upload.SessionID]
	tempDir := s.tempDir
	s.expiresAt = time.Now().Add(-time.Minute)
	s.hardExpiry = time.Now().Add(-time.Minute)
	mgr.mu.Unlock()

	mgr.CleanupExpired()
	assert.Equal(t, 0, mgr.CountActive(7))
	assert.NoDirExists(t, tempDir)
}
