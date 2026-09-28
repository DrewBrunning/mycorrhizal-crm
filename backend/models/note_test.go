package models

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteBeforeCreateGeneratesUUID covers the stable portable UUID added for
// the account bundle (issue #1260, migration 000067): a new note gets one when
// the caller supplies none.
func TestNoteBeforeCreateGeneratesUUID(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "note-uuid", Password: "x", Email: "note-uuid@example.com"}
	require.NoError(t, db.Create(&user).Error)

	note := Note{UserID: user.ID, Content: "hello", Date: time.Now()}
	require.NoError(t, db.Create(&note).Error)
	assert.NotEmpty(t, note.UUID, "BeforeCreate must mint a stable UUID")
}

func TestNoteBeforeCreatePreservesExplicitUUID(t *testing.T) {
	t.Parallel()
	db := dbtest.New(t)
	user := User{Username: "note-uuid-2", Password: "x", Email: "note-uuid-2@example.com"}
	require.NoError(t, db.Create(&user).Error)

	note := Note{UUID: "explicit-note-uuid", UserID: user.ID, Content: "hello", Date: time.Now()}
	require.NoError(t, db.Create(&note).Error)
	assert.Equal(t, "explicit-note-uuid", note.UUID)
}
