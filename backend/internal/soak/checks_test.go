package soak

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFTSCheck_DetectsDriftAndMissingTables(t *testing.T) {
	db := dbtest.New(t)
	ctx := context.Background()
	c := ftsCheck(ctx, db)
	assert.True(t, c.OK, c.Detail)

	// An orphan FTS row with no live base row is drift.
	require.NoError(t, db.Exec("INSERT INTO contacts_fts(rowid, firstname, user_id) VALUES (999, 'ghost', 1)").Error)
	c = ftsCheck(ctx, db)
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "contacts_fts has 1 rows for 0 live contacts")

	dbtest.HideTable(t, db, "notes")
	c = ftsCheck(ctx, db)
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "notes")
}

func TestFTSCheck_MissingFTSTable(t *testing.T) {
	db := dbtest.New(t)
	dbtest.HideTable(t, db, "contacts_fts")
	c := ftsCheck(context.Background(), db)
	assert.False(t, c.OK)
	assert.Contains(t, c.Detail, "contacts_fts")
}

func TestDBChecks_ReportsEachOutcome(t *testing.T) {
	db := dbtest.New(t)
	cfg := &config.Config{ProfilePhotoDir: t.TempDir(), AttachmentsDir: t.TempDir()}
	r := &Report{}
	r.dbChecks(context.Background(), db, cfg)
	assert.True(t, r.OK(), r.Failures)

	// External target: the config-dependent pass is skipped, not passed.
	r = &Report{}
	r.dbChecks(context.Background(), db, nil)
	var skipped bool
	for _, c := range r.Checks {
		skipped = skipped || c.Skipped
	}
	assert.True(t, skipped)

	// Data-integrity violation surfaces as a failure with its detail.
	require.NoError(t, db.Exec("INSERT INTO users(username, email, password) VALUES ('a','a@x.com','x')").Error)
	require.NoError(t, db.Exec("INSERT INTO contacts(user_id, firstname, vcard_uid, card) VALUES (1,'A','u-1','{bad')").Error)
	r = &Report{}
	r.dbChecks(context.Background(), db, cfg)
	assert.False(t, r.OK())
	assert.Contains(t, r.Failures[len(r.Failures)-1], "canonical_record.invalid_json")

	// A broken storage pass (closed database) is a failed check, not a panic.
	closed := dbtest.New(t)
	sqlDB, err := closed.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())
	r = &Report{}
	r.dbChecks(context.Background(), closed, cfg)
	assert.False(t, r.OK())
}

func TestSmallHelpers(t *testing.T) {
	assert.Equal(t, "abc...", truncate([]byte("abcdef"), 3))
	assert.Equal(t, "abc", truncate([]byte("abc"), 3))
	line := progressLine(90*time.Second, Snapshot{Values: map[string]float64{SigGoroutines: 14, SigWALBytes: 4 << 20}})
	assert.Contains(t, line, "goroutines=14")
	assert.Contains(t, line, "wal=4.0MiB")
}
