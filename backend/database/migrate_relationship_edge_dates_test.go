package database

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigration000079_RelationshipEdgeDatesBackfill proves issue #2001's data
// guarantee: existing edges (with and without metadata since/until) survive
// 000079, valid since/until strings are promoted to typed PartialDate columns,
// everything else stays NULL, metadata is never altered, and the down
// migration drops only the new columns.
func TestMigration000079_RelationshipEdgeDatesBackfill(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "edge-dates.db")
	require.NoError(t, MigrateUpTo(dbPath, 78))

	sqlDB, err := sql.Open("sqlite", openDSN(dbPath))
	require.NoError(t, err)
	defer sqlDB.Close()

	_, err = sqlDB.Exec(`INSERT INTO users (username, email, password, created_at, updated_at)
		VALUES ('u', 'u@example.com', 'x', datetime('now'), datetime('now'))`)
	require.NoError(t, err)

	rows := []struct{ id, meta string }{
		{"full", `{"since":"2010-05-17","until":"2018-06"}`},
		{"yearonly", `{"since":"2001","until":"2020"}`},
		{"reversed", `{"since":"2019-01-01","until":"2018-01-01"}`},
		{"garbage", `{"since":"last summer","until":"2024-13-45","kind":"chosen"}`},
		{"nometa", ``},
		{"jsonnull", `null`},
		{"unrelated", `{"kind":"biological"}`},
	}
	for i, r := range rows {
		var meta any
		if r.meta != "" {
			meta = r.meta
		}
		_, err = sqlDB.Exec(`INSERT INTO relationship_edges
			(id, created_at, updated_at, user_id, source_id, target_id, type, directional, metadata, source, confidence, status, sensitivity)
			VALUES (?, datetime('now'), datetime('now'), 1, ?, ?, 'friend_of', 0, ?, 'user-confirmed', 1, 'confirmed', 'normal')`,
			r.id, "11111111-1111-4111-8111-11111111111"+string(rune('0'+i)), "22222222-2222-4222-8222-222222222222", meta)
		require.NoError(t, err, r.id)
	}
	require.NoError(t, MigrateUpTo(dbPath, 79))

	type got struct{ start, end, meta sql.NullString }
	read := func(id string) got {
		var g got
		require.NoError(t, sqlDB.QueryRow(`SELECT start_date, end_date, metadata FROM relationship_edges WHERE id = ?`, id).
			Scan(&g.start, &g.end, &g.meta))
		return g
	}

	var n int
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM relationship_edges`).Scan(&n))
	assert.Equal(t, len(rows), n, "no edge may be lost")

	g := read("full")
	assert.JSONEq(t, `{"year":2010,"month":5,"day":17}`, g.start.String)
	assert.JSONEq(t, `{"year":2018,"month":6}`, g.end.String)
	g = read("yearonly")
	assert.JSONEq(t, `{"year":2001}`, g.start.String)
	assert.JSONEq(t, `{"year":2020}`, g.end.String)

	g = read("reversed")
	assert.True(t, g.start.Valid)
	assert.False(t, g.end.Valid, "a reversed pair must not violate end >= start; until stays in metadata")
	assert.JSONEq(t, `{"since":"2019-01-01","until":"2018-01-01"}`, g.meta.String)

	g = read("garbage")
	assert.False(t, g.start.Valid)
	assert.False(t, g.end.Valid)
	assert.JSONEq(t, `{"since":"last summer","until":"2024-13-45","kind":"chosen"}`, g.meta.String)

	for _, id := range []string{"nometa", "jsonnull", "unrelated"} {
		g = read(id)
		assert.False(t, g.start.Valid, id)
		assert.False(t, g.end.Valid, id)
	}
	g = read("full")
	assert.JSONEq(t, `{"since":"2010-05-17","until":"2018-06"}`, g.meta.String, "metadata must be left untouched")

	// Down migration: columns gone, rows and metadata intact.
	require.NoError(t, MigrateDown(dbPath))
	_, err = sqlDB.Exec(`SELECT start_date FROM relationship_edges`)
	require.Error(t, err)
	require.NoError(t, sqlDB.QueryRow(`SELECT COUNT(*) FROM relationship_edges`).Scan(&n))
	assert.Equal(t, len(rows), n)
}
