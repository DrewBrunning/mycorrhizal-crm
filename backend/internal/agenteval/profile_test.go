package agenteval

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProfile(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "testdata/agent-eval/agents.yaml", strings.Join([]string{
		"version: 1",
		"profiles:",
		"  dry-run:",
		"    command: \"true\"",
		"    name: dry-run",
		"    model: none",
		"    version: \"0\"",
		"    dry_run: true",
		"  pinned:",
		"    command: \"opencode run {prompt}\"",
		"    name: opencode",
		"    model: deepseek-flash",
		"    version: \"1.2.3\"",
	}, "\n"))
	p, err := LoadProfile(root, "testdata/agent-eval/agents.yaml", "pinned")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "opencode" || p.Model != "deepseek-flash" || p.Version != "1.2.3" || p.DryRun {
		t.Fatalf("profile = %+v", p)
	}
	if _, err := LoadProfile(root, "testdata/agent-eval/agents.yaml", "missing"); err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Fatalf("missing profile err = %v", err)
	}
}

func TestLoadProfileErrors(t *testing.T) {
	root := t.TempDir()
	path := "testdata/agent-eval/agents.yaml"
	if _, err := LoadProfile(root, path, "pinned"); err == nil || !strings.Contains(err.Error(), "read agent profiles") {
		t.Fatalf("missing file err = %v", err)
	}

	writeRepoFile(t, root, path, "version: [")
	if _, err := LoadProfile(root, path, "pinned"); err == nil || !strings.Contains(err.Error(), "parse agent profiles") {
		t.Fatalf("malformed yaml err = %v", err)
	}

	writeRepoFile(t, root, path, "version: 0\nprofiles: {}\n")
	if _, err := LoadProfile(root, path, "pinned"); err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("bad version err = %v", err)
	}

	writeRepoFile(t, root, path, strings.Join([]string{
		"version: 1",
		"profiles:",
		"  unpinned:",
		"    command: \"agent {prompt}\"",
		"    name: agent",
		"    model: m",
		"    version: latest",
	}, "\n"))
	if _, err := LoadProfile(root, path, "unpinned"); err == nil || !strings.Contains(err.Error(), "not pinned") {
		t.Fatalf("validation err = %v", err)
	}
}

func TestProfileValidation(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
		want    string
	}{
		{"no name", Profile{Command: "x {prompt}", Model: "m", Version: "1"}, "no name"},
		{"no model", Profile{Command: "x {prompt}", Name: "n", Version: "1"}, "no model"},
		{"unpinned", Profile{Command: "x {prompt}", Name: "n", Model: "m", Version: "latest"}, "not pinned"},
		{"no command", Profile{Name: "n", Model: "m", Version: "1"}, "no command"},
		{"latest tag", Profile{Command: "npx agent@latest {prompt}", Name: "n", Model: "m", Version: "1"}, "@latest"},
		{"no prompt", Profile{Command: "true", Name: "n", Model: "m", Version: "1"}, "no {prompt}"},
		{"none model", Profile{Command: "x {prompt}", Name: "n", Model: "none", Version: "1"}, "model none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.profile.Validate("p")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	dry := Profile{Command: "true", Name: "dry", Model: "none", Version: "0", DryRun: true}
	if err := dry.Validate("dry"); err != nil {
		t.Fatalf("dry run rejected: %v", err)
	}
}

func TestCommittedProfilesLoad(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LoadProfile(root, "testdata/agent-eval/agents.yaml", "dry-run"); err != nil {
		t.Fatal(err)
	}
}
