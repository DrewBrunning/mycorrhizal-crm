package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
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

// TestRun_BadFlag pins the usage exit: an unknown flag is exit code 2 (distinct
// from the runtime-failure code 1), with the flag package's complaint on
// stderr and nothing on stdout.
func TestRun_BadFlag(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run([]string{"-no-such-flag"}, envMap(nil), &out, &errBuf)
	require.Equal(t, 2, code)
	require.Contains(t, errBuf.String(), "flag provided but not defined")
	require.Empty(t, out.String())
}

// searchRebuildFault is services' faultSearchRebuild seam (an unexported
// constant there); the name is part of the documented fault catalogue
// (docs/development/fault-injection.md), so it is stable.
const searchRebuildFault = "services.search.rebuild"

// TestRun_RebuildFailure pins the rebuild-error exit: a failure inside the
// rebuild transaction exits 1 with the cause on stderr and no success line on
// stdout (the CLI must never report "rebuilt successfully" for a failed run).
func TestRun_RebuildFailure(t *testing.T) {
	_, dbPath := newMigratedDB(t)

	injected := errors.New("injected failure mid-FTS-rebuild")
	faults.ArmError(searchRebuildFault, injected)
	t.Cleanup(func() { faults.Disarm(searchRebuildFault) })

	var out, errBuf bytes.Buffer
	code := run([]string{"-db", dbPath}, envMap(nil), &out, &errBuf)
	require.Equal(t, 1, code)
	require.Contains(t, errBuf.String(), "failed to rebuild search index")
	require.Contains(t, errBuf.String(), injected.Error())
	require.Empty(t, out.String(), "a failed rebuild must not print the success line")
}
