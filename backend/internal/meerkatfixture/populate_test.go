package meerkatfixture

import (
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/meerkat"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPopulate_ProducesAReadableMeerkatDatabase(t *testing.T) {
	m, err := Read()
	require.NoError(t, err)

	path := filepath.Join(t.TempDir(), "meerkat.db")
	require.NoError(t, Populate(path, m))

	// The production reader must consume the built file exactly like a real
	// deployment's database.
	snap, err := meerkat.Open(path)
	require.NoError(t, err)

	require.Len(t, snap.Contacts, 5)
	assert.Equal(t, "Ada", *snap.Contacts[0].Firstname)
	assert.Equal(t, `["Family","Book Club"]`, *snap.Contacts[0].CirclesJSON)
	assert.Equal(t, "photos/ada.jpg", *snap.Contacts[0].Photo)
	assert.Len(t, snap.Relationships, 6)
	assert.Len(t, snap.Notes, 3)
	assert.Len(t, snap.Activities, 1)
	assert.Len(t, snap.ActivityContacts, 2)
	assert.Len(t, snap.Reminders, 2)
	assert.Equal(t, 2, snap.SourceUserCount, "the fixture carries a second user for the multi-user filter")
}

func TestPopulate_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.db")
	require.NoError(t, os.WriteFile(path, []byte("already here"), 0o644))

	m, err := Read()
	require.NoError(t, err)
	err = Populate(path, m)
	assert.Error(t, err, "Populate must refuse to clobber an existing file")
}
