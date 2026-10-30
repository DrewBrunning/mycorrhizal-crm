package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"mycorrhizal/services"

	"gorm.io/gorm"
)

// runDavDeleteRepair implements doctor -repair-dav-deletes (issue #2254): the
// operator-run repair for contacts deleted over CardDAV before #2249, which
// were soft-deleted without running the cascade or writing a trash snapshot.
//
// It follows the same conventions as runRepair: dry-run by default, refuses to
// touch a structurally corrupt database, and sets a distinct exit code when it
// does. A dry run reports the affected contacts and their leftover rows; -apply
// runs each contact's missing cascade in its own transaction; -write-snapshot
// additionally writes the best-effort (and deliberately lossy) trash snapshot.
func runDavDeleteRepair(ctx context.Context, db *gorm.DB, apply, writeSnapshot, asJSON bool, stdout, stderr io.Writer) int {
	// The cascade is destructive (hard joins) and tombstones soft rows, so the
	// same storage gate as -repair applies (issue #921).
	storage, sErr := services.RunStorageIntegrityChecks(db)
	switch {
	case sErr != nil:
		msg := fmt.Sprintf("storage integrity check could not run: %v", sErr)
		emitRepairRefusal(asJSON, stdout, msg)
		fmt.Fprintf(stderr, "doctor: refusing to repair: %s\n", msg)
		return 3
	case !storage.OK:
		detail := storage.Detail()
		msg := fmt.Sprintf("storage integrity is not OK (%s)", detail)
		emitRepairRefusal(asJSON, stdout, msg)
		fmt.Fprintf(stderr, "doctor: refusing to repair: %s — restore from a good backup before repairing\n", msg)
		return 3
	}

	report, err := services.RepairDavDeletedContacts(ctx, db, services.DavDeleteRepairOptions{
		Apply:         apply,
		WriteSnapshot: writeSnapshot,
	})
	if err != nil {
		fmt.Fprintf(stderr, "doctor: CardDAV delete repair failed: %v\n", err)
		return 1
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(report)
		if report.Failed > 0 {
			return 1
		}
		return 0
	}

	if report.DryRun {
		fmt.Fprintln(stdout, "== CardDAV delete repair DRY RUN (no changes made) ==")
	} else {
		fmt.Fprintln(stdout, "== CardDAV delete repair ==")
	}
	if len(report.Contacts) == 0 {
		fmt.Fprintln(stdout, "  nothing to repair")
		return 0
	}

	var rows int64
	for _, c := range report.Contacts {
		rows += c.Leftovers()
		fmt.Fprintf(stdout, "  user %d contact %d (deleted %s): %s, %d leftover row(s)\n",
			c.UserID, c.ContactID, c.DeletedAt.UTC().Format("2006-01-02T15:04:05Z"), c.Action, c.Leftovers())
		if detail := leftoversDetail(c); detail != "" {
			fmt.Fprintf(stdout, "    %s\n", detail)
		}
		if c.Error != "" {
			fmt.Fprintf(stdout, "    error: %s\n", c.Error)
		}
	}

	if report.DryRun {
		fmt.Fprintf(stdout, "\n%d contact(s), %d row(s) would be repaired — re-run with -apply to apply\n", len(report.Contacts), rows)
		fmt.Fprintln(stdout, "add -write-snapshot (with -apply) to also make them visible in Trash and restorable (lossy)")
		return 0
	}

	fmt.Fprintf(stdout, "\n%d contact(s) repaired, %d row(s) touched, %d snapshot(s) written\n",
		report.Cascaded, rows, report.Snapshotted)
	if writeSnapshot {
		fmt.Fprintln(stdout, "note: snapshots are lossy and best-effort; hard-join rows removed before this repair cannot be restored")
	}
	if report.Failed > 0 {
		fmt.Fprintf(stdout, "%d contact(s) failed — see the errors above\n", report.Failed)
		return 1
	}
	return 0
}

// leftoversDetail renders the per-table counts sorted, so the dry-run output is
// stable and an operator can see exactly what would change.
func leftoversDetail(c services.DavDeleteRepairContact) string {
	var parts []string
	for _, group := range []struct {
		label  string
		counts map[string]int64
	}{{"soft", c.Soft}, {"hard", c.Hard}, {"mutate", c.Mutate}} {
		keys := make([]string, 0, len(group.counts))
		for k := range group.counts {
			keys = append(keys, k)
		}
		if len(keys) == 0 {
			continue
		}
		sort.Strings(keys)
		vals := make([]string, 0, len(keys))
		for _, k := range keys {
			vals = append(vals, fmt.Sprintf("%s=%d", k, group.counts[k]))
		}
		parts = append(parts, group.label+"("+strings.Join(vals, ",")+")")
	}
	return strings.Join(parts, " ")
}
