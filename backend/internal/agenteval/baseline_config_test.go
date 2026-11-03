package agenteval

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/internal/agentcontext"

	yaml "go.yaml.in/yaml/v3"
)

// baselineConfigFindings recomputes every committed baseline entry's
// configuration fingerprint from the committed profile, task spec and suite
// commands. It catches a stale or hand-edited config_hash (the baseline is
// only comparable while the measurement identity is unchanged), and a baseline
// entry whose profile or task no longer exists.
func baselineConfigFindings(root string) ([]string, error) {
	baselinePath := filepath.Join(root, "testdata", "agent-eval", "baseline.json")
	base, err := LoadBaseline(baselinePath)
	if err != nil {
		return nil, err
	}

	profileBody, err := os.ReadFile(filepath.Join(root, "testdata", "agent-eval", "agents.yaml")) // #nosec G304 -- fixed repository path
	if err != nil {
		return nil, fmt.Errorf("read agent profiles: %w", err)
	}
	var profiles profileFile
	if err := yaml.Unmarshal(profileBody, &profiles); err != nil {
		return nil, fmt.Errorf("parse agent profiles: %w", err)
	}

	suites := agentcontext.Suites()
	names := make([]string, 0, len(suites))
	local := make([]Suite, 0, len(suites))
	for _, s := range suites {
		names = append(names, s.Name)
		local = append(local, Suite{Name: s.Name, Commands: s.Commands})
	}

	var findings []string
	profileNames := make([]string, 0, len(base.Profiles))
	for name := range base.Profiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)

	for _, profileName := range profileNames {
		profile, ok := profiles.Profiles[profileName]
		if !ok {
			findings = append(findings, fmt.Sprintf("baseline profile %q is not in agents.yaml", profileName))
			continue
		}
		if err := profile.Validate(profileName); err != nil {
			findings = append(findings, fmt.Sprintf("baseline profile %q is invalid: %v", profileName, err))
			continue
		}
		taskNames := make([]string, 0, len(base.Profiles[profileName]))
		for name := range base.Profiles[profileName] {
			taskNames = append(taskNames, name)
		}
		sort.Strings(taskNames)
		for _, taskName := range taskNames {
			summary := base.Profiles[profileName][taskName]
			spec, err := LoadTask(root, "testdata/agent-eval", taskName, names)
			if err != nil {
				findings = append(findings, fmt.Sprintf("baseline task %q: %v", taskName, err))
				continue
			}
			want := ConfigFingerprint(profile, spec, local)
			switch {
			case summary.ConfigHash == "":
				findings = append(findings, fmt.Sprintf("%s/%s has no config_hash; re-baseline from a passing run", profileName, taskName))
			case summary.ConfigHash != want:
				findings = append(findings, fmt.Sprintf("%s/%s config_hash stale: baseline %s, current %s; re-baseline from a passing run", profileName, taskName, summary.ConfigHash, want))
			}
		}
	}
	return findings, nil
}

func TestCommittedBaselineConfigHashes(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	findings, err := baselineConfigFindings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) > 0 {
		t.Fatalf("committed baseline does not match the committed configuration:\n%s", strings.Join(findings, "\n"))
	}
}

func TestBaselineConfigFindings(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "testdata/agent-eval/agents.yaml", strings.Join([]string{
		"version: 1",
		"profiles:",
		"  pinned:",
		`    command: "agent {prompt}"`,
		"    name: agent",
		"    model: m",
		`    version: "1.2.3"`,
	}, "\n"))
	writeRepoFile(t, root, "testdata/agent-eval/demo/spec.yaml", strings.Join([]string{
		"name: demo",
		"prompt: do it",
		"base_ref: " + strings.Repeat("a", 40),
		"suite: documentation-maintenance",
		`agent_command: "true"`,
		"expect_paths: [docs/**]",
	}, "\n"))

	profile, err := LoadProfile(root, "testdata/agent-eval/agents.yaml", "pinned")
	if err != nil {
		t.Fatal(err)
	}
	suites := agentcontext.Suites()
	names := make([]string, 0, len(suites))
	local := make([]Suite, 0, len(suites))
	for _, s := range suites {
		names = append(names, s.Name)
		local = append(local, Suite{Name: s.Name, Commands: s.Commands})
	}
	spec, err := LoadTask(root, "testdata/agent-eval", "demo", names)
	if err != nil {
		t.Fatal(err)
	}

	base := Baseline{Version: 1, Profiles: map[string]map[string]RunSummary{
		"pinned": {"demo": {Status: "passed", Success: true, CommandsPassed: 1, CommandsTotal: 1, ConfigHash: ConfigFingerprint(profile, spec, local)}},
	}}
	baselinePath := filepath.Join(root, "testdata", "agent-eval", "baseline.json")
	if err := WriteBaseline(baselinePath, base); err != nil {
		t.Fatal(err)
	}

	findings, err := baselineConfigFindings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Fatalf("matching baseline findings = %v", findings)
	}

	base.Profiles["pinned"]["demo"] = RunSummary{Status: "passed", Success: true, CommandsPassed: 1, CommandsTotal: 1, ConfigHash: "deadbeefdeadbeef"}
	if err := WriteBaseline(baselinePath, base); err != nil {
		t.Fatal(err)
	}
	findings, err = baselineConfigFindings(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(findings, "\n"), "config_hash stale") {
		t.Fatalf("stale hash findings = %v", findings)
	}

	base.Profiles["missing"] = map[string]RunSummary{"demo": {ConfigHash: "x"}}
	if err := WriteBaseline(baselinePath, base); err != nil {
		t.Fatal(err)
	}
	findings, err = baselineConfigFindings(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(findings, "\n"), `baseline profile "missing" is not in agents.yaml`) {
		t.Fatalf("missing profile findings = %v", findings)
	}
}

// TestCommittedBaselinePromotions keeps every committed entry tied to a
// measured promotion. A hand-edited config_hash that happens to match the
// committed spec still fails this because only -write-baseline and
// -promote-artifact set verified and the promotion provenance.
func TestCommittedBaselinePromotions(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	base, err := LoadBaseline(filepath.Join(root, "testdata", "agent-eval", "baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	for profile, tasks := range base.Profiles {
		for task, summary := range tasks {
			if !summary.Verified {
				t.Errorf("%s/%s is unverified; promote it from a passing pinned-profile artifact", profile, task)
			}
			if summary.ConfigHash == "" {
				t.Errorf("%s/%s has no config_hash", profile, task)
			}
			if summary.PromotedFrom == "" {
				t.Errorf("%s/%s has no promotion provenance", profile, task)
			}
		}
	}
}
