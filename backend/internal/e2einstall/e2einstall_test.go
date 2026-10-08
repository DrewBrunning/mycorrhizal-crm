// Package e2einstall pins the e2e-tests.yml Playwright browser-install
// hardening (issue #1549): every "Install Playwright browsers" step must be
// time-bounded, retried, and keyed on the browsers it installs so one job's
// cache entry cannot shadow another job's different browser set. Test-only.
package e2einstall

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const workflowRel = "../../../.github/workflows/e2e-tests.yml"

type step struct {
	Name           string            `yaml:"name"`
	Run            string            `yaml:"run"`
	TimeoutMinutes int               `yaml:"timeout-minutes"`
	With           map[string]string `yaml:"with"`
}

type job struct {
	Steps []step `yaml:"steps"`
}

type workflow struct {
	Jobs map[string]job `yaml:"jobs"`
}

var installRe = regexp.MustCompile(`npx playwright install --with-deps ([a-z ]+)`)

func load(t *testing.T) workflow {
	t.Helper()
	b, err := os.ReadFile(workflowRel)
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(b, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestInstallStepsBoundedRetriedAndKeyed(t *testing.T) {
	w := load(t)
	found := 0
	for id, j := range w.Jobs {
		for i, s := range j.Steps {
			if s.Name != "Install Playwright browsers" {
				continue
			}
			found++
			m := installRe.FindStringSubmatch(s.Run)
			if m == nil {
				t.Errorf("%s: install step has no playwright install command", id)
				continue
			}
			browsers := strings.Fields(m[1])
			if s.TimeoutMinutes != 8 {
				t.Errorf("%s: timeout-minutes = %d, want 8", id, s.TimeoutMinutes)
			}
			for _, want := range []string{"for attempt in 1 2 3", "timeout 300 npx", "sleep 15", "exit 1", "&& exit 0"} {
				if !strings.Contains(s.Run, want) {
					t.Errorf("%s: install run missing %q", id, want)
				}
			}
			if i == 0 || !strings.HasPrefix(j.Steps[i-1].Name, "Cache Playwright browsers") {
				t.Errorf("%s: install step not preceded by the cache step", id)
				continue
			}
			key := j.Steps[i-1].With["key"]
			wantKey := "playwright-${{ runner.os }}-" + strings.Join(browsers, "-") + "-${{ hashFiles('frontend/yarn.lock') }}"
			if key != wantKey {
				t.Errorf("%s: cache key %q, want %q", id, key, wantKey)
			}
		}
	}
	if found != 6 {
		t.Errorf("found %d install steps, want 6", found)
	}
}
