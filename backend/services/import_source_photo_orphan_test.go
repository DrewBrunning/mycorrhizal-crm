package services

import (
	"context"
	"encoding/base64"
	"os"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/photostore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1341: profile-photo files are written inside the import transaction;
// a row that never commits must not leave its photo file behind.

func orphanPhotoContact(t *testing.T, ext, uid, name string) MappedContact {
	t.Helper()
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(bundleTestPNG(t))
	return MappedContact{
		Ref: SourceRef{System: "mycorrhizal", ExternalID: ext},
		Record: &contactmodel.Record{Card: contactmodel.Card{
			UID:   uid,
			Name:  &contactmodel.Name{Full: name, Components: []contactmodel.NameComponent{{Kind: "given", Value: name}}},
			Media: []contactmodel.Resource{{Kind: "photo", URI: uri}},
		}},
	}
}

func dirFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func TestSourceImport_PhotoFilesFollowCommittedRows(t *testing.T) {
	t.Run("success persists the file, columns, and references it", func(t *testing.T) {
		db := dbtest.New(t)
		user := bundleTestUser(t, db, "orphan-ok")
		dir := useBundlePhotoDir(t)
		plan := &ImportSourcePlan{System: "mycorrhizal", Contacts: []MappedContact{orphanPhotoContact(t, "c1", "uid-1", "Ada")}}

		_, refs, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, nil, nil)
		require.NoError(t, err)

		var c models.Contact
		require.NoError(t, db.First(&c, refs["c1"]).Error)
		require.NotEmpty(t, c.Photo)
		assert.NotEmpty(t, c.PhotoThumbnail)
		assert.Equal(t, []string{c.Photo}, dirFiles(t, dir))
	})

	t.Run("a Create failure leaves no file, and the committed rows keep theirs", func(t *testing.T) {
		db := dbtest.New(t)
		user := bundleTestUser(t, db, "orphan-create")
		dir := useBundlePhotoDir(t)
		// Two contacts share a vcard UID: the second violates the unique
		// (user, uid) index, so its Create fails after its photo was saved.
		plan := &ImportSourcePlan{System: "mycorrhizal", Contacts: []MappedContact{
			orphanPhotoContact(t, "c1", "dup-uid", "Ada"),
			orphanPhotoContact(t, "c2", "dup-uid", "Bob"),
			orphanPhotoContact(t, "c3", "uid-3", "Cy"),
		}}

		report, refs, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, nil, nil)
		require.NoError(t, err)
		require.Equal(t, 2, report.ContactsCreated)
		require.NotContains(t, refs, "c2")

		var kept []string
		for _, ext := range []string{"c1", "c3"} {
			var c models.Contact
			require.NoError(t, db.First(&c, refs[ext]).Error)
			kept = append(kept, c.Photo)
		}
		assert.ElementsMatch(t, kept, dirFiles(t, dir), "exactly the committed rows' photos remain")
	})

	t.Run("a later-stage error rolls back and removes every file", func(t *testing.T) {
		db := dbtest.New(t)
		user := bundleTestUser(t, db, "orphan-rollback")
		dir := useBundlePhotoDir(t)
		dbtest.HideTable(t, db, "import_source_links")
		plan := &ImportSourcePlan{System: "mycorrhizal", Contacts: []MappedContact{
			orphanPhotoContact(t, "c1", "uid-1", "Ada"),
			orphanPhotoContact(t, "c2", "uid-2", "Bob"),
		}}

		_, _, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, nil, nil)
		require.Error(t, err)
		assert.Empty(t, dirFiles(t, dir))
	})

	t.Run("context cancellation after a photo was written removes it", func(t *testing.T) {
		db := dbtest.New(t)
		user := bundleTestUser(t, db, "orphan-cancel")
		dir := useBundlePhotoDir(t)
		plan := &ImportSourcePlan{System: "mycorrhizal", Contacts: []MappedContact{
			orphanPhotoContact(t, "c1", "uid-1", "Ada"),
			orphanPhotoContact(t, "c2", "uid-2", "Bob"),
		}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// progress ticks once per contact before it is processed: cancel on the
		// second tick, after contact 1's photo has been written.
		_, _, err := ExecuteSourceImportWithActions(ctx, db, user.ID, plan, nil, func(done, _ int) {
			if done == 2 {
				cancel()
			}
		})
		require.Error(t, err)
		assert.Empty(t, dirFiles(t, dir))
	})
}

func TestImportedPhotoFiles_Discard(t *testing.T) {
	w := &importedPhotoFiles{dir: t.TempDir()}
	w.discard("") // no-op
	w.track("")   // no-op
	assert.Empty(t, w.files)
	w.discard("never-tracked.jpg") // removing an absent file is fine
}

func TestRemoveContactPhoto(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/a.jpg", []byte("x"), 0o600))
	require.NoError(t, photostore.RemoveContactPhoto(dir, "a.jpg"))
	assert.Empty(t, dirFiles(t, dir))
	require.NoError(t, photostore.RemoveContactPhoto(dir, "a.jpg"), "missing file is not an error")
	require.NoError(t, photostore.RemoveContactPhoto(dir, ""))
	require.NoError(t, os.WriteFile(dir+"/keep.jpg", []byte("x"), 0o600))
	require.NoError(t, photostore.RemoveContactPhoto(dir, "../"+"keep.jpg"), "traversal is ignored")
	assert.Equal(t, []string{"keep.jpg"}, dirFiles(t, dir))
	// A non-empty directory named like a file makes os.Remove fail with a real error.
	require.NoError(t, os.MkdirAll(dir+"/sub/inner", 0o750))
	assert.Error(t, photostore.RemoveContactPhoto(dir, "sub"))
}
