package main

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func newMigratedDB(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "search.db")
	db := dbtest.NewAt(t, dbPath)
	return db, dbPath
}

// TestRun_RebuildsIndex seeds one contact/note/activity, wipes the derived FTS
// tables to simulate a bulk write that bypassed the triggers, then runs the CLI
// and proves the index was rebuilt: the reported counts match and each row is
// findable via MATCH.
func TestRun_RebuildsIndex(t *testing.T) {
	db, dbPath := newMigratedDB(t)

	user := models.User{Username: "search-cli", Password: "password123!A", Email: "search-cli@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Searcher"}
	require.NoError(t, db.Create(&contact).Error)
	require.NoError(t, db.Create(&models.Note{
		UserID: user.ID, ContactID: &contact.ID, Content: "talked about mushrooms", Date: time.Now(),
	}).Error)
	require.NoError(t, db.Create(&models.Activity{
		UserID: user.ID, Title: "fungi walk", Description: "found chanterelles", Date: time.Now(),
	}).Error)

	// A bulk import / raw-SQL migration bypasses the FTS triggers: wipe the
	// derived tables so the rebuild has real work to do.
	require.NoError(t, db.Exec("DELETE FROM contacts_fts").Error)
	require.NoError(t, db.Exec("DELETE FROM notes_fts").Error)
	require.NoError(t, db.Exec("DELETE FROM activities_fts").Error)

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath}, envMap(nil), &out, &errBuf)
	require.Equal(t, 0, code, "stderr: %s", errBuf.String())
	require.Contains(t, out.String(), "contacts=1 notes=1 activities=1")

	countFTS := func(table, term string) int64 {
		t.Helper()
		var n int64
		require.NoError(t, db.Raw(
			"SELECT COUNT(*) FROM "+table+" WHERE "+table+" MATCH ? AND user_id = ?", term, user.ID,
		).Scan(&n).Error)
		return n
	}
	require.EqualValues(t, 1, countFTS("contacts_fts", "Alice"), "rebuilt index must find the contact")
	require.EqualValues(t, 1, countFTS("notes_fts", "mushrooms"), "rebuilt index must find the note")
	require.EqualValues(t, 1, countFTS("activities_fts", "fungi"), "rebuilt index must find the activity")
}

// TestRun_UnopenableDatabase pins the failure path: a database path that cannot
// be opened exits non-zero with a message.
func TestRun_UnopenableDatabase(t *testing.T) {
	var out, errBuf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "no", "such", "dir", "x.db")
	code := run([]string{"-db", missing}, envMap(nil), &out, &errBuf)
	require.NotZero(t, code)
	require.Contains(t, errBuf.String(), "failed to open database")
}
