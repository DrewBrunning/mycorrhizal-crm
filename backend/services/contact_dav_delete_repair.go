package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Repair for contacts deleted over CardDAV before issue #2249 (issue #2254).
//
// Before #2249 a CardDAV DELETE ran a bare db.Delete(&contact): the contact row
// was soft-deleted but the ContactCascadeRegistry never ran and no
// ContactDeletionSnapshot was written (ADR 0037). Their soft-cascade dependants
// stayed live, hard-join rows (edges, circle members, tags, ...) stayed behind,
// and the contact is invisible to ListTrash and unrestorable.
//
// There is no reliable "deleted over DAV" marker, so the detection criterion is
// exactly: a soft-deleted contact with no contact_deletion_snapshots row for
// (user_id, contact_id). That also matches a contact deleted through the REST
// API before ADR 0037 shipped (migration 000081); its cascade already ran, so
// it reports no leftovers and is handled identically (equally unrestorable).
//
// Following ADR 0012's repair conventions this is operator-run only (the
// doctor CLI), dry-run by default, never run at startup, and it never touches
// a live contact or a snapshot-backed trashed contact.

// DavDeleteRepairOptions selects the repair mode.
type DavDeleteRepairOptions struct {
	// Apply performs the missing cascade; false is a dry run (no writes).
	Apply bool
	// WriteSnapshot additionally writes a best-effort deletion snapshot so the
	// contact appears in Trash and restores. Requires Apply.
	WriteSnapshot bool
	// Now stamps the notes updated_at bump and the feed revocation. Soft
	// cascade deleted_at always uses the contact's own deleted_at, never Now.
	// Zero means time.Now().
	Now time.Time
}

// DavDeleteRepairContact is one affected contact.
type DavDeleteRepairContact struct {
	UserID    uint      `json:"user_id"`
	ContactID uint      `json:"contact_id"`
	DeletedAt time.Time `json:"deleted_at"`
	// Soft counts live soft-cascade dependants per table.
	Soft map[string]int64 `json:"soft,omitempty"`
	// Hard counts remaining hard-cascade rows per table.
	Hard map[string]int64 `json:"hard,omitempty"`
	// Mutate counts un-run mutate steps ("users" self pointer, "feeds").
	Mutate map[string]int64 `json:"mutate,omitempty"`
	// Action is what happened (or, in a dry run, would happen): "cascade",
	// "cascade+snapshot", "snapshot", "none" (nothing to do), or "failed".
	Action string `json:"action"`
	Error  string `json:"error,omitempty"`
}

// Leftovers is the total number of rows the missing cascade would touch.
func (c DavDeleteRepairContact) Leftovers() int64 {
	var n int64
	for _, m := range []map[string]int64{c.Soft, c.Hard, c.Mutate} {
		for _, v := range m {
			n += v
		}
	}
	return n
}

// DavDeleteRepairReport is the result of one run.
type DavDeleteRepairReport struct {
	DryRun        bool                     `json:"dry_run"`
	WriteSnapshot bool                     `json:"write_snapshot"`
	Contacts      []DavDeleteRepairContact `json:"contacts"`
	Cascaded      int                      `json:"cascaded"`
	Snapshotted   int                      `json:"snapshotted"`
	Failed        int                      `json:"failed"`
}

// FindTrashedContactsWithoutSnapshot returns every soft-deleted contact with no
// deletion snapshot, with per-table leftover counts. Read-only.
func FindTrashedContactsWithoutSnapshot(db *gorm.DB) ([]DavDeleteRepairContact, error) {
	var contacts []models.Contact
	if err := db.Unscoped().
		Where("deleted_at IS NOT NULL AND NOT EXISTS (SELECT 1 FROM contact_deletion_snapshots s WHERE s.user_id = contacts.user_id AND s.contact_id = contacts.id)").
		Order("user_id, id").Find(&contacts).Error; err != nil {
		return nil, err
	}
	out := make([]DavDeleteRepairContact, 0, len(contacts))
	for _, c := range contacts {
		rc, err := davRepairInspect(db, c)
		if err != nil {
			return nil, err
		}
		out = append(out, rc)
	}
	return out, nil
}

func addCount(m *map[string]int64, table string, n int64) {
	if n == 0 {
		return
	}
	if *m == nil {
		*m = map[string]int64{}
	}
	(*m)[table] = n
}

func davRepairInspect(db *gorm.DB, c models.Contact) (DavDeleteRepairContact, error) {
	rc := DavDeleteRepairContact{UserID: c.UserID, ContactID: c.ID, DeletedAt: c.DeletedAt.Time}
	target := ContactCascadeTarget{ID: c.ID, VCardUID: c.VCardUID, UserID: c.UserID}
	for _, step := range ContactCascadeRegistry() {
		var n int64
		var err error
		switch step.Mode {
		case CascadeSoft:
			query, args := step.Match(target)
			err = db.Table(step.Table).Where(query, args...).Where("deleted_at IS NULL").Count(&n).Error
			addCount(&rc.Soft, step.Table, n)
		case CascadeHard:
			query, args := step.Match(target)
			err = db.Table(step.Table).Where(query, args...).Count(&n).Error
			addCount(&rc.Hard, step.Table, n)
		case CascadeMutate:
			switch step.Table {
			case "users":
				err = db.Model(&models.User{}).Where("id = ? AND self_contact_vcard_uid = ?", c.UserID, c.VCardUID).Count(&n).Error
			case "feeds":
				err = db.Model(&models.Feed{}).
					Where("user_id = ? AND kind = ? AND entity_id = ? AND revoked_at IS NULL", c.UserID, models.FeedKindContact, c.VCardUID).Count(&n).Error
			}
			addCount(&rc.Mutate, step.Table, n)
		}
		if err != nil {
			return rc, fmt.Errorf("inspect %s: %w", step.Table, err)
		}
	}
	return rc, nil
}

type davRepairTally struct{ contacts, rows, snaps int64 }

// RepairDavDeletedContacts finds the snapshot-less soft-deleted contacts and,
// with opts.Apply, runs each one's missing cascade in its own transaction.
// Idempotent: a repaired contact has no leftovers, and one given a snapshot is
// no longer a candidate. A per-contact failure rolls only that contact back and
// is reported; the remaining contacts are still processed.
//
// Operational events (ADR 0005, system_events) record each user's outcome. The
// append-only audit trail is deliberately not written: this is an operator
// repair of an earlier incomplete delete, not a user edit.
func RepairDavDeletedContacts(ctx context.Context, db *gorm.DB, opts DavDeleteRepairOptions) (*DavDeleteRepairReport, error) {
	if opts.WriteSnapshot && !opts.Apply {
		return nil, errors.New("write-snapshot requires apply")
	}
	now := opts.Now
	if now.IsZero() {
		now = Now().UTC()
	}
	found, err := FindTrashedContactsWithoutSnapshot(db)
	if err != nil {
		return nil, err
	}
	report := &DavDeleteRepairReport{DryRun: !opts.Apply, WriteSnapshot: opts.WriteSnapshot, Contacts: []DavDeleteRepairContact{}}

	ok := map[uint]*davRepairTally{}
	failed := map[uint]string{}
	for _, rc := range found {
		clean := rc.Leftovers() == 0
		switch {
		case !clean && opts.WriteSnapshot:
			rc.Action = "cascade+snapshot"
		case !clean:
			rc.Action = "cascade"
		case opts.WriteSnapshot:
			rc.Action = "snapshot"
		default:
			rc.Action = "none"
		}
		if opts.Apply && rc.Action != "none" {
			if err := repairOneDavDeletedContact(db, rc, opts.WriteSnapshot, now); err != nil {
				rc.Action, rc.Error = "failed", err.Error()
				report.Failed++
				failed[rc.UserID] = err.Error()
			} else {
				t := ok[rc.UserID]
				if t == nil {
					t = &davRepairTally{}
					ok[rc.UserID] = t
				}
				t.contacts++
				t.rows += rc.Leftovers()
				if !clean {
					report.Cascaded++
				}
				if opts.WriteSnapshot {
					report.Snapshotted++
					t.snaps++
				}
			}
		}
		report.Contacts = append(report.Contacts, rc)
	}

	if opts.Apply {
		recordDavRepairEvents(ctx, db, ok, failed)
	}
	return report, nil
}

// recordDavRepairEvents writes one low-cardinality operational event per user
// (counts only, never a contact id: ADR 0005). It reuses the job_completed /
// job_failed vocabulary because system_events.event_type is a closed CHECK set
// and this repair needs no schema change.
func recordDavRepairEvents(ctx context.Context, db *gorm.DB, ok map[uint]*davRepairTally, failed map[uint]string) {
	okUsers := make([]uint, 0, len(ok))
	for u := range ok {
		okUsers = append(okUsers, u)
	}
	sort.Slice(okUsers, func(i, j int) bool { return okUsers[i] < okUsers[j] })
	for _, u := range okUsers {
		uid, t := u, ok[u]
		result := "success"
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType: models.SysEventJobCompleted, Component: "contact_dav_delete_repair", Operation: "repair",
			Result: &result, UserID: &uid,
			Detail: fmt.Sprintf("contacts=%d rows=%d snapshots=%d", t.contacts, t.rows, t.snaps),
		})
	}
	failedUsers := make([]uint, 0, len(failed))
	for u := range failed {
		failedUsers = append(failedUsers, u)
	}
	sort.Slice(failedUsers, func(i, j int) bool { return failedUsers[i] < failedUsers[j] })
	for _, u := range failedUsers {
		uid := u
		result := "failure"
		models.RecordSystemEvent(ctx, db, models.SystemEvent{
			EventType: models.SysEventJobFailed, Component: "contact_dav_delete_repair", Operation: "repair",
			Result: &result, UserID: &uid, Error: failed[u],
		})
	}
}

// repairOneDavDeletedContact runs the missing cascade (and optionally writes the
// snapshot) for one contact in a single transaction. The candidate is re-checked
// inside the transaction so a concurrent restore, purge or snapshot is skipped.
func repairOneDavDeletedContact(db *gorm.DB, rc DavDeleteRepairContact, writeSnapshot bool, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var contact models.Contact
		err := tx.Unscoped().Where("id = ? AND user_id = ? AND deleted_at IS NOT NULL", rc.ContactID, rc.UserID).First(&contact).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // restored or purged since the scan: nothing to repair
		}
		if err != nil {
			return err
		}
		var snaps int64
		if err := tx.Model(&models.ContactDeletionSnapshot{}).Where("user_id = ? AND contact_id = ?", rc.UserID, rc.ContactID).Count(&snaps).Error; err != nil {
			return err
		}
		if snaps > 0 {
			return nil // became snapshot-backed since the scan: not ours to touch
		}

		// Capture BEFORE the cascade, exactly as DeleteContact does: the live
		// soft rows are the ones this repair tombstones, and the remaining
		// hard rows are the ones it removes. Rows removed before the repair
		// are gone and cannot be captured (the snapshot is lossy by design).
		var payload ContactDeletionPayload
		if writeSnapshot {
			if payload, err = captureContactDeletion(tx, contact, rc.UserID); err != nil {
				return err
			}
		}

		target := ContactCascadeTarget{ID: contact.ID, VCardUID: contact.VCardUID, UserID: rc.UserID}
		for _, step := range ContactCascadeRegistry() {
			if step.Mode == CascadeSoft {
				// Tombstone with the contact's own deleted_at so retention and
				// the batch marker line up; GORM's Delete would stamp now.
				query, args := step.Match(target)
				if err := tx.Table(step.Table).Where(query, args...).Where("deleted_at IS NULL").
					UpdateColumn("deleted_at", contact.DeletedAt.Time).Error; err != nil {
					return err
				}
				if step.After != nil {
					if err := step.After(tx, target, now); err != nil {
						return err
					}
				}
				continue
			}
			if err := runCascadeStep(tx, step, target, now); err != nil {
				return err
			}
		}

		if !writeSnapshot {
			return nil
		}
		return writeContactDeletionSnapshot(tx, contact, rc.UserID, contact.DeletedAt.Time, payload)
	})
}
