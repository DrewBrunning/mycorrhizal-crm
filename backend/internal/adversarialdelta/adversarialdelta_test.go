package adversarialdelta

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectSurfaces(t *testing.T) {
	changed := []string{
		"backend/routes/routes.go",
		"backend/database/migrations/000072_new.up.sql",
		"backend/services/immich_client.go",
		"backend/integrations/registry.go",
		"backend/middleware/auth.go",
		"backend/internal/releasenotes/releasenotes.go",
		"docs/readme.md",
		"",
	}
	got := DetectSurfaces(changed)
	assert.Equal(t, []string{SurfaceAuthPath, SurfaceOutboundClient, SurfacePersistence, SurfaceRoute}, got)
}

func TestDetectSurfacesAuthFilenames(t *testing.T) {
	assert.Equal(t, []string{SurfaceAuthPath}, DetectSurfaces([]string{"backend/controllers/device_grant_service.go"}))
	assert.Equal(t, []string{SurfaceAuthPath}, DetectSurfaces([]string{"backend/services/two_factor_replay_test.go"}))
	assert.Empty(t, DetectSurfaces([]string{"backend/services/contact_sync_service.go"}))
}

const ledgerWithRow = `# deltas

| Release | Date | Surface | Coverage |
|---|---|---|---|
| v0.8.8 | 2026-09-18 | none | No new class of security-relevant surface. |
| v0.8.9 | 2026-09-20 | route, auth-path | authorization matrix and the session-minting route gate. |
`

func TestEvaluateNoSurfaces(t *testing.T) {
	v := Evaluate([]byte(ledgerWithRow), []string{"docs/readme.md"}, "v0.9.0")
	assert.Empty(t, v.Surfaces)
	assert.True(t, v.OK())
}

func TestEvaluateRowCoversSurfaces(t *testing.T) {
	v := Evaluate([]byte(ledgerWithRow), []string{"backend/routes/routes.go"}, "v0.8.9")
	assert.Equal(t, []string{SurfaceRoute}, v.Surfaces)
	assert.True(t, v.RowFound)
	assert.Empty(t, v.Missing)
	assert.True(t, v.OK())
}

func TestEvaluateMissingRow(t *testing.T) {
	v := Evaluate([]byte(ledgerWithRow), []string{"backend/routes/routes.go"}, "v0.9.0")
	assert.False(t, v.RowFound)
	assert.False(t, v.OK())
	assert.Contains(t, FormatMissing(v, "v0.9.0"), "no row")
}

func TestEvaluateRowMissingClass(t *testing.T) {
	v := Evaluate([]byte(ledgerWithRow), []string{"backend/database/migrations/000072_x.up.sql"}, "v0.8.8")
	assert.True(t, v.RowFound)
	assert.Equal(t, []string{SurfacePersistence}, v.Missing)
	assert.False(t, v.OK())
	assert.Contains(t, FormatMissing(v, "v0.8.8"), "persistence")
}

func TestParseRowsSkipsHeaderAndSeparator(t *testing.T) {
	rows := parseRows([]byte(ledgerWithRow))
	assert.NotContains(t, rows, "Release")
	assert.NotContains(t, rows, "---")
	require.Contains(t, rows, "v0.8.8")
	assert.Contains(t, rows["v0.8.9"], "auth-path")
}

func TestFormatMissingNoSurfaces(t *testing.T) {
	assert.Contains(t, FormatMissing(Verdict{}, "v1.0.0"), "no recognised")
}
