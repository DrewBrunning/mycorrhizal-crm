package models

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"

	"github.com/stretchr/testify/assert"
)

// TestAuditRecorder_EventForDeletingUserIsQuietlyDropped pins the residual half
// of issue #1693: an event that persists after its user was hard-deleted (a
// concurrent request landing between the deletion's Flush and its commit)
// would have been removed by the audit_events ON DELETE CASCADE anyway, so for
// a user marked via NoteUserDeletion it is dropped at debug level -- not
// warned, not counted as a failed write. An unmarked missing user (the #1471
// bug class) still warns and counts; TestAuditRecorder_FailedWritesAreCounted
// pins that side.
func TestAuditRecorder_EventForDeletingUserIsQuietlyDropped(t *testing.T) {
	db := dbtest.New(t)
	rec := NewAuditRecorder(db)
	logs := logtest.Capture(t)

	NoteUserDeletion(rec, 987654)
	rec.Record(db, "auth", "x", "login", 987654, "") // user already gone
	rec.Flush()

	logs.AssertNoWarnings(t)
	assert.Zero(t, rec.(interface{ FailedWrites() int64 }).FailedWrites())
	assert.Zero(t, auditCount(t, db))

	// Marked but still existing: the discriminator requires the user to be
	// gone, so a real failure for a live user is never swallowed.
	u := mustUser(t, db, "alive")
	NoteUserDeletion(rec, u.ID)
	assert.False(t, auditActorGone(db, u.ID))
	assert.True(t, auditActorGone(db, 987654))

	// Unmarked, missing user: still a genuine failure.
	rec.Record(db, "auth", "y", "login", 555555, "")
	rec.Flush()
	assert.EqualValues(t, 1, rec.(interface{ FailedWrites() int64 }).FailedWrites())
	assert.Len(t, logs.Warnings(), 1)
}
