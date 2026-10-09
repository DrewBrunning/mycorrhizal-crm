package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

// Issues #1625/#1626: a native Go fuzz target that is not in
// `.github/scripts/run-fuzz-targets.sh`'s `targets=(…)` array never runs in CI
// — the same silent-gap shape as a scheduled workflow missing from the
// nightly-failure alert. This pins the two-way parity: every `func Fuzz*` in
// backend/ is registered, and every registered target names a real function.
//
// It also pins the nightly shard list: `fuzz-nightly`'s
// `strategy.matrix.package` must equal the distinct package dirs of the
// registered targets, so a target added to a new package cannot silently be
// left out of the nightly schedule (the PR smoke runs every target, but the
// nightly is what gives it the full budget).

var (
	fuzzFuncRe   = regexp.MustCompile(`(?m)^func (Fuzz\w+)\(`)
	fuzzScriptRe = regexp.MustCompile(`^\s*"(\./\S+) \^(\w+)\$"\s*$`)
)

func TestEveryFuzzTargetIsRegistered(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	backendRoot := cwd
	repoRoot := filepath.Dir(backendRoot)

	// 1. Every fuzz function declared in backend's test files.
	defined := map[string]string{} // name -> package dir as the script names it
	require.NoError(t, filepath.WalkDir(backendRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "testdata", "vendor", "node_modules", ".git":
				return filepath.SkipDir
			}
			if strings.HasPrefix(d.Name(), ".") && path != backendRoot {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path) // #nosec G304 -- fixed walk within the repo, not attacker input
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(backendRoot, filepath.Dir(path))
		if err != nil {
			return err
		}
		dir := "./" + filepath.ToSlash(rel)
		if rel == "." {
			dir = "."
		}
		for _, m := range fuzzFuncRe.FindAllStringSubmatch(string(data), -1) {
			defined[m[1]] = dir
		}
		return nil
	}))

	// 2. The runner script's target list.
	scriptPath := filepath.Join(repoRoot, ".github", "scripts", "run-fuzz-targets.sh")
	scriptData, err := os.ReadFile(scriptPath)
	require.NoError(t, err, "run-fuzz-targets.sh must exist at %s", scriptPath)

	registered := map[string]string{} // name -> package dir
	dirSet := map[string]bool{}
	for _, line := range strings.Split(string(scriptData), "\n") {
		if m := fuzzScriptRe.FindStringSubmatch(line); m != nil {
			registered[m[2]] = m[1]
			dirSet[m[1]] = true
		}
	}
	require.NotEmpty(t, registered, "run-fuzz-targets.sh must declare a non-empty targets=(…) array")

	// Every declared target is registered, with the matching package dir.
	definedNames := sortedKeys(defined)
	registeredNames := sortedKeys(registered)
	for _, name := range definedNames {
		require.Contains(t, registered, name,
			"fuzz target %s (%s) is not in run-fuzz-targets.sh's targets array — an unregistered target never runs", name, defined[name])
		assert.Equal(t, defined[name], registered[name],
			"fuzz target %s is registered for %q but declared in %q", name, registered[name], defined[name])
	}
	// Every registered target names a real function (a stale entry is dead CI time).
	for _, name := range registeredNames {
		assert.Contains(t, defined, name,
			"run-fuzz-targets.sh registers %s (%s) but no `func %s` exists in backend/ — remove it", name, registered[name], name)
	}

	// 3. The nightly shard list covers exactly the registered package dirs.
	wfPath := filepath.Join(repoRoot, ".github", "workflows", "unit-tests.yml")
	wfData, err := os.ReadFile(wfPath) // #nosec G304 -- fixed path within the repo, not attacker input
	require.NoError(t, err)

	var wf struct {
		Jobs map[string]struct {
			Strategy struct {
				Matrix struct {
					Package []string `yaml:"package"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
		} `yaml:"jobs"`
	}
	require.NoError(t, yaml.Unmarshal(wfData, &wf))

	job, ok := wf.Jobs["fuzz-nightly"]
	require.True(t, ok, "unit-tests.yml must declare a `fuzz-nightly` job")
	require.NotEmpty(t, job.Strategy.Matrix.Package, "fuzz-nightly must shard by a strategy.matrix.package list")

	wantDirs := make([]string, 0, len(dirSet))
	for d := range dirSet {
		wantDirs = append(wantDirs, d)
	}
	sort.Strings(wantDirs)
	gotDirs := append([]string(nil), job.Strategy.Matrix.Package...)
	sort.Strings(gotDirs)
	assert.Equal(t, wantDirs, gotDirs,
		"fuzz-nightly's matrix.package must list exactly the packages that have registered fuzz targets")
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
