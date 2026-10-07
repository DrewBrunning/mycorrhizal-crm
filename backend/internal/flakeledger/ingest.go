package flakeledger

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// ArtifactPrefix marks the artifacts the ledger reads. Every other artifact a
// workflow uploads is ignored (and never downloaded).
const ArtifactPrefix = "flake-"

// RunsFile is the metadata index the collector writes at the data root.
const RunsFile = "runs.json"

// suiteOf maps an artifact name to a ledger suite: "flake-go-core" ->
// "go/core", "flake-playwright-e2e" -> "playwright/e2e".
func suiteOf(artifact string) string {
	name := strings.TrimPrefix(artifact, ArtifactPrefix)
	if i := strings.Index(name, "-"); i > 0 {
		return name[:i] + "/" + name[i+1:]
	}
	return name
}

func readObs(path string, parse func(*os.File) (Observations, error)) (Observations, error) {
	f, err := os.Open(path) // #nosec G304 -- path comes from walking the ledger data dir
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parse(f)
}

// LoadRuns reads <root>/runs.json and, for each run, the artifact trees under
// <root>/<run_id>/<artifact name>/. Unparseable files become warnings, never
// errors: the ledger is advisory and one corrupt artifact must not blind it.
func LoadRuns(root string) ([]RunData, []string, error) {
	b, err := os.ReadFile(filepath.Join(root, RunsFile)) // #nosec G304 -- operator-supplied data dir
	if err != nil {
		return nil, nil, err
	}
	var runs []Run
	if err := json.Unmarshal(b, &runs); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", RunsFile, err)
	}
	var warns []string
	out := make([]RunData, 0, len(runs))
	for _, run := range runs {
		rd := RunData{Run: run, Suites: map[string]Observations{}}
		runDir := filepath.Join(root, strconv.FormatInt(run.RunID, 10))
		entries, _ := os.ReadDir(runDir) // a run with no artifacts is normal
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), ArtifactPrefix) {
				continue
			}
			warns = append(warns, loadArtifact(filepath.Join(runDir, e.Name()), e.Name(), &rd)...)
		}
		out = append(out, rd)
	}
	return out, warns, nil
}

func addSuite(rd *RunData, suite string, obs Observations) {
	cur, ok := rd.Suites[suite]
	if !ok {
		cur = Observations{}
		rd.Suites[suite] = cur
	}
	cur.Merge(obs)
}

func loadArtifact(dir, name string, rd *RunData) []string {
	var warns []string
	suite := suiteOf(name)
	first, final, override := Observations{}, Observations{}, Observations{}
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	warn := func(p string, err error) {
		rel, _ := filepath.Rel(dir, p)
		warns = append(warns, fmt.Sprintf("run %d %s/%s: %v", rd.Run.RunID, name, rel, err))
	}
	for _, p := range files {
		base := filepath.Base(p)
		switch {
		case strings.HasPrefix(base, "flake-signals") && strings.HasSuffix(base, ".json"):
			f, err := os.Open(p) // #nosec G304 -- walked from the data dir
			if err != nil {
				warn(p, err)
				continue
			}
			s, obs, err := ParseSignals(f)
			_ = f.Close()
			if err != nil {
				warn(p, err)
				continue
			}
			addSuite(rd, s, obs)
		case strings.HasPrefix(base, "rerun-fails") && strings.HasSuffix(base, ".txt"):
			obs, err := readObs(p, func(f *os.File) (Observations, error) { return ParseRerunReport(f) })
			if err != nil {
				warn(p, err)
				continue
			}
			// The rerun report is gotestsum's own verdict on the final
			// state, so it overrides what the per-attempt JUnit implies.
			for k, v := range obs {
				override[k] = v
			}
		case strings.HasSuffix(base, ".xml"):
			obs, err := readObs(p, func(f *os.File) (Observations, error) { return ParseJUnit(f) })
			if err != nil {
				warn(p, err)
				continue
			}
			if inFlakyAttempt(dir, p) {
				first.Merge(obs)
			} else {
				final.Merge(obs)
			}
		case strings.HasSuffix(base, ".json"):
			obs, err := readObs(p, func(f *os.File) (Observations, error) { return ParsePlaywright(f) })
			if err != nil {
				warn(p, err)
				continue
			}
			final.Merge(obs)
		}
	}
	if len(first) > 0 {
		final = MergeAttempts(first, final)
	}
	for k, v := range override {
		final[k] = v
	}
	if len(final) > 0 {
		addSuite(rd, suite, final)
	}
	return warns
}

func inFlakyAttempt(root, p string) bool {
	rel, _ := filepath.Rel(root, filepath.Dir(p))
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(part, "flaky-attempt-") {
			return true
		}
	}
	return false
}
