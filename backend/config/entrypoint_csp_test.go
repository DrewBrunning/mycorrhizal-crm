package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoFile reads a file relative to the repo root (backend/config -> ../..).
func repoFile(t *testing.T, rel ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{"..", ".."}, rel...)...)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// renderCSPTile runs the shared frontend/docker/render-csp-tile.sh with the
// given MAP_TILE_STYLE_URL and returns the rendered include (or the failure).
func renderCSPTile(t *testing.T, url string, set bool) (rendered string, err error) {
	t.Helper()
	script, absErr := filepath.Abs(filepath.Join("..", "..", "frontend", "docker", "render-csp-tile.sh"))
	if absErr != nil {
		t.Fatal(absErr)
	}
	out := filepath.Join(t.TempDir(), "csp_tile.conf")
	cmd := exec.Command("sh", script, out)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
	if set {
		cmd.Env = append(cmd.Env, "MAP_TILE_STYLE_URL="+url)
	}
	if combined, runErr := cmd.CombinedOutput(); runErr != nil {
		return string(combined), runErr
	}
	b, readErr := os.ReadFile(out)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(b), nil
}

// TestCSPTileScriptDefaultMatchesGoDefault pins the two copies of the
// OpenFreeMap default: config.DefaultMapTileStyleURL (what the backend serves
// from GET /api/v1/config/map) and the fallback the shared render script bakes
// into the SPA CSP when MAP_TILE_STYLE_URL is unset. If they drift, a
// default-config deployment serves one tile style but CSP-allows another
// origin, and the map silently loses its basemap.
func TestCSPTileScriptDefaultMatchesGoDefault(t *testing.T) {
	if !strings.Contains(repoFile(t, "frontend", "docker", "render-csp-tile.sh"), DefaultMapTileStyleURL) {
		t.Fatalf("render-csp-tile.sh no longer names the default tile style %q", DefaultMapTileStyleURL)
	}
	want := `set $csp_tile_origin "https://tiles.openfreemap.org";` + "\n"
	for _, f := range []string{"frontend/docker/csp_tile.inc.default"} {
		if got := repoFile(t, strings.Split(f, "/")...); got != want {
			t.Fatalf("%s = %q, want %q", f, got, want)
		}
	}
	if !strings.Contains(repoFile(t, "Dockerfile"), `set \$csp_tile_origin \"https://tiles.openfreemap.org\"`) {
		t.Fatal("root Dockerfile's seeded csp_tile.conf no longer carries the default origin")
	}
}

// TestCSPTileScriptRendersOrigin runs the real script against realistic and
// hostile MAP_TILE_STYLE_URL values. Only scheme://host[:port] may ever reach
// the nginx include; anything unparseable must refuse to render (non-zero) so
// the container fails to start instead of shipping a broken/injectable CSP.
func TestCSPTileScriptRendersOrigin(t *testing.T) {
	ok := map[string]string{
		"https://tiles.openfreemap.org/styles/liberty":    "https://tiles.openfreemap.org",
		"https://tiles.example.org:8443/a/b.json":         "https://tiles.example.org:8443",
		"http://localhost:8080/style.json":                "http://localhost:8080",
		"https://tiles.example.org":                       "https://tiles.example.org",
		"https://tiles.example.org/":                      "https://tiles.example.org",
		"https://tiles.example.org?key=abc":               "https://tiles.example.org",
		"https://tiles.example.org#frag":                  "https://tiles.example.org",
		"https://tiles.example.org:8443?key=abc":          "https://tiles.example.org:8443",
		"http://[::1]:8000/style.json":                    "http://[::1]:8000",
		"http://10.0.0.5:3000/styles/x":                   "http://10.0.0.5:3000",
		"  https://tiles.example.org/styles/x  ":          "https://tiles.example.org",
		"https://tiles.example.org/styles/x?k=\";evil;\"": "https://tiles.example.org", // path/query is discarded, never emitted
	}
	for in, origin := range ok {
		got, err := renderCSPTile(t, in, true)
		if err != nil {
			t.Errorf("%q: unexpected failure: %v: %s", in, err, got)
			continue
		}
		if want := `set $csp_tile_origin "` + origin + "\";\n"; got != want {
			t.Errorf("%q: rendered %q, want %q", in, got, want)
		}
	}

	// Unset and blank both mean "use the default".
	for _, blank := range []string{"", "   "} {
		got, err := renderCSPTile(t, blank, true)
		if err != nil || got != "set $csp_tile_origin \"https://tiles.openfreemap.org\";\n" {
			t.Errorf("blank %q: got %q, %v", blank, got, err)
		}
	}
	if got, err := renderCSPTile(t, "", false); err != nil || !strings.Contains(got, "tiles.openfreemap.org") {
		t.Errorf("unset: got %q, %v", got, err)
	}

	bad := []string{
		"https://user:pw@tiles.example.org/styles/x", // credentials would land in the header
		"https://tiles.example.org\"; add_header X-Pwn 1; #",
		"https://tiles.example.org;evil",
		"https://tiles.example.org$host/x",
		"https://tiles.example.org\nhttps://other.example",
		"ftp://tiles.example.org/x",
		"//tiles.example.org/x",
		"tiles.example.org/x",
		"https://",
		"https:///path",
		"https://tiles.example.org:notaport/x",
		"javascript:alert(1)",
	}
	for _, in := range bad {
		if got, err := renderCSPTile(t, in, true); err == nil {
			t.Errorf("%q: rendered %q, want a refusal", in, got)
		}
	}
}

var cspHeader = regexp.MustCompile(`add_header Content-Security-Policy "([^"]*)" always;`)

// TestNginxCSPConsistentAcrossImages: the SPA CSP lives in two hand-maintained
// nginx configs (the all-in-one image and the split frontend image). Every
// emission in both must be byte-identical, carry the directives the contact map
// needs, take the tile origin from $csp_tile_origin (never a hard-coded host,
// which silently ignores MAP_TILE_STYLE_URL), and the config must include the
// file that defines that variable.
func TestNginxCSPConsistentAcrossImages(t *testing.T) {
	confs := map[string]string{
		"docker/nginx.conf":   repoFile(t, "docker", "nginx.conf"),
		"frontend/nginx.conf": repoFile(t, "frontend", "nginx.conf"),
	}
	includes := map[string]string{
		"docker/nginx.conf":   "include /etc/nginx/csp_tile.conf;",
		"frontend/nginx.conf": "include /etc/nginx/conf.d/csp_tile.inc;",
	}
	var canonical string
	for name, body := range confs {
		matches := cspHeader.FindAllStringSubmatch(body, -1)
		if len(matches) == 0 {
			t.Fatalf("%s: no Content-Security-Policy header found", name)
		}
		if !strings.Contains(body, includes[name]) {
			t.Errorf("%s: missing %q — $csp_tile_origin would be undefined", name, includes[name])
		}
		for _, m := range matches {
			csp := m[1]
			if canonical == "" {
				canonical = csp
			}
			if csp != canonical {
				t.Errorf("%s: CSP drifted from the other emissions:\n got  %s\n want %s", name, csp, canonical)
			}
		}
	}
	for _, need := range []string{
		"connect-src 'self' $csp_tile_origin",
		"img-src 'self' data: blob: $csp_tile_origin",
		"worker-src 'self' blob:",
		"frame-ancestors 'none'",
	} {
		if !strings.Contains(canonical, need) {
			t.Errorf("CSP lacks %q: %s", need, canonical)
		}
	}
	// No hard-coded hosts and no blanket scheme sources anywhere.
	if regexp.MustCompile(`https?:[^/ ]|https?://[a-z]`).MatchString(canonical) {
		t.Errorf("CSP hard-codes a host instead of $csp_tile_origin: %s", canonical)
	}
	if strings.Contains(canonical, " https:") || strings.Contains(canonical, " http:") || strings.Contains(canonical, " *") {
		t.Errorf("CSP must not allow a blanket scheme or wildcard: %s", canonical)
	}
}

// TestDockerfilesWireTileCSP: both images must ship the render script where
// their entrypoint will run it, plus a seeded default include.
func TestDockerfilesWireTileCSP(t *testing.T) {
	root := repoFile(t, "Dockerfile")
	for _, want := range []string{"/app/render-csp-tile.sh", "frontend/docker/render-csp-tile.sh"} {
		if !strings.Contains(root, want) {
			t.Errorf("root Dockerfile does not reference %s", want)
		}
	}
	if !strings.Contains(repoFile(t, "docker", "entrypoint.sh"), "/app/render-csp-tile.sh /etc/nginx/csp_tile.conf") {
		t.Error("docker/entrypoint.sh no longer renders the tile origin via the shared script")
	}
	split := repoFile(t, "frontend", "Dockerfile")
	for _, want := range []string{
		"/docker-entrypoint.d/40-render-csp-tile.sh",
		"CSP_TILE_CONF_PATH=/etc/nginx/conf.d/csp_tile.inc",
		"/etc/nginx/conf.d/csp_tile.inc",
	} {
		if !strings.Contains(split, want) {
			t.Errorf("frontend/Dockerfile does not reference %s", want)
		}
	}
}
