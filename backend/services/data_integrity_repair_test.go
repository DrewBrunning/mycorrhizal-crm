package services

import (
	"context"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// countRows is an Unscoped count (trap #6): it must see a row whether it is
// live or soft-deleted, so a repair test can prove a row is actually gone.
func countRows(t *testing.T, db *gorm.DB, table, where string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	q := db.Table(table)
	if where != "" {
		q = q.Where(where, args...)
	}
	require.NoError(t, q.Count(&n).Error)
	return n
}

func seedOrphans(t *testing.T, db *gorm.DB) (userID uint, keepEdgeID string) {
	t.Helper()
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")

	// A valid edge that must survive.
	keep := mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)

	// An edge whose target contact never existed.
	mkEdge(t, db, u.ID, a.VCardUID, "deadbeef-0000-4000-8000-000000000000", "friend_of", models.RelationshipStatusConfirmed)

	// A circle membership whose contact never existed.
	circleID := mkCircle(t, db, u.ID)
	require.NoError(t, db.Create(&models.CircleMember{
		CircleID: circleID, UserID: u.ID, MemberVCardUID: "deadbeef-0000-4000-8000-000000000001",
	}).Error)
	// A valid circle membership that must survive.
	require.NoError(t, db.Create(&models.CircleMember{
		CircleID: circleID, UserID: u.ID, MemberVCardUID: a.VCardUID,
	}).Error)

	return u.ID, keep.ID
}

func TestRepair_DryRunReportsButMutatesNothing(t *testing.T) {
	db, _ := integrityTestDB(t)
	_, _ = seedOrphans(t, db)

	edgesBefore := countRows(t, db, "relationship_edges", "")
	membersBefore := countRows(t, db, "circle_members", "")

	report, err := RepairDataIntegrity(context.Background(), db, RepairOptions{DryRun: true})
	require.NoError(t, err)
	assert.True(t, report.DryRun)
	assert.Equal(t, 2, report.TotalRows(), "1 orphan edge + 1 orphan membership")

	// Nothing was deleted.
	assert.Equal(t, edgesBefore, countRows(t, db, "relationship_edges", ""))
	assert.Equal(t, membersBefore, countRows(t, db, "circle_members", ""))

	byCheck := map[string]int{}
	for _, a := range report.Actions {
		byCheck[a.Check] = a.Deleted
	}
	assert.Equal(t, 1, byCheck["relationship_edge.endpoint_missing"])
	assert.Equal(t, 1, byCheck["circle_member.orphaned_contact"])
}

func TestRepair_ConfirmDeletesOnlyOrphans(t *testing.T) {
	db, cfg := integrityTestDB(t)
	userID, keepEdgeID := seedOrphans(t, db)

	report, err := RepairDataIntegrity(context.Background(), db, RepairOptions{DryRun: false})
	require.NoError(t, err)
	assert.False(t, report.DryRun)
	assert.Equal(t, 2, report.TotalRows())

	// The orphans are gone…
	assert.Equal(t, int64(0), countRows(t, db, "relationship_edges",
		"target_id = ?", "deadbeef-0000-4000-8000-000000000000"))
	assert.Equal(t, int64(0), countRows(t, db, "circle_members",
		"member_vcard_uid = ?", "deadbeef-0000-4000-8000-000000000001"))
	// …and the valid rows are untouched.
	assert.Equal(t, int64(1), countRows(t, db, "relationship_edges", "id = ?", keepEdgeID))
	assert.Equal(t, int64(1), countRows(t, db, "circle_members", "user_id = ? AND member_vcard_uid <> ?",
		userID, "deadbeef-0000-4000-8000-000000000001"))

	// A follow-up detection run is now clean for those classes.
	r := runDataChecks(t, db, cfg)
	_, e1 := findingFor(r, "relationship_edge.endpoint_missing", userID)
	_, e2 := findingFor(r, "circle_member.orphaned_contact", userID)
	assert.False(t, e1)
	assert.False(t, e2)
}

// A row that points at a merely SOFT-deleted contact must never be repaired —
// the contact can still be undeleted. This is the safety boundary in
// data_integrity_repair.go.
func TestRepair_LeavesSoftDeletedReferentsAlone(t *testing.T) {
	db, _ := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	edge := mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)

	circleID := mkCircle(t, db, u.ID)
	require.NoError(t, db.Create(&models.CircleMember{
		CircleID: circleID, UserID: u.ID, MemberVCardUID: b.VCardUID,
	}).Error)

	softDeleteContact(t, db, b)

	report, err := RepairDataIntegrity(context.Background(), db, RepairOptions{DryRun: false})
	require.NoError(t, err)
	assert.Equal(t, 0, report.TotalRows(), "nothing is safe to repair here")

	assert.Equal(t, int64(1), countRows(t, db, "relationship_edges", "id = ?", edge.ID))
	assert.Equal(t, int64(1), countRows(t, db, "circle_members", "member_vcard_uid = ?", b.VCardUID))
}

func TestRepair_NoOpOnHealthyDB(t *testing.T) {
	db, _ := integrityTestDB(t)
	u := mkUser(t, db, "alice")
	a := mkContact(t, db, u.ID, "A")
	b := mkContact(t, db, u.ID, "B")
	mkEdge(t, db, u.ID, a.VCardUID, b.VCardUID, "friend_of", models.RelationshipStatusConfirmed)

	report, err := RepairDataIntegrity(context.Background(), db, RepairOptions{DryRun: false})
	require.NoError(t, err)
	assert.Empty(t, report.Actions)
	assert.Equal(t, 0, report.TotalRows())
}
