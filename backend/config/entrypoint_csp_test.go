package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEntrypointTileOriginDefaultMatchesGoDefault pins the two copies of the
// OpenFreeMap default: config.DefaultMapTileStyleURL (what the backend serves
// from GET /api/v1/config/map) and the fallback docker/entrypoint.sh bakes into
// the SPA Content-Security-Policy when MAP_TILE_STYLE_URL is unset. If they
// drift, a default-config deployment serves one tile style but CSP-allows
// another origin, and the map silently loses its basemap.
func TestEntrypointTileOriginDefaultMatchesGoDefault(t *testing.T) {
	path := filepath.Join("..", "..", "docker", "entrypoint.sh")
	script, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(script), DefaultMapTileStyleURL) {
		t.Fatalf("docker/entrypoint.sh no longer names the default tile style %q — "+
			"update its CSP fallback to match config.DefaultMapTileStyleURL", DefaultMapTileStyleURL)
	}
}
