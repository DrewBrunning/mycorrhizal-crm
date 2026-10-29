package services

import (
	"context"
	"database/sql"
	"mime/multipart"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireMeerkatReason(t *testing.T, appErr *apperrors.AppError, want apperrors.ReasonCode) {
	t.Helper()
	require.NotNil(t, appErr)
	assert.Equal(t, string(want), appErr.ReasonCode)
}

func TestMeerkatImportSession_UploadRejectionReasons(t *testing.T) {
	mgr := NewMeerkatImportManager()

	t.Run("oversized", func(t *testing.T) {
		h := fileHeaderFrom(t, "big.db", []byte("SQLite format 3\x00padding"))
		h.Size = MaxMeerkatDBSize + 1
		_, appErr := mgr.Upload(1, h)
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatDBSize)
	})
	t.Run("wrong extension", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "contacts.csv", meerkatFixtureBytes(t)))
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatUploadNotSQLite)
	})
	t.Run("not sqlite", func(t *testing.T) {
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "notes.db", []byte("this is not a sqlite database at all")))
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatFileNotMeerkat)
	})
	t.Run("upload cannot be opened", func(t *testing.T) {
		_, appErr := mgr.Upload(1, &multipart.FileHeader{Filename: "gone.db", Size: 10})
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatUploadUnreadable)
	})
}

func TestMeerkatImportSession_LookupReasons(t *testing.T) {
	db := setupSourceImportTestDB(t)
	owner := createSourceImportUser(t, db)
	mgr := NewMeerkatImportManager()

	_, appErr := mgr.Status(owner.ID, "missing")
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatSessionNotFound)

	upload, appErr := mgr.Upload(owner.ID, fileHeaderFrom(t, "meerkat.db", meerkatFixtureBytes(t)))
	require.Nil(t, appErr)

	_, appErr = mgr.Status(owner.ID+999, upload.SessionID)
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatSessionNotOwned)

	// Not yet fetched: preview and confirm are refused as not prepared.
	_, appErr = mgr.Preview(owner.ID, upload.SessionID)
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatNotPrepared)
	appErr = mgr.Confirm(db, owner.ID, models.SourceImportConfirmRequest{SessionID: upload.SessionID}, meerkatTestLogger())
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatNotPrepared)

	mgr.mu.Lock()
	s := mgr.sessions[upload.SessionID]
	s.expiresAt = time.Now().Add(-time.Minute)
	s.hardExpiry = time.Now().Add(-time.Minute)
	mgr.mu.Unlock()
	_, appErr = mgr.Status(owner.ID, upload.SessionID)
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatSessionExpired)
}

func TestMeerkatAPI_ConflictAndConnectReasons(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	mgr := NewMeerkatImportManager()
	srv := fakeWithContacts(t, 1)

	_, appErr := mgr.Connect(context.Background(), user.ID, models.MeerkatConnectRequest{BaseURL: srv.URL}, apiCfg(t))
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatSignInInputMissing)

	resp, appErr := mgr.Connect(context.Background(), user.ID, tokenReq(srv), apiCfg(t))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger()))
	waitMeerkatPhase(t, mgr, user.ID, resp.SessionID, models.SourceImportPhaseReady)

	appErr = mgr.StartFetch(db, user.ID, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger())
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatImportInProgress)

	// startAPIFetch re-checks the phase itself; reach it directly.
	mgr.mu.RLock()
	s := mgr.sessions[resp.SessionID]
	mgr.mu.RUnlock()
	appErr = mgr.startAPIFetch(db, s, models.MeerkatFetchRequest{SessionID: resp.SessionID}, meerkatTestLogger())
	requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatImportInProgress)
}

// writeMeerkatSQLite builds a minimal SQLite file holding the given tables,
// each pre-filled with the given number of rows keyed by id.
func writeMeerkatSQLite(t *testing.T, tables map[string]int) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	_, err = db.Exec("CREATE TABLE placeholder (id INTEGER)")
	require.NoError(t, err)
	for name, rows := range tables {
		_, err = db.Exec("CREATE TABLE " + name + " (id INTEGER PRIMARY KEY)")
		require.NoError(t, err)
		if rows > 0 {
			_, err = db.Exec("WITH RECURSIVE n(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM n WHERE i < ?) INSERT INTO "+name+" (id) SELECT i FROM n", rows)
			require.NoError(t, err)
		}
	}
	require.NoError(t, db.Close())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}

func TestMeerkatImportSession_UploadContentReasons(t *testing.T) {
	mgr := NewMeerkatImportManager()

	t.Run("too many contacts", func(t *testing.T) {
		data := writeMeerkatSQLite(t, map[string]int{"contacts": MaxMeerkatContacts + 1})
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "many.db", data))
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatDBTooManyContacts)
	})
	t.Run("too many records", func(t *testing.T) {
		data := writeMeerkatSQLite(t, map[string]int{"notes": MaxMeerkatEntities + 1})
		_, appErr := mgr.Upload(1, fileHeaderFrom(t, "records.db", data))
		requireMeerkatReason(t, appErr, apperrors.ReasonMeerkatDBTooManyRecords)
	})
}
