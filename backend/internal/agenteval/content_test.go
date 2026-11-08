package agenteval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeContentFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckContentPaths(t *testing.T) {
	root := t.TempDir()
	writeContentFile(t, root, "docs/trouble.md", "Run `make gen-schema-fixtures`. Never hand-edit the dump.\n")
	writeContentFile(t, root, "README-developer.md", "See the troubleshooting entry.\n")
	checks := []ContentCheck{{
		Name:        "names the regeneration command",
		Paths:       []string{"docs/**", "README-developer.md"},
		ContainsAll: []string{"gen-schema-fixtures", "hand-edit"},
	}}
	findings := CheckContent(root, []string{"docs/trouble.md", "README-developer.md"}, checks)
	if len(findings) != 0 {
		t.Fatalf("findings = %v", findings)
	}

	checks[0].ContainsAny = []string{"drift", "mismatch"}
	findings = CheckContent(root, []string{"docs/trouble.md", "README-developer.md"}, checks)
	if len(findings) != 1 || !strings.Contains(findings[0], "none of") {
		t.Fatalf("findings = %v", findings)
	}
}

func TestCheckContentForbidsAndNoMatch(t *testing.T) {
	root := t.TempDir()
	writeContentFile(t, root, ".github/workflows/weekly.yml", "on:\n  schedule:\n  pull_request:\n")
	checks := []ContentCheck{{
		Name:    "adds a scheduled gate without a PR trigger",
		Paths:   []string{".github/workflows/**"},
		Forbids: []string{"pull_request"},
	}}
	findings := CheckContent(root, []string{".github/workflows/weekly.yml"}, checks)
	if len(findings) != 1 || !strings.Contains(findings[0], "forbidden text") {
		t.Fatalf("findings = %v", findings)
	}

	checks = []ContentCheck{{Name: "docs changed", Paths: []string{"docs/**"}, ContainsAll: []string{"x"}}}
	findings = CheckContent(root, []string{".github/workflows/weekly.yml"}, checks)
	if len(findings) != 1 || !strings.Contains(findings[0], "no changed file matched") {
		t.Fatalf("findings = %v", findings)
	}
}

func TestCheckContentFiles(t *testing.T) {
	root := t.TempDir()
	for _, locale := range []string{"en", "de"} {
		writeContentFile(t, root, "frontend/src/i18n/locales/"+locale+".json", `{"total_one":"1","total_other":"n"}`)
	}
	writeContentFile(t, root, "frontend/src/i18n/locales/fr.json", `{"total":"n"}`)
	checks := []ContentCheck{{
		Name:        "every locale has the plural family",
		Files:       []string{"frontend/src/i18n/locales/en.json", "frontend/src/i18n/locales/de.json", "frontend/src/i18n/locales/fr.json"},
		ContainsAll: []string{`"total_one"`, `"total_other"`},
	}}
	findings := CheckContent(root, []string{"frontend/src/i18n/locales/fr.json"}, checks)
	if len(findings) != 2 || !strings.Contains(findings[0], "fr.json") {
		t.Fatalf("findings = %v", findings)
	}
}

func TestCheckContentMissingFile(t *testing.T) {
	checks := []ContentCheck{{Name: "missing", Files: []string{"docs/nope.md"}, ContainsAll: []string{"x"}}}
	findings := CheckContent(t.TempDir(), nil, checks)
	if len(findings) != 1 || !strings.Contains(findings[0], "unreadable") {
		t.Fatalf("findings = %v", findings)
	}
}

func TestParseSpecValidatesContentChecks(t *testing.T) {
	base := "name: demo\nprompt: x\nbase_ref: " + sha + "\nsuite: database-migration\nagent_command: true\nexpect_paths: [backend/**]\n"
	cases := map[string]string{
		"no name":            base + "content_checks:\n  - paths: [docs/**]\n    contains_all: [x]\n",
		"no target":          base + "content_checks:\n  - name: x\n    contains_all: [x]\n",
		"paths and files":    base + "content_checks:\n  - name: x\n    paths: [docs/**]\n    files: [README.md]\n    contains_all: [x]\n",
		"no assertion":       base + "content_checks:\n  - name: x\n    paths: [docs/**]\n",
		"bad glob":           base + "content_checks:\n  - name: x\n    paths: ['[']\n    contains_all: [x]\n",
		"absolute file path": base + "content_checks:\n  - name: x\n    files: [/etc/passwd]\n    contains_all: [x]\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseSpec([]byte(body), t.TempDir(), "demo", []string{"database-migration"}); err == nil || !strings.Contains(err.Error(), "content_checks") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestRunFailsOnContentCheck(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	spec.ExpectPaths = []string{"docs/**"}
	spec.ContentChecks = []ContentCheck{{
		Name:        "names the command",
		Paths:       []string{"docs/**"},
		ContainsAll: []string{"genschema"},
	}}
	artifact, err := Run(context.Background(), Options{
		Root:         root,
		Spec:         spec,
		Suites:       []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand: "true",
		CommandRunner: func(_ context.Context, dir, _ string, _ time.Duration) CommandResult {
			writeRepoFile(t, dir, "docs/trouble.md", "Run the tool.\n")
			return CommandResult{ExitCode: 0}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "failed" || artifact.Context == nil || artifact.Context.Success {
		t.Fatalf("artifact = %+v", artifact)
	}
	if len(artifact.Context.ContentCheckFindings) != 1 || !strings.Contains(artifact.Context.ContentCheckFindings[0], "genschema") {
		t.Fatalf("findings = %v", artifact.Context.ContentCheckFindings)
	}
}

func TestCheckContentEdgeCases(t *testing.T) {
	root := t.TempDir()
	writeContentFile(t, root, "docs/case.md", "Drift\n")
	checks := []ContentCheck{{Name: "case sensitive", Paths: []string{"docs/**"}, ContainsAll: []string{"drift"}, CaseSensitive: true}}
	if findings := CheckContent(root, []string{"docs/case.md"}, checks); len(findings) != 1 {
		t.Fatalf("case-sensitive findings = %v", findings)
	}
	checks[0].CaseSensitive = false
	if findings := CheckContent(root, []string{"docs/case.md"}, checks); len(findings) != 0 {
		t.Fatalf("case-insensitive findings = %v", findings)
	}

	writeContentFile(t, root, "docs/binary.md", "a\x00b")
	checks = []ContentCheck{{Name: "binary", Paths: []string{"docs/binary.md"}, ContainsAll: []string{"a"}}}
	if findings := CheckContent(root, []string{"docs/binary.md"}, checks); len(findings) == 0 || !strings.Contains(strings.Join(findings, "\n"), "not a text file") {
		t.Fatalf("binary findings = %v", findings)
	}

	large := strings.Repeat("a", maxContentCheckFileBytes+1)
	writeContentFile(t, root, "docs/large.md", large)
	checks = []ContentCheck{{Name: "large", Paths: []string{"docs/large.md"}, ContainsAll: []string{"a"}}}
	if findings := CheckContent(root, []string{"docs/large.md"}, checks); len(findings) == 0 || !strings.Contains(strings.Join(findings, "\n"), "larger than") {
		t.Fatalf("large findings = %v", findings)
	}

	if err := os.MkdirAll(filepath.Join(root, "docs/dir.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	checks = []ContentCheck{{Name: "directory", Paths: []string{"docs/dir.md"}, ContainsAll: []string{"a"}}}
	if findings := CheckContent(root, []string{"docs/dir.md"}, checks); len(findings) == 0 || !strings.Contains(strings.Join(findings, "\n"), "not a regular file") {
		t.Fatalf("directory findings = %v", findings)
	}

	checks = []ContentCheck{{Name: "unreadable", Paths: []string{"docs/missing.md"}, ContainsAll: []string{"a"}}}
	if findings := CheckContent(root, []string{"docs/missing.md"}, checks); len(findings) == 0 || !strings.Contains(strings.Join(findings, "\n"), "unreadable") {
		t.Fatalf("missing findings = %v", findings)
	}
}

func TestValidateContentChecksErrors(t *testing.T) {
	cases := []struct {
		name  string
		check ContentCheck
	}{
		{"duplicate", ContentCheck{Name: "x", Paths: []string{"docs/**"}, ContainsAll: []string{"x"}}},
		{"bad file", ContentCheck{Name: "x", Files: []string{"docs/../x"}, ContainsAll: []string{"x"}}},
		{"empty assertion", ContentCheck{Name: "x", Paths: []string{"docs/**"}, ContainsAll: []string{" "}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checks := []ContentCheck{tc.check}
			if tc.name == "duplicate" {
				checks = append(checks, tc.check)
			}
			if err := ValidateContentChecks(checks); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

// A workflow that explains in a full-line YAML comment that it has no
// pull_request trigger is correct; only an actual key may trip the forbid
// (#2426: the nightly add-quality-gate eval failed on exactly this comment).
func TestCheckContentIgnoreComments(t *testing.T) {
	root := t.TempDir()
	writeContentFile(t, root, ".github/workflows/weekly.yml",
		"# deliberately has no pull_request trigger; runs depexceptions\non:\n  schedule:\n    - cron: '0 7 * * 1' # not pull_request\n")
	changed := []string{".github/workflows/weekly.yml"}
	check := ContentCheck{
		Name:           "scheduled gate without a PR trigger",
		Paths:          []string{".github/workflows/**"},
		ContainsAll:    []string{"schedule:"},
		Forbids:        []string{"pull_request"},
		IgnoreComments: true,
	}
	if findings := CheckContent(root, changed, []ContentCheck{check}); len(findings) != 1 || !strings.Contains(findings[0], "forbidden text") {
		// the trailing "# not pull_request" is code-side text and stays visible
		t.Fatalf("trailing comment must still be scanned conservatively, findings = %v", findings)
	}

	writeContentFile(t, root, ".github/workflows/weekly.yml",
		"# deliberately has no pull_request trigger; runs depexceptions\non:\n  schedule:\n    - cron: '0 7 * * 1'\n")
	if findings := CheckContent(root, changed, []ContentCheck{check}); len(findings) != 0 {
		t.Fatalf("full-line comment must be ignored, findings = %v", findings)
	}

	// A mention that lives only in a comment must not satisfy contains_all.
	check.ContainsAll = []string{"depexceptions"}
	if findings := CheckContent(root, changed, []ContentCheck{check}); len(findings) != 1 || !strings.Contains(findings[0], "missing text") {
		t.Fatalf("comment-only mention satisfied contains_all, findings = %v", findings)
	}

	// Without the option, behaviour is unchanged.
	check.IgnoreComments = false
	check.ContainsAll = nil
	if findings := CheckContent(root, changed, []ContentCheck{check}); len(findings) != 1 {
		t.Fatalf("default must stay literal, findings = %v", findings)
	}
}
