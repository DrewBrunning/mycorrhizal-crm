// Package e2einstall pins the e2e-tests.yml Playwright browser-install
// hardening (issue #1549): every "Install Playwright browsers" step must be
// time-bounded, retried, and keyed on the browsers it installs so one job's
// cache entry cannot shadow another job's different browser set. The retry
// lives in .github/scripts/playwright-install-retry.sh; the step timeout must
// cover that script's worst case, computed here from its budget lines, so the
// two cannot drift apart (they did: an 8 min step around a 930 s loop). Test-only.
package e2einstall

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	workflowRel = "../../../.github/workflows/e2e-tests.yml"
	scriptRel   = "../../../.github/scripts/playwright-install-retry.sh"
)

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

var (
	installRe    = regexp.MustCompile(`^bash \.\./\.github/scripts/playwright-install-retry\.sh ([a-z ]+)$`)
	budgetLineRe = regexp.MustCompile(`(?m)^(attempts|per_attempt|kill_after|backoff|lock_wait)=(\d+)$`)
)

// worstCaseSeconds reads the script's budget lines and returns the longest an
// install can take: every attempt runs to its timeout plus kill grace, and
// every gap waits the full backoff and lock wait.
func worstCaseSeconds(t *testing.T, script string) int {
	t.Helper()
	v := map[string]int{}
	for _, m := range budgetLineRe.FindAllStringSubmatch(script, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatal(err)
		}
		v[m[1]] = n
	}
	for _, k := range []string{"attempts", "per_attempt", "kill_after", "backoff", "lock_wait"} {
		if _, ok := v[k]; !ok {
			t.Fatalf("install script has no %s=<n> budget line", k)
		}
	}
	if v["attempts"] < 2 {
		t.Fatalf("install script must retry: attempts=%d", v["attempts"])
	}
	return v["attempts"]*(v["per_attempt"]+v["kill_after"]) + (v["attempts"]-1)*(v["backoff"]+v["lock_wait"])
}

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
	script, err := os.ReadFile(scriptRel)
	if err != nil {
		t.Fatal(err)
	}
	worst := worstCaseSeconds(t, string(script))
	for _, want := range []string{"timeout --kill-after=", "pkill -9 -x apt-get", "fuser /var/lib/dpkg/lock-frontend", "exit 1"} {
		if !strings.Contains(string(script), want) {
			t.Errorf("install script missing %q", want)
		}
	}
	found := 0
	for id, j := range w.Jobs {
		for i, s := range j.Steps {
			if s.Name != "Install Playwright browsers" {
				continue
			}
			found++
			m := installRe.FindStringSubmatch(strings.TrimSpace(s.Run))
			if m == nil {
				t.Errorf("%s: install step must run the retry script, got %q", id, s.Run)
				continue
			}
			browsers := strings.Fields(m[1])
			if s.TimeoutMinutes*60 < worst {
				t.Errorf("%s: timeout-minutes = %d (%ds) cannot cover the install script's worst case %ds; later attempts would never run", id, s.TimeoutMinutes, s.TimeoutMinutes*60, worst)
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
