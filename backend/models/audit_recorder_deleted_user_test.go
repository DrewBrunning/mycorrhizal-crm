package models

import (
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/logtest"

	"github.com/stretchr/testify/assert"
)

// TestAuditRecorder_EventForDeletedUserIsQuietlyDropped pins the residual half
// of issue #1693: an event that persists after its user was hard-deleted (a
// concurrent request landing between the deletion's Flush and its commit)
// would have been removed by the audit_events ON DELETE CASCADE anyway, so it
// is dropped at debug level -- not warned, not counted as a failed write.
func TestAuditRecorder_EventForDeletedUserIsQuietlyDropped(t *testing.T) {
	db := dbtest.New(t)
	rec := NewAuditRecorder(db)
	logs := logtest.Capture(t)

	rec.Record(db, "auth", "x", "login", 987654, "") // no such user
	rec.Flush()

	logs.AssertNoWarnings(t)
	assert.Zero(t, rec.(interface{ FailedWrites() int64 }).FailedWrites())
	assert.Zero(t, auditCount(t, db))

	u := mustUser(t, db, "alive")
	assert.False(t, auditActorGone(db, u.ID))
	assert.True(t, auditActorGone(db, 987654))
}
