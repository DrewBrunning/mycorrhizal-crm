package mutationscope

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

// findBackendRoot walks up from the test's package directory to the backend
// module root (marked by go.mod), mirroring cmd/genmutationscope's own
// lookup so the test exercises the same generation the CLI does.
func findBackendRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "backend module root (go.mod) not found above %s", dir)
		dir = parent
	}
}

// TestGeneratedConfigsAreCurrent is the issue #915 drift gate: the
// controllers-delete-cascade and services-import scopes narrow mutation to a
// few files inside much larger packages by excluding every other file, and
// that exclude list must track the package's actual file set. Regenerate
// with `cd backend && go run ./cmd/genmutationscope` after adding, removing,
// or renaming a file in any scoped package.
func TestGeneratedConfigsAreCurrent(t *testing.T) {
	root := findBackendRoot(t)

	regenerated, err := Generate(root)
	require.NoError(t, err)
	require.Len(t, regenerated, len(Scopes), "one generated config per scope")

	for rel, want := range regenerated {
		path := filepath.Join(root, rel)
		got, err := os.ReadFile(path) // #nosec G304 -- path built from Generate's own known-safe output, not attacker input
		require.NoError(t, err, "committed config missing at %s — run `go run ./cmd/genmutationscope`", rel)
		assert.Equal(t, want, string(got),
			"%s is stale — run `cd backend && go run ./cmd/genmutationscope` and commit the diff", rel)
	}
}

// TestScopesHaveReasons enforces the same discipline PERF-04's budgets.json
// uses: a threshold number is a deliberate, reviewed edit that must carry a
// reason, not a bare figure someone can silently loosen.
func TestScopesHaveReasons(t *testing.T) {
	require.NotEmpty(t, Scopes)
	seen := make(map[string]bool, len(Scopes))
	for _, s := range Scopes {
		t.Run(s.Name, func(t *testing.T) {
			assert.NotEmpty(t, s.Name)
			assert.NotEmpty(t, s.PackageDir)
			assert.NotEmpty(t, s.Reason, "threshold must record why it was set")
			assert.False(t, seen[s.Name], "duplicate scope name %q", s.Name)
			seen[s.Name] = true
			assert.Greater(t, s.Efficacy, 0.0)
			assert.LessOrEqual(t, s.Efficacy, 100.0)
			assert.Greater(t, s.MutantCoverage, 0.0)
			assert.LessOrEqual(t, s.MutantCoverage, 100.0)
		})
	}
}

// TestScopedPackagesExist guards against a scope naming a package or target
// file that no longer exists — the generator would otherwise silently write
// an empty exclude list (whole-package scope) instead of failing loudly.
func TestScopedPackagesExist(t *testing.T) {
	root := findBackendRoot(t)
	for _, s := range Scopes {
		t.Run(s.Name, func(t *testing.T) {
			info, err := os.Stat(filepath.Join(root, s.PackageDir))
			require.NoError(t, err, "PackageDir %q must exist", s.PackageDir)
			require.True(t, info.IsDir())
		})
	}
}

// TestWorkflowMatrixMatchesScopes fails if go-mutation.yml's
// strategy.matrix.include legs drift from internal/mutationscope.Scopes —
// the two are hand-kept in sync (see the package doc) rather than
// generated, because the workflow file also carries each leg's package dir
// and there is no clean single source that would make regenerating YAML
// into YAML worth the indirection.
func TestWorkflowMatrixMatchesScopes(t *testing.T) {
	root := findBackendRoot(t)
	repoRoot := filepath.Dir(root)
	path := filepath.Join(repoRoot, ".github", "workflows", "go-mutation.yml")
	data, err := os.ReadFile(path) // #nosec G304 -- fixed path within the repo, not attacker input
	require.NoError(t, err, "go-mutation.yml must exist at %s", path)

	var wf struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Include []struct {
						Scope string `yaml:"scope"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(data, &wf))

	var matrixScopes []string
	for _, job := range wf.Jobs {
		for _, entry := range job.Strategy.Matrix.Include {
			if entry.Scope != "" {
				matrixScopes = append(matrixScopes, entry.Scope)
			}
		}
	}
	require.NotEmpty(t, matrixScopes, "go-mutation.yml must declare a matrix.include list with a scope field")

	wantNames := make([]string, 0, len(Scopes))
	for _, s := range Scopes {
		wantNames = append(wantNames, s.Name)
	}
	assert.ElementsMatch(t, wantNames, matrixScopes,
		"go-mutation.yml's matrix.scope must list exactly internal/mutationscope.Scopes' names")
}
