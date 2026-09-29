package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	ghAPICall     = regexp.MustCompile(`\bgh api\b`)
	ghAPIField    = regexp.MustCompile(`\s-[fF]\s`)
	ghAPIMethod   = regexp.MustCompile(`\s(-X|--method)[\s=]`)
	shellContLine = regexp.MustCompile(`\\\n\s*`)
)

// gh api silently switches to POST as soon as -f/-F is given. A read-only
// lookup that passes query parameters that way 404s (or worse) at runtime --
// nightly-failure-reconcile.yml did exactly that for every workflow. Any
// `gh api` call with -f/-F must therefore state its method explicitly.
func TestGhAPIFieldsAlwaysStateAMethod(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, pat := range []string{".github/workflows/*.yml", ".github/scripts/*/*.sh"} {
		m, _ := filepath.Glob(filepath.Join(root, pat))
		files = append(files, m...)
	}
	if len(files) == 0 {
		t.Fatal("found no workflow/script files to scan")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, stmt := range strings.Split(shellContLine.ReplaceAllString(string(b), " "), "\n") {
			if ghAPICall.MatchString(stmt) && ghAPIField.MatchString(stmt) && !ghAPIMethod.MatchString(stmt) {
				t.Errorf("%s: `gh api` with -f/-F but no -X/--method (defaults to POST): %s", f, strings.TrimSpace(stmt))
			}
		}
	}
}

// Delivering an intent to the rule's singleTask MainActivity via
// targetContext.startActivity(FLAG_ACTIVITY_NEW_TASK) leaves the scenario's
// activity PAUSED, so ActivityScenarioRule teardown fails (nightly 36556357772).
// E2E tests must use E2eBaseTest.deliverToRunningActivity instead.
func TestAndroidE2eDoesNotDeliverIntentsViaNewTask(t *testing.T) {
	root, err := findRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(root, "android/app/src/androidTest/kotlin/com/mycorrhizal/crm/e2e/*.kt"))
	if len(files) == 0 {
		t.Fatal("found no android e2e tests to scan")
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(b), "\n") {
			// KDoc/line comments legitimately describe the cross-task path.
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, "FLAG_ACTIVITY_NEW_TASK") {
				t.Errorf("%s: use deliverToRunningActivity(intent), not FLAG_ACTIVITY_NEW_TASK", f)
				break
			}
		}
	}
}
