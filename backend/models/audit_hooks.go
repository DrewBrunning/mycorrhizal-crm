package models

import (
	"fmt"

	"gorm.io/gorm"
)

// Audit hook methods for the audited entities (T18). Models that already
// define a hook of the same name (Contact/Activity/LifeEvent/Note/Gift) have
// the audit call folded into the existing method in their own files — GORM
// calls exactly one method per hook name per model.

// --- Activity (BeforeSave is new; AfterSave/AfterDelete fold into activity.go)

func (a *Activity) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Activity](tx, AuditEntityActivity, a.ID, a.ID == 0)
	return nil
}

// --- LifeEvent (BeforeSave is new; AfterSave/AfterDelete fold into life_event.go)

func (l *LifeEvent) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[LifeEvent](tx, AuditEntityLifeEvent, l.ID, l.ID == "")
	return nil
}

// --- Note (BeforeSave/AfterSave are new; AfterDelete folds into note.go)

func (n *Note) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Note](tx, AuditEntityNote, n.ID, n.ID == 0)
	return nil
}

func (n *Note) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityNote, fmt.Sprintf("%d", n.ID), n.UserID)
	return nil
}

// --- Gift (BeforeSave/AfterSave are new; AfterDelete folds into gift.go)

func (g *Gift) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Gift](tx, AuditEntityGift, g.ID, g.ID == "")
	return nil
}

func (g *Gift) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityGift, g.ID, g.UserID)
	return nil
}

// --- Circle (all hooks new)

func (c *Circle) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Circle](tx, AuditEntityCircle, c.ID, c.ID == "")
	return nil
}

func (c *Circle) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityCircle, c.ID, c.UserID)
	return nil
}

func (c *Circle) AfterDelete(tx *gorm.DB) error {
	auditAfterDelete(tx, AuditEntityCircle, c.ID, c.UserID, c)
	return nil
}

// --- Tag (all hooks new)

func (t *Tag) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Tag](tx, AuditEntityTag, t.ID, t.ID == "")
	return nil
}

func (t *Tag) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityTag, t.ID, t.UserID)
	return nil
}

func (t *Tag) AfterDelete(tx *gorm.DB) error {
	auditAfterDelete(tx, AuditEntityTag, t.ID, t.UserID, t)
	return nil
}

// --- Household (AfterSave/AfterDelete new; BeforeCreate already defined)

func (h *Household) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Household](tx, AuditEntityHousehold, h.ID, h.ID == "")
	return nil
}

func (h *Household) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityHousehold, h.ID, h.UserID)
	return nil
}

func (h *Household) AfterDelete(tx *gorm.DB) error {
	auditAfterDelete(tx, AuditEntityHousehold, h.ID, h.UserID, h)
	return nil
}

// --- Reminder (all hooks new)

func (r *Reminder) BeforeSave(tx *gorm.DB) error {
	auditBeforeSave[Reminder](tx, AuditEntityReminder, r.ID, r.ID == 0)
	return nil
}

func (r *Reminder) AfterSave(tx *gorm.DB) error {
	auditAfterSave(tx, AuditEntityReminder, fmt.Sprintf("%d", r.ID), r.UserID)
	return nil
}

func (r *Reminder) AfterDelete(tx *gorm.DB) error {
	auditAfterDelete(tx, AuditEntityReminder, fmt.Sprintf("%d", r.ID), r.UserID, r)
	return nil
}
