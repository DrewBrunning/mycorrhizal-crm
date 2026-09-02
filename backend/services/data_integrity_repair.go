package services

import (
	"context"
	"fmt"
	"sort"

	"gorm.io/gorm"
)

// Data-integrity repair (DB-01, issue #460). Deliberately a SEPARATE file and a
// SEPARATE entry point from detection (data_integrity_service.go): the
// milestone requires detection and repair to be clearly separated and
// detection to ship first (gate #538), because a repair that runs on a
// misunderstood invariant is a data-loss mechanism.
//
// What this repairs: only TRULY-ORPHANED hard-delete edge/join rows — a row
// whose referenced parent entity does not exist at all (not merely
// soft-deleted). Per ADR 0004 these rows are hard-delete, small, bounded, and
// re-derivable (a client re-pulls the collection), so removing a genuinely
// orphaned one restores the invariant with no recoverable data lost.
//
// What this NEVER touches:
//   - a row whose referent is only soft-deleted (the entity may be undeleted);
//   - Card JSON, files, or the relation-type registry (a code defect, not data);
//   - anything on the soft-delete "content the user authored" side.
//
// It is dry-run by default. The only caller that passes DryRun:false is
// cmd/doctor with an explicit -confirm flag; there is no HTTP repair route.

// RepairOptions controls a repair run.
type RepairOptions struct {
	// DryRun reports what would be deleted without deleting it. The zero value
	// is a dry run — a caller must opt in to mutation.
	DryRun bool
}

// RepairAction is the outcome for one repair class.
type RepairAction struct {
	// Check matches the detection finding's Check slug where possible.
	Check string `json:"check"`
	// Deleted is the number of rows removed (DryRun:false) or that would be
	// removed (DryRun:true).
	Deleted int `json:"deleted"`
	// Detail is a short, secret-free human message.
	Detail string `json:"detail"`
}

// RepairReport is the full repair-run outcome.
type RepairReport struct {
	DryRun  bool           `json:"dry_run"`
	Actions []RepairAction `json:"actions"`
}

// TotalRows is the sum of Deleted across every action.
func (r RepairReport) TotalRows() int {
	n := 0
	for _, a := range r.Actions {
		n += a.Deleted
	}
	return n
}

// repairClass is one orphan class: a table and the predicate that identifies a
// row whose referent is entirely gone. where is a compile-time constant from
// this file only.
type repairClass struct {
	check string
	table string
	where string
	desc  string
}

// repairClasses enumerates every safely-repairable orphan class. The NOT
// EXISTS predicates treat a soft-deleted contact as present (its row exists),
// so a row pointing at a soft-deleted entity is never matched here — matching
// detection's INV-D3 (missing) vs INV-D7 (soft-deleted) split.
func repairClasses() []repairClass {
	return []repairClass{
		{
			check: "relationship_edge.endpoint_missing",
			table: "relationship_edges",
			where: `NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = relationship_edges.source_id AND c.user_id = relationship_edges.user_id)
			     OR NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = relationship_edges.target_id AND c.user_id = relationship_edges.user_id)`,
			desc: "relationship edges with an endpoint that no longer exists",
		},
		{
			check: "circle_member.orphaned_contact",
			table: "circle_members",
			where: `NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = circle_members.member_vcard_uid AND c.user_id = circle_members.user_id)`,
			desc:  "circle memberships whose contact no longer exists",
		},
		{
			check: "household_member.orphaned_contact",
			table: "household_members",
			where: `NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = household_members.member_vcard_uid AND c.user_id = household_members.user_id)`,
			desc:  "household memberships whose contact no longer exists",
		},
		{
			check: "contact_tag.orphaned_contact",
			table: "contact_tags",
			where: `NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = contact_tags.contact_vcard_uid AND c.user_id = contact_tags.user_id)`,
			desc:  "tag assignments whose contact no longer exists",
		},
		{
			check: "field_value.orphaned",
			table: "field_values",
			where: `NOT EXISTS (SELECT 1 FROM contacts c WHERE c.vcard_uid = field_values.entity_id AND c.user_id = field_values.user_id)
			     OR NOT EXISTS (SELECT 1 FROM field_definitions d WHERE d.id = field_values.field_definition_id)`,
			desc: "field values whose contact or definition no longer exists",
		},
	}
}

// RepairDataIntegrity removes truly-orphaned hard-delete join/edge rows. With
// DryRun it only counts. All deletes run in one transaction so a mid-run
// failure leaves the database untouched (INV-A2).
func RepairDataIntegrity(ctx context.Context, db *gorm.DB, opts RepairOptions) (RepairReport, error) {
	report := RepairReport{DryRun: opts.DryRun}

	if opts.DryRun {
		for _, rc := range repairClasses() {
			var n int64
			// #nosec G201 -- rc.table/rc.where are constants from repairClasses().
			q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", rc.table, rc.where)
			if err := db.WithContext(ctx).Raw(q).Scan(&n).Error; err != nil {
				return report, fmt.Errorf("repair dry-run %s: %w", rc.table, err)
			}
			if n > 0 {
				report.Actions = append(report.Actions, RepairAction{
					Check: rc.check, Deleted: int(n),
					Detail: fmt.Sprintf("would delete %d %s", n, rc.desc),
				})
			}
		}
		sortActions(report.Actions)
		return report, nil
	}

	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, rc := range repairClasses() {
			// #nosec G201 -- rc.table/rc.where are constants from repairClasses().
			q := fmt.Sprintf("DELETE FROM %s WHERE %s", rc.table, rc.where)
			res := tx.Exec(q)
			if res.Error != nil {
				return fmt.Errorf("repair %s: %w", rc.table, res.Error)
			}
			if res.RowsAffected > 0 {
				report.Actions = append(report.Actions, RepairAction{
					Check: rc.check, Deleted: int(res.RowsAffected),
					Detail: fmt.Sprintf("deleted %d %s", res.RowsAffected, rc.desc),
				})
			}
		}
		return nil
	})
	if err != nil {
		return RepairReport{DryRun: false}, err
	}
	sortActions(report.Actions)
	return report, nil
}

func sortActions(a []RepairAction) {
	sort.Slice(a, func(i, j int) bool { return a[i].Check < a[j].Check })
}
