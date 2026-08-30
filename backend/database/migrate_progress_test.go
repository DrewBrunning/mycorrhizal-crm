package database

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/logger"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrationEmitsPerStepProgress pins issue #495 action 6 ("emit progress":
// a long migration that logs nothing is indistinguishable from a hung one).
// Every pending migration must produce a migration_step_started line before
// its body runs and a migration_step_completed line (with a duration) after it
// commits and is marked clean, both naming the NNNNNN_name.up.sql file.
func TestMigrationEmitsPerStepProgress(t *testing.T) {
	buf := captureMigrationLoggerAt(t, zerolog.InfoLevel)

	dbPath := filepath.Join(t.TempDir(), "progress.db")
	require.NoError(t, MigrateUp(dbPath))

	out := buf.String()
	require.Contains(t, out, "migration_step_started", "a fresh DB has pending migrations, so at least one start line must be emitted")
	require.Contains(t, out, "migration_step_completed", "every started migration must complete")
	require.Contains(t, out, `"migration":"000001_initial_schema.up.sql"`, "the step line must name the migration file")

	// Every migration file in the embedded set has exactly one started line and
	// one completed line, in the right order (start of N before complete of N).
	latest, err := LatestMigrationVersion()
	require.NoError(t, err)
	seen := map[uint]bool{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, `"component":"migration"`) {
			continue
		}
		var event, name string
		// Structured JSON line; extract event + migration fields.
		event = fieldValue(line, "event")
		name = fieldValue(line, "migration")
		if name == "" || !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		switch event {
		case "migration_step_started":
			assert.False(t, seen[versionFromFile(name)], "migration %s started twice", name)
			seen[versionFromFile(name)] = true
		case "migration_step_completed":
			require.True(t, seen[versionFromFile(name)], "migration %s completed before it started", name)
			seen[versionFromFile(name)] = false // completed: release the marker
		}
	}
	assert.Equal(t, int(latest), countOf(out, "migration_step_completed"),
		"one completed line per pending migration (all %d ran)", latest)

	// The completed line carries a measurable duration field.
	require.Contains(t, out, `"duration_ms":`)
}

// TestMigrationLogNameParsing covers the parse-failure branches of the
// per-step filename extraction directly: golang-migrate's LogString shape is
// "VERSION/u IDENTIFIER", and anything that does not match must degrade to ""
// rather than panic or emit a bogus name.
func TestMigrationLogNameParsing(t *testing.T) {
	assert.Equal(t, "", migrationLogName(nil))
	assert.Equal(t, "", migrationLogName([]interface{}{}))
	assert.Equal(t, "", migrationLogName([]interface{}{123}))
	assert.Equal(t, "", migrationLogName([]interface{}{"no-version-slash"}))
	assert.Equal(t, "", migrationLogName([]interface{}{"abc/up 000001_initial_schema"}))
	assert.Equal(t, "000001_initial_schema.up.sql", migrationLogName([]interface{}{"1/u 000001_initial_schema"}))
}

// captureMigrationLoggerAt is captureMigrationLogger (migrate_test.go) but at
// an explicit level, so a test can see INFO progress lines.
func captureMigrationLoggerAt(t *testing.T, level zerolog.Level) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := logger.Logger
	oldDefault := zerolog.DefaultContextLogger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(buf)
	zerolog.DefaultContextLogger = &logger.Logger
	zerolog.SetGlobalLevel(level)
	t.Cleanup(func() {
		logger.Logger = old
		zerolog.DefaultContextLogger = oldDefault
		zerolog.SetGlobalLevel(oldLevel)
	})
	return buf
}

// fieldValue extracts the "key":"value" pair from a zerolog JSON line.
func fieldValue(line, key string) string {
	token := `"` + key + `":`
	i := strings.Index(line, token)
	if i < 0 {
		return ""
	}
	rest := line[i+len(token):]
	if !strings.HasPrefix(rest, `"`) {
		return ""
	}
	rest = rest[1:]
	if j := strings.IndexByte(rest, '"'); j >= 0 {
		return rest[:j]
	}
	return ""
}

func versionFromFile(name string) uint {
	prefix, _, _ := strings.Cut(name, "_")
	var v uint
	for _, ch := range prefix {
		if ch < '0' || ch > '9' {
			return 0
		}
		v = v*10 + uint(ch-'0')
	}
	return v
}

func countOf(s, sub string) int {
	return strings.Count(s, sub)
}
