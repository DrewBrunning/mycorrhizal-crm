package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"mycorrhizal/logger"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Denormalized contact-column rebuild (issue #497). The flat contacts.*
// projection scalars, contacts.sort_name, contacts.addresses_flat and
// contacts.phones_normalized are derived data — a deterministic projection of
// the authoritative nested Card, kept live by BeforeSave. This is their
// standalone rebuild, the flat-column analogue of RebuildSearchIndex for the
// FTS5 index: it re-derives every live contact's denormalized columns through
// models.Contact.RederiveDenormalized (the same deriveDenormalized the write
// path runs) and writes back only the rows that had drifted.
//
// Run it after any write that bypassed the GORM hooks — a bulk import that
// INSERTs contact rows directly, a hand-written raw-SQL migration that touched
// a base column, a restore from a backup — exactly the situations the search
// index backfill exists for. On a faithful database it is a no-op: the flat
// projection is a fixpoint of a plain re-save (ADR 0012 INV-A5 / INV-D8).
//
// Idempotent and re-runnable. Unlike the FTS rebuild it does NOT run in one
// big transaction: it commits page by page, so a huge instance never holds
// SQLite's single write lock for the whole run and an interrupted rebuild
// resumes cleanly on the next invocation (each row's update is a fixpoint).

// derivedColumnsRebuildPageSize is how many contacts one rebuild transaction
// loads, re-derives, and writes back. A page is one write-lock acquisition.
const derivedColumnsRebuildPageSize = 500

// DerivedColumnsRebuildStats reports what a rebuild did: how many live
// contacts it scanned, how many it had to rewrite, and — for diagnostics —
// how many rewrites touched each column.
type DerivedColumnsRebuildStats struct {
	ContactsScanned int64            `json:"contacts_scanned"`
	ContactsUpdated int64            `json:"contacts_updated"`
	ColumnUpdates   map[string]int64 `json:"column_updates,omitempty"`
}

// derivedColumnsRebuildMu serialises RebuildDerivedContactColumnsExclusive
// within a single process, the same guard RebuildSearchIndexExclusive uses
// and for the same reason: SQLite already serialises the writes, this only
// spares a redundant second full pass racing the first (an operator
// double-submitting the endpoint, or a post-restore rebuild overlapping a
// manual one).
var derivedColumnsRebuildMu sync.Mutex

// RebuildDerivedContactColumns re-derives every live contact's denormalized
// columns and persists the rows that had drifted. See the package comment for
// when to run it. The context bounds the run — a cancelled context stops it
// between pages, leaving every page it already committed correct.
func RebuildDerivedContactColumns(ctx context.Context, db *gorm.DB) (DerivedColumnsRebuildStats, error) {
	stats := DerivedColumnsRebuildStats{ColumnUpdates: map[string]int64{}}
	start := time.Now()
	var lastID uint

	for {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		var ids []uint
		if err := db.WithContext(ctx).Raw(
			`SELECT id FROM contacts WHERE deleted_at IS NULL AND id > ? ORDER BY id LIMIT ?`,
			lastID, derivedColumnsRebuildPageSize,
		).Scan(&ids).Error; err != nil {
			return stats, fmt.Errorf("rebuild derived contact columns (page): %w", err)
		}
		if len(ids) == 0 {
			break
		}
		lastID = ids[len(ids)-1]

		var page []models.Contact
		if err := db.WithContext(ctx).Where("id IN ?", ids).Order("id").Find(&page).Error; err != nil {
			// A row whose card is unreadable (corrupt JSON, or ciphertext the
			// key can't open) fails the batch decode — an INV-D8 violation the
			// integrity checker reports separately. Fall back to per-row loads
			// so the rest of the page still gets rebuilt.
			page = page[:0]
			for _, id := range ids {
				var one models.Contact
				if err := db.WithContext(ctx).First(&one, id).Error; err != nil {
					logger.Warn().Uint("contact_id", id).Err(err).
						Msg("derived columns rebuild: skipping contact whose canonical record will not load")
					continue
				}
				page = append(page, one)
			}
		}

		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for i := range page {
				c := &page[i]
				stats.ContactsScanned++

				fixes := c.RecomputeDerivedColumns()
				if len(fixes) == 0 {
					continue
				}
				cols := make(map[string]any, len(fixes))
				for _, f := range fixes {
					cols[f.Column] = f.Want
				}
				// UpdateColumns skips the GORM hooks — this is not a user edit,
				// so no BeforeSave re-entry, no audit event, no etag bump, no
				// updated_at churn (raw migration 000022 skips the hooks for the
				// same reason). The contacts_fts triggers still fire on the
				// UPDATE, so the searchable columns propagate to the index.
				if err := tx.Model(&models.Contact{}).
					Where("id = ?", c.ID).
					UpdateColumns(cols).Error; err != nil {
					return fmt.Errorf("rebuild derived contact columns (write id=%d): %w", c.ID, err)
				}
				stats.ContactsUpdated++
				for _, f := range fixes {
					stats.ColumnUpdates[f.Column]++
				}
			}
			return nil
		})
		if err != nil {
			return stats, err
		}

		if len(ids) < derivedColumnsRebuildPageSize {
			break
		}
	}

	if len(stats.ColumnUpdates) == 0 {
		stats.ColumnUpdates = nil
	}
	logger.Info().
		Int64("contacts_scanned", stats.ContactsScanned).
		Int64("contacts_updated", stats.ContactsUpdated).
		Str("columns", summarizeColumnUpdates(stats.ColumnUpdates)).
		Int64("duration_ms", time.Since(start).Milliseconds()).
		Msg("derived contact columns rebuilt")
	return stats, nil
}

// RebuildDerivedContactColumnsExclusive is RebuildDerivedContactColumns
// behind the in-process concurrency guard. It returns ErrJobSkipped without
// touching a row when another rebuild is already running in this process, so
// an operator-facing caller reports "already in progress" instead of starting
// a redundant second pass.
func RebuildDerivedContactColumnsExclusive(ctx context.Context, db *gorm.DB) (DerivedColumnsRebuildStats, error) {
	if !derivedColumnsRebuildMu.TryLock() {
		return DerivedColumnsRebuildStats{}, ErrJobSkipped
	}
	defer derivedColumnsRebuildMu.Unlock()
	return RebuildDerivedContactColumns(ctx, db)
}

// summarizeColumnUpdates renders the per-column rewrite counts as a stable,
// low-cardinality "col=n col=n" string for a log line.
func summarizeColumnUpdates(m map[string]int64) string {
	if len(m) == 0 {
		return ""
	}
	cols := make([]string, 0, len(m))
	for col := range m {
		cols = append(cols, col)
	}
	sort.Strings(cols)
	parts := make([]string, 0, len(cols))
	for _, col := range cols {
		parts = append(parts, fmt.Sprintf("%s=%d", col, m[col]))
	}
	return strings.Join(parts, " ")
}
