package agenteval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// RunSummary is the stable, comparable part of one run. It deliberately omits
// wall-clock and changed-file counts: an agent may solve a task differently
// without regressing, and neither number is a quality signal on its own.
type RunSummary struct {
	Status              string `json:"status"` // passed | failed
	Success             bool   `json:"success"`
	CommandsPassed      int    `json:"commands_passed"`
	CommandsTotal       int    `json:"commands_total"`
	ScopeViolations     int    `json:"scope_violations"`
	HandEditedGenerated int    `json:"hand_edited_generated"`
}

// Baseline is the committed record of the last accepted run per profile and
// task. A missing file or entry is "no baseline", never a regression.
type Baseline struct {
	Version  int                              `json:"version"`
	Profiles map[string]map[string]RunSummary `json:"profiles"`
}

// BaselineComparison is the per-run delta against the committed baseline.
type BaselineComparison struct {
	Profile     string      `json:"profile"`
	Task        string      `json:"task"`
	Present     bool        `json:"present"`
	Baseline    *RunSummary `json:"baseline,omitempty"`
	Current     RunSummary  `json:"current"`
	Regressions []string    `json:"regressions,omitempty"`
}

// Regressed reports whether the current run is worse than the baseline.
func (b BaselineComparison) Regressed() bool { return len(b.Regressions) > 0 }

// SummarizeRun extracts the comparable part of one finished run.
func SummarizeRun(status string, run *RunResult) RunSummary {
	s := RunSummary{Status: status}
	if run == nil {
		return s
	}
	s.Success = run.Success
	s.CommandsTotal = len(run.SuiteCommands)
	for _, c := range run.SuiteCommands {
		if c.Passed {
			s.CommandsPassed++
		}
	}
	s.ScopeViolations = len(run.ScopeViolations)
	s.HandEditedGenerated = len(run.HandEditedGenerated)
	return s
}

// LoadBaseline reads path. A missing file is an empty baseline; a malformed
// one is an error, because silently discarding a committed baseline would
// hide every regression.
func LoadBaseline(path string) (Baseline, error) {
	b := Baseline{Version: 1, Profiles: map[string]map[string]RunSummary{}}
	body, err := os.ReadFile(path) // #nosec G304 -- operator-supplied baseline path
	if err != nil {
		if os.IsNotExist(err) {
			return b, nil
		}
		return Baseline{}, fmt.Errorf("read baseline: %w", err)
	}
	if err := json.Unmarshal(body, &b); err != nil {
		return Baseline{}, fmt.Errorf("parse baseline: %w", err)
	}
	if b.Version < 1 {
		return Baseline{}, fmt.Errorf("baseline version = %d, want >= 1", b.Version)
	}
	if b.Profiles == nil {
		b.Profiles = map[string]map[string]RunSummary{}
	}
	return b, nil
}

// WriteBaseline writes the baseline atomically enough for a CLI artifact:
// render first, then replace the file.
func WriteBaseline(path string, b Baseline) error {
	if b.Version < 1 {
		b.Version = 1
	}
	if b.Profiles == nil {
		b.Profiles = map[string]map[string]RunSummary{}
	}
	body, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil { // #nosec G306 -- committed baseline, not secret
		return err
	}
	return os.Rename(tmp, path)
}

// RecordBaseline stores one run, preserving every other profile and task.
func RecordBaseline(b *Baseline, profile, task string, summary RunSummary) {
	if b.Profiles == nil {
		b.Profiles = map[string]map[string]RunSummary{}
	}
	if b.Profiles[profile] == nil {
		b.Profiles[profile] = map[string]RunSummary{}
	}
	b.Profiles[profile][task] = summary
}

// CompareBaseline reports regressions against the committed entry. A missing
// profile/task is not a regression.
func CompareBaseline(b Baseline, profile, task string, current RunSummary) BaselineComparison {
	c := BaselineComparison{Profile: profile, Task: task, Current: current}
	byTask, ok := b.Profiles[profile]
	if !ok {
		return c
	}
	base, ok := byTask[task]
	if !ok {
		return c
	}
	c.Present = true
	c.Baseline = &base
	if base.Success && !current.Success {
		c.Regressions = append(c.Regressions, "task passed before; now failed")
	}
	if current.CommandsPassed < base.CommandsPassed {
		c.Regressions = append(c.Regressions, fmt.Sprintf("suite commands passed %d -> %d", base.CommandsPassed, current.CommandsPassed))
	}
	if current.ScopeViolations > base.ScopeViolations {
		c.Regressions = append(c.Regressions, fmt.Sprintf("scope violations %d -> %d", base.ScopeViolations, current.ScopeViolations))
	}
	if current.HandEditedGenerated > base.HandEditedGenerated {
		c.Regressions = append(c.Regressions, fmt.Sprintf("generated-artifact hand-edits %d -> %d", base.HandEditedGenerated, current.HandEditedGenerated))
	}
	sort.Strings(c.Regressions)
	return c
}
