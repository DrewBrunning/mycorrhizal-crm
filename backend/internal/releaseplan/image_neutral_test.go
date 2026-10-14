package releaseplan

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1704: docker-publish.yml's tag-time reuse publishes the image the
// pre-tag battery validated INSTEAD of rebuilding from the tag commit. That is
// sound only because the fixture-registration commit (FixtureAllowlist) cannot
// change the runtime image: the files sit in the Docker build context
// (`COPY backend/ .`) but are not compiled or embedded into the server binary,
// and the Dockerfile's final stage copies only that binary, static assets,
// the built frontend and docker/ config. This test proves the "not compiled or
// embedded" half by asking the Go toolchain for the server binary's exact
// build inputs, for both release platforms. If a future change links
// schemafixture into the server, or go:embeds a schema dump, reuse would
// silently publish an image that lacks the tag commit's content -- this fails
// first.
func TestFixtureAllowlistIsNotInTheRuntimeBinary(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	backend := filepath.Join(root, "backend")

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}

	allow := map[string]bool{}
	for _, p := range FixtureAllowlist("v9.9.9") {
		allow[filepath.Join(root, filepath.FromSlash(p))] = true
	}

	for _, goarch := range []string{"amd64", "arm64"} {
		cmd := exec.Command("go", "list", "-deps",
			"-f", `{{.ImportPath}}|{{.Dir}}|{{join .GoFiles ","}}|{{join .EmbedFiles ","}}`, ".")
		cmd.Dir = backend
		cmd.Env = append(cmd.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+goarch)
		out, err := cmd.Output()
		require.NoError(t, err, "go list -deps (the server binary's build inputs) for linux/%s", goarch)

		seen := 0
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			parts := strings.Split(line, "|")
			require.Len(t, parts, 4, line)
			importPath, dir := parts[0], parts[1]
			seen++
			assert.NotEqual(t, "mycorrhizal/internal/schemafixture", importPath,
				"the server binary links the schema-fixture package: a fixture-only tag commit now changes the runtime image, so reuse must not adopt the validated digest (issue #1704)")
			for _, list := range []string{parts[2], parts[3]} {
				for _, f := range strings.Split(list, ",") {
					if f == "" {
						continue
					}
					assert.False(t, allow[filepath.Join(dir, f)],
						"%s (linux/%s) compiles or embeds %s, which a fixture-only tag commit changes (issue #1704)", importPath, goarch, f)
				}
			}
		}
		require.Greater(t, seen, 50, "go list returned too few packages to be the server's dependency closure")
	}
}
