package agenteval

import (
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"mycorrhizal/internal/agentcontext"
)

// Documentation-maintenance's nightly failure (#2276) was an extra link from
// backend/database/testdata/schemas/README.md: the suite still passed, so only
// the scope check caught it. Pin the task's allowed/forbidden paths and the
// prompt sentence that names the generated schema directory; widening either
// would make the same failure possible again.
func documentationMaintenanceScopeFindings(spec Spec) []string {
	var findings []string

	allowed := map[string]bool{"docs/**": true, "README-developer.md": true}
	if len(spec.ExpectPaths) != len(allowed) {
		findings = append(findings, "expect_paths must be exactly docs/** and README-developer.md")
	}
	for _, path := range spec.ExpectPaths {
		if !allowed[path] {
			findings = append(findings, "unexpected expect_path: "+path)
		}
	}

	requiredForbid := []string{"backend/**", "frontend/**", "android/**", ".github/**"}
	haveForbid := map[string]bool{}
	for _, path := range spec.ForbidPaths {
		haveForbid[path] = true
	}
	for _, path := range requiredForbid {
		if !haveForbid[path] {
			findings = append(findings, "missing forbid_path: "+path)
		}
	}

	prompt := strings.ToLower(spec.Prompt)
	for _, want := range []string{"docs/", "readme-developer.md", "schema-fixture readme", "backend/"} {
		if !strings.Contains(prompt, want) {
			findings = append(findings, "prompt does not name "+want)
		}
	}

	sort.Strings(findings)
	return findings
}

func TestDocumentationMaintenanceScopeIsPinned(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	suites := agentcontext.Suites()
	names := make([]string, 0, len(suites))
	for _, s := range suites {
		names = append(names, s.Name)
	}
	spec, err := LoadTask(root, "testdata/agent-eval", "documentation-maintenance", names)
	if err != nil {
		t.Fatal(err)
	}
	if findings := documentationMaintenanceScopeFindings(spec); len(findings) > 0 {
		t.Fatalf("documentation-maintenance scope is not pinned:\n%s", strings.Join(findings, "\n"))
	}
}

func TestDocumentationMaintenanceScopeFindings(t *testing.T) {
	valid := Spec{
		ExpectPaths: []string{"docs/**", "README-developer.md"},
		ForbidPaths: []string{"backend/**", "frontend/**", "android/**", ".github/**"},
		Prompt:      "Change only files under `docs/` and `README-developer.md`. Do not edit anything under `backend/` — even a README there, including the schema-fixture README.",
	}
	if findings := documentationMaintenanceScopeFindings(valid); len(findings) != 0 {
		t.Fatalf("valid spec findings = %v", findings)
	}

	widened := valid
	widened.ExpectPaths = append(append([]string{}, valid.ExpectPaths...), "backend/**")
	if findings := documentationMaintenanceScopeFindings(widened); len(findings) == 0 {
		t.Fatal("widened expect_paths produced no finding")
	}

	missingForbid := valid
	missingForbid.ForbidPaths = []string{"frontend/**", "android/**", ".github/**"}
	if findings := documentationMaintenanceScopeFindings(missingForbid); len(findings) == 0 {
		t.Fatal("missing backend forbid_path produced no finding")
	}

	ambiguous := valid
	ambiguous.Prompt = "Add an operator troubleshooting entry to the deployment guide."
	if findings := documentationMaintenanceScopeFindings(ambiguous); len(findings) == 0 {
		t.Fatal("prompt without the explicit scope sentence produced no finding")
	}
}
