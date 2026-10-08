package models

import (
	"fmt"

	"gorm.io/gorm"
)

// AuditedEntity is implemented by every model whose deletes are audited by an
// AfterDelete hook. AuditIdentity returns exactly the (entity type, entity id,
// user id) that hook records, so code that deletes such rows without firing a
// per-row hook can record the same event (RecordCascadeDelete).
//
// TestAuditIdentity_MatchesTheDeleteHook pins each implementation against its
// hook; a new audited model must implement this too, or a cascade that bulk-
// deletes it leaves its audit history without a delete (issue #1471 follow-up).
type AuditedEntity interface {
	AuditIdentity() (entityType, entityID string, userID uint)
}

func (c *Contact) AuditIdentity() (string, string, uint) {
	return AuditEntityContact, c.VCardUID, c.UserID
}

func (n *Note) AuditIdentity() (string, string, uint) {
	return AuditEntityNote, fmt.Sprintf("%d", n.ID), n.UserID
}

func (a *Activity) AuditIdentity() (string, string, uint) {
	return AuditEntityActivity, a.UUID, a.UserID
}

func (l *LifeEvent) AuditIdentity() (string, string, uint) {
	return AuditEntityLifeEvent, l.ID, l.UserID
}

func (g *Gift) AuditIdentity() (string, string, uint) {
	return AuditEntityGift, g.ID, g.UserID
}

func (c *Circle) AuditIdentity() (string, string, uint) {
	return AuditEntityCircle, c.ID, c.UserID
}

func (t *Tag) AuditIdentity() (string, string, uint) {
	return AuditEntityTag, t.ID, t.UserID
}

func (h *Household) AuditIdentity() (string, string, uint) {
	return AuditEntityHousehold, h.ID, h.UserID
}

func (r *Reminder) AuditIdentity() (string, string, uint) {
	return AuditEntityReminder, fmt.Sprintf("%d", r.ID), r.UserID
}

// RecordCascadeDelete records the delete audit event that a per-row
// tx.Delete(row) would have recorded through row's AfterDelete hook — the same
// entity identity and the same redacted before-snapshot. A cascade deletes
// children with one bulk statement, whose hook fires once on a zero-value
// model and is dropped (skipZeroIdentityAudit); without this, every cascaded
// child's audit history would end without a delete, as if it still existed.
// row must be the loaded row as it was before the delete.
func RecordCascadeDelete(tx *gorm.DB, row AuditedEntity) {
	entityType, entityID, userID := row.AuditIdentity()
	auditAfterDelete(tx, entityType, entityID, userID, row)
}
