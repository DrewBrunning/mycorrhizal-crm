package e2einstall

import (
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const warmRel = "../../../.github/workflows/playwright-cache-warm.yml"

// The release-only e2e-sw-upgrade job installs chromium+firefox and was cold on
// both v1.6.0 release runs (issue #2392): its cache entries are only written by
// a green nightly of the e2e job it needs, and are evicted when unused. The
// playwright-cache-warm workflow warms the same entries on a schedule, so its
// cache steps must stay byte-identical to the job's -- this test is what stops
// the two key sets drifting apart.

type warmWorkflow struct {
	On   map[string]any `yaml:"on"`
	Jobs map[string]job `yaml:"jobs"`
}

func stepByName(t *testing.T, steps []step, name string) step {
	t.Helper()
	for _, s := range steps {
		if s.Name == name {
			return s
		}
	}
	t.Fatalf("no step named %q", name)
	return step{}
}

func TestWarmWorkflowCacheKeysMatchSWUpgradeJob(t *testing.T) {
	b, err := os.ReadFile(warmRel)
	if err != nil {
		t.Fatalf("read warm workflow: %v", err)
	}
	var warm warmWorkflow
	if err := yaml.Unmarshal(b, &warm); err != nil {
		t.Fatal(err)
	}
	if _, ok := warm.On["schedule"]; !ok {
		t.Error("warm workflow has no schedule: trigger; the caches would go cold again")
	}
	if _, ok := warm.On["workflow_dispatch"]; !ok {
		t.Error("warm workflow has no workflow_dispatch trigger")
	}
	wj, ok := warm.Jobs["warm"]
	if !ok {
		t.Fatal("warm workflow has no `warm` job")
	}
	sw, ok := load(t).Jobs["e2e-sw-upgrade"]
	if !ok {
		t.Fatal("e2e-tests.yml has no e2e-sw-upgrade job")
	}
	for _, name := range []string{"Detect runner image", "Cache apt packages for Playwright", "Cache Playwright browsers", "Install Playwright browsers"} {
		want, got := stepByName(t, sw.Steps, name), stepByName(t, wj.Steps, name)
		if got.Run != want.Run {
			t.Errorf("%s: run %q differs from e2e-sw-upgrade's %q", name, got.Run, want.Run)
		}
		if got.WorkingDir != want.WorkingDir {
			t.Errorf("%s: working-directory %q differs from e2e-sw-upgrade's %q", name, got.WorkingDir, want.WorkingDir)
		}
		for _, k := range []string{"path", "key", "restore-keys"} {
			if got.With[k] != want.With[k] {
				t.Errorf("%s: with.%s %q differs from e2e-sw-upgrade's %q", name, k, got.With[k], want.With[k])
			}
		}
	}
	// The warm-up must install exactly the browser set whose entries it keeps warm.
	inst := stepByName(t, wj.Steps, "Install Playwright browsers")
	if m := installRe.FindStringSubmatch(strings.TrimSpace(inst.Run)); m == nil || strings.Join(strings.Fields(m[1]), "-") != "chromium-firefox" {
		t.Errorf("warm install step %q must install exactly chromium firefox", inst.Run)
	}
}
