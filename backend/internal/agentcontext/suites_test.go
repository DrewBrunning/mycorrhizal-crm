package agentcontext

import (
	"strings"
	"testing"
)

func TestSuiteWithNonexistentJobFails(t *testing.T) {
	root := validTree(t)
	got := strings.Join(findingsForSuites(root, []Suite{{
		Name:      "bad-job",
		Commands:  []string{"cd backend && go test ./..."},
		CIJobs:    []string{"does-not-exist"},
		CICommand: "go test",
	}}), "\n")
	if !strings.Contains(got, `suite "bad-job" names nonexistent CI job "does-not-exist"`) {
		t.Fatalf("nonexistent job not reported: %s", got)
	}
}

func TestSuiteCICommandMustAppear(t *testing.T) {
	root := validTree(t)
	got := strings.Join(findingsForSuites(root, []Suite{{
		Name:      "no-command",
		Commands:  []string{"cd backend && go test ./..."},
		CIJobs:    []string{"backend-tests"},
		CICommand: "make something",
	}}), "\n")
	if !strings.Contains(got, `suite "no-command": command "make something" appears in none of its CI jobs`) {
		t.Fatalf("missing CI command not reported: %s", got)
	}
}

func TestSuiteRequiresCommandsAndCICommand(t *testing.T) {
	root := validTree(t)
	got := strings.Join(findingsForSuites(root, []Suite{
		{Name: "no-commands", CIJobs: []string{"backend-tests"}, CICommand: "go test"},
		{Name: "no-ci-command", Commands: []string{"go test ./..."}},
	}), "\n")
	if !strings.Contains(got, `suite "no-commands" has no DoD commands`) {
		t.Fatalf("missing commands not reported: %s", got)
	}
	if !strings.Contains(got, `suite "no-ci-command" has no CICommand`) {
		t.Fatalf("missing CICommand not reported: %s", got)
	}
}

func TestDoDCommandMustBeRegistered(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "unregistered-dod")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "DoD command is not registered in any Suite: cd backend && go test ./not/a/suite/...") {
		t.Fatalf("unregistered DoD command not reported: %s", got)
	}
}

func TestRegisteredDoDCommandPasses(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	got := findings(root)
	if len(got) != 0 {
		t.Fatalf("registered DoD command should pass: %s", strings.Join(got, "\n"))
	}
}

func TestCommandSegmentsNormalizeCDAndWhitespace(t *testing.T) {
	got := commandSegments("cd backend &&  go test   ./database/...")
	want := "backend | go test ./database/..."
	if len(got) != 1 || got[0] != want {
		t.Fatalf("commandSegments() = %v, want %q", got, want)
	}
}

func TestParseJobsIgnoresOtherTopLevelKeys(t *testing.T) {
	jobs := parseJobs(strings.Join([]string{
		"name: x",
		"on: push",
		"jobs:",
		"  one:",
		"    runs-on: ubuntu-latest",
		"  two:",
		"    steps:",
		"      - run: go test ./...",
		"after: jobs",
	}, "\n"))
	if len(jobs) != 2 {
		t.Fatalf("parseJobs() = %v", jobs)
	}
	if !strings.Contains(jobs["two"], "go test ./...") {
		t.Fatalf("job two block = %q", jobs["two"])
	}
}

func findingsForSuites(root string, suites []Suite) []string {
	var out []string
	for _, f := range checkSuites(root, nil, suites) {
		out = append(out, f.String())
	}
	return out
}
