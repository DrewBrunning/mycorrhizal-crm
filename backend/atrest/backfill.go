package atrest

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// EncryptedColumn describes one (table, column) pair whose values are
// encrypted at rest by the "encrypted"/"encryptedjson" serializers. Backfill
// must know this list because SQL cannot run AES-GCM — the data transform
// that encrypts pre-existing plaintext rows is a Go step, the same way
// RebuildSearchIndex is the Go step that rebuilds derived FTS data.
//
// IMPORTANT: keep this list in sync with the model struct tags that carry
// `serializer:encrypted` / `serializer:encryptedjson`. A column added here
// without a serializer tag (or vice versa) silently breaks one of the two
// consistency tests in backfill_test.go.
var EncryptedColumns = []string{
	"contacts.card",
	"contacts.crm",
	"contacts.passthrough",
	"contacts.how_we_met",
	"contacts.work_information",
	"contacts.contact_information",
	"life_events.description",
	"reminders.message",
	"reminder_completions.message",
	"gifts.description",
	"gifts.notes",
	"gifts.occasion",
	"preferences.value",
	"preferences.notes",
	"conversation_agenda.content",
	"audit_events.before_snapshot",
	"contact_sync_conflicts.local_value",
	"contact_sync_conflicts.remote_value",
	"contact_sync_links.synced_values",
}

// Backfill encrypts any rows whose encrypted column still holds plaintext
// (written before at-rest encryption was enabled, or in a deployment that ran
// with serializers pass-through). Idempotent and re-runnable: values that
// already carry the ciphertext prefix are left untouched, so re-running after
// an interruption simply continues. Runs inside one transaction per column so
// a failure cannot leave a half-backfilled column.
//
// Row counts are preserved by construction — this only UPDATEs existing rows,
// never inserts or deletes. The issue's "existing rows are backfilled, not
// dropped (assert row counts before/after migration)" verification is pinned
// by TestBackfill_PreservesRowCounts.
//
// Backfill is a no-op when encryption is not armed (no master key configured)
// or when the data_encryption_keys table does not exist yet.
func Backfill(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("atrest: backfill requires a db handle")
	}
	engine.mu.RLock()
	on := engine.on
	engine.mu.RUnlock()
	if !on {
		return nil
	}

	for _, spec := range EncryptedColumns {
		table, column, ok := splitSpec(spec)
		if !ok {
			return fmt.Errorf("atrest: malformed encrypted-column spec %q", spec)
		}
		if err := backfillColumn(db, table, column); err != nil {
			return fmt.Errorf("atrest: backfill %s.%s: %w", table, column, err)
		}
	}
	return nil
}

// backfillColumn encrypts one column's remaining plaintext rows in a single
// transaction. It batches by streaming the plaintext rows and updating each
// in place; the NOT LIKE guard makes it idempotent across runs.
func backfillColumn(db *gorm.DB, table, column string) error {
	// GORM's native map scan handles both uint and string PKs without needing
	// to know the table's id type up front.
	rows := []map[string]interface{}{}
	sel := db.Table(table).
		Select(fmt.Sprintf("id, %s AS value", column)).
		Where(fmt.Sprintf("%s IS NOT NULL AND %s <> '' AND %s NOT LIKE ?", column, column, column), ciphertextPrefix+"%").
		Scan(&rows)
	if sel.Error != nil {
		if isNoSuchTable(sel.Error) {
			// Table doesn't exist (fresh DB, an AutoMigrate-only test, a
			// disabled feature) — nothing to backfill.
			return nil
		}
		return sel.Error
	}

	return db.Transaction(func(tx *gorm.DB) error {
		for _, r := range rows {
			id, err := idAsString(r["id"])
			if err != nil {
				return err
			}
			value, ok := r["value"].(string)
			if !ok {
				// NULL/other driver types are skipped by the WHERE already.
				continue
			}
			ct, err := Encrypt(value)
			if err != nil {
				return err
			}
			if err := tx.Table(table).Where("id = ?", id).Update(column, ct).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// idAsString normalizes the driver's id value (int64 for INTEGER PKs, string
// for TEXT/UUID PKs) to a string usable in a WHERE id = ? clause.
func idAsString(id interface{}) (string, error) {
	switch v := id.(type) {
	case string:
		return v, nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case uint64:
		return fmt.Sprintf("%d", v), nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("unexpected id type %T", id)
	}
}

// isNoSuchTable reports whether err is SQLite's "no such table" error.
func isNoSuchTable(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no such table")
}

// splitSpec splits "table.column" into its parts.
func splitSpec(spec string) (table, column string, ok bool) {
	for i := 0; i < len(spec); i++ {
		if spec[i] == '.' {
			return spec[:i], spec[i+1:], i > 0 && i < len(spec)-1
		}
	}
	return "", "", false
}
