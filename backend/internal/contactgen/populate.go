package contactgen

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"mycorrhizal/contactmodel"
	"mycorrhizal/database"
	"mycorrhizal/models"

	"gorm.io/gorm"
)

// TB is the minimal test-helper surface the DB helpers need. Both
// *testing.T and *rapid.T satisfy it — rapid's *T is deliberately not a
// testing.TB (it lacks Name/Helper/TempDir), which is exactly why these
// helpers do not require one.
type TB interface {
	Cleanup(func())
}

// templateOnce builds one migrated template per process (the internal/dbtest
// arrangement), then hands each caller a byte copy. Re-running the full
// embedded migration set per rapid check would dominate the property tests'
// wall-clock; a sub-millisecond file copy does not.
var (
	templateOnce sync.Once
	templatePath string
	templateErr  error
)

// migratedTemplate returns the path of a fully-migrated database file,
// checkpointed so a plain copy is a complete, consistent database. Errors are
// memoized: a template build failure fails every call rather than retrying.
func migratedTemplate() (string, error) {
	templateOnce.Do(func() {
		dir, err := os.MkdirTemp("", "contactgen-template-")
		if err != nil {
			templateErr = err // # pragma: no cover — MkdirTemp fails only when /tmp is unusable
			return            // # pragma: no cover
		}
		p := filepath.Join(dir, "template.db")

		db, err := database.InitDB(p)
		if err != nil {
			templateErr = err // # pragma: no cover — the embedded migration set is exercised elsewhere; failure here means a broken schema
			return            // # pragma: no cover
		}
		sqlDB, err := db.DB()
		if err != nil {
			templateErr = err // # pragma: no cover — the connection was just opened
			return            // # pragma: no cover
		}
		if _, err := sqlDB.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			templateErr = err // # pragma: no cover — the file was just created
			return            // # pragma: no cover
		}
		if err := sqlDB.Close(); err != nil {
			templateErr = err // # pragma: no cover — the handle was just opened
			return            // # pragma: no cover
		}
		templatePath = p
	})
	if templateErr != nil {
		return "", fmt.Errorf("contactgen: building migrated template database: %w", templateErr) // # pragma: no cover — any failure above is an OS/invariant break, memoized and surfaced here
	}
	return templatePath, nil
}

// MigratedDB returns an isolated, fully-migrated database plus its file path
// for one generative check. The temp directory and the gorm.DB connection are
// cleaned up via t.Cleanup, so a property that draws a fresh database per
// check cannot accumulate files. The path is returned because the migration
// property needs to run MigrateUp against the file, not just query it.
func MigratedDB(t TB) (*gorm.DB, string, error) {
	src, err := migratedTemplate()
	if err != nil {
		return nil, "", err // # pragma: no cover — a template build failure is an OS/invariant break (migratedTemplate memoizes it)
	}

	dir, err := os.MkdirTemp("", "contactgen-db-")
	if err != nil {
		return nil, "", fmt.Errorf("contactgen: creating test db dir: %w", err) // # pragma: no cover — MkdirTemp fails only when /tmp is unusable
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	path := filepath.Join(dir, "x.db")
	if err := copyFile(src, path); err != nil {
		return nil, "", fmt.Errorf("contactgen: copying migrated template: %w", err) // # pragma: no cover — the dst dir was just created and src was just produced
	}

	db, err := database.OpenMigratedFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("contactgen: opening migrated database: %w", err) // # pragma: no cover — the file is a byte copy of a just-migrated template
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db, path, nil
}

// copyFile copies src to dst (test-only helper; neither path is request
// input — the template and t-scoped temp dir are created by this package).
func copyFile(src, dst string) error {
	in, err := os.Open(src) // #nosec G304 -- src is this package's own migrated template
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst) // #nosec G304 -- dst is a t-scoped MkdirTemp path
	if err != nil {
		return err // # pragma: no cover — dst is a fresh path in a just-created dir
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close() // # pragma: no cover — both handles are open local files
		return err      // # pragma: no cover
	}
	return out.Close()
}

// Populate inserts generated records as contacts for userID through the same
// code path the REST API uses (ApplyRecordToContact, then Create — CLAUDE.md
// backend trap #2), so the search/migration/idempotency properties operate on
// exactly the persistence shape the application produces. It returns the
// created contact rows.
func Populate(db *gorm.DB, userID uint, recs []*contactmodel.Record) ([]models.Contact, error) {
	contacts := make([]models.Contact, 0, len(recs))
	for i, rec := range recs {
		c := models.Contact{UserID: userID}
		models.ApplyRecordToContact(&c, rec, "")
		if err := db.Create(&c).Error; err != nil {
			return nil, fmt.Errorf("contactgen: creating generated contact %d: %w", i, err)
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

// NewUser creates a fresh user row for generated contacts to scope to.
func NewUser(db *gorm.DB, label string) (models.User, error) {
	u := models.User{Username: "gen-" + label, Email: "gen-" + label + "@example.com", Password: "x"}
	if err := db.Create(&u).Error; err != nil {
		return models.User{}, fmt.Errorf("contactgen: creating user: %w", err)
	}
	return u, nil
}
