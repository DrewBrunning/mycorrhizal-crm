package models

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAuditIdentity_MatchesTheDeleteHook pins every AuditedEntity against its
// own AfterDelete hook: a single-row delete records exactly the identity
// AuditIdentity reports. RecordCascadeDelete relies on that to write the event
// a bulk cascade's per-row hook would have.
func TestAuditIdentity_MatchesTheDeleteHook(t *testing.T) {
	db := dbtest.New(t)
	u := mustUser(t, db, "identity")
	c := Contact{UserID: u.ID, Firstname: "Identity"}
	require.NoError(t, db.Create(&c).Error)

	rows := []AuditedEntity{
		&Contact{UserID: u.ID, Firstname: "Gone"},
		&Note{UserID: u.ID, ContactID: &c.ID, Content: "n"},
		&Activity{UserID: u.ID, Title: "a", Date: time.Now()},
		&LifeEvent{UserID: u.ID, EntityID: c.VCardUID, Type: "moved"},
		&Gift{UserID: u.ID, EntityID: c.VCardUID, Description: "g", Status: "idea"},
		&Circle{UserID: u.ID, Name: "circle"},
		&Tag{UserID: u.ID, Name: "tag"},
		&Household{UserID: u.ID, Name: "household"},
		&Reminder{UserID: u.ID, ContactID: &c.ID, Message: "r", Recurrence: "once", RemindAt: time.Now().Add(time.Hour)},
	}
	for _, row := range rows {
		require.NoErrorf(t, db.Create(row).Error, "%T", row)
		typ, id, uid := row.AuditIdentity()
		require.NotEmptyf(t, id, "%T: identity must be populated after create", row)
		require.NoErrorf(t, db.Delete(row).Error, "%T", row)

		var ev AuditEvent
		require.NoErrorf(t, db.Where("entity_type = ? AND operation = ?", typ, AuditOpDelete).
			Order("id desc").First(&ev).Error, "%T: its delete hook recorded no %s/delete event", row, typ)
		assert.Equalf(t, id, ev.EntityID, "%T: AuditIdentity id must equal the hook's", row)
		assert.Equalf(t, uid, ev.UserID, "%T", row)
	}
}

// TestRecordCascadeDelete_RecordsTheHookEquivalentEvent: the event written for
// a loaded row matches what a single-row delete records, snapshot included.
func TestRecordCascadeDelete_RecordsTheHookEquivalentEvent(t *testing.T) {
	db := dbtest.New(t)
	u := mustUser(t, db, "cascaderec")
	c := Contact{UserID: u.ID, Firstname: "Parent"}
	require.NoError(t, db.Create(&c).Error)
	n := Note{UserID: u.ID, ContactID: &c.ID, Content: "remember me"}
	require.NoError(t, db.Create(&n).Error)

	RecordCascadeDelete(db, &n)

	var ev AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND operation = ?", AuditEntityNote, AuditOpDelete).First(&ev).Error)
	typ, id, uid := n.AuditIdentity()
	assert.Equal(t, AuditEntityNote, typ)
	assert.Equal(t, id, ev.EntityID)
	assert.Equal(t, uid, ev.UserID)
	assert.Contains(t, ev.BeforeSnapshot, "remember me")
}
