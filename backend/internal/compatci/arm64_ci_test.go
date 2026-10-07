package compatci

// Issue #1485: no test used to execute the arm64 image or the arm64 suite.
// arm64-tests.yml and docker-publish.yml's post-publish-smoke arm64 leg now
// do; these guards fail if a later edit quietly turns them back into amd64
// (a runner label swapped to ubuntu-latest, the matrix leg dropped) or turns
// the cheap smoke into a -race/coverage run. They are structural: only CI
// can prove the arm64 job is green.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

const armRunner = "ubuntu-24.04-arm"

type armWorkflow struct {
	On   map[string]any `yaml:"on"`
	Jobs map[string]struct {
		RunsOn   any `yaml:"runs-on"`
		Strategy struct {
			Matrix map[string]any `yaml:"matrix"`
		} `yaml:"strategy"`
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func parseArmWorkflow(src []byte) (*armWorkflow, error) {
	var w armWorkflow
	if err := yaml.Unmarshal(src, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

// armTestsFindings checks arm64-tests.yml's contract.
func armTestsFindings(src []byte) []string {
	w, err := parseArmWorkflow(src)
	if err != nil {
		return []string{"arm64-tests.yml does not parse: " + err.Error()}
	}
	var out []string
	if _, ok := w.On["workflow_call"]; !ok {
		out = append(out, "arm64-tests.yml lost its workflow_call trigger (release-validate.yml composes it)")
	}
	job, ok := w.Jobs["arm64-go"]
	if !ok {
		return append(out, "arm64-tests.yml has no arm64-go job")
	}
	if job.RunsOn != armRunner {
		out = append(out, fmt.Sprintf("arm64-go runs-on %v, want %s", job.RunsOn, armRunner))
	}
	for _, s := range job.Steps {
		if strings.Contains(s.Run, "-race") {
			out = append(out, "arm64-go uses -race; it is a cheap no-race smoke by design")
		}
		if strings.Contains(s.Run, "-coverprofile") {
			out = append(out, "arm64-go collects coverage; amd64 owns coverage by design")
		}
	}
	// The leg list is a fromJSON expression (PR: rest only; otherwise both),
	// so assert on its text: both leg names must appear.
	legExpr := fmt.Sprint(job.Strategy.Matrix["leg"])
	for _, want := range []string{`"rest"`, `"controllers"`} {
		if !strings.Contains(legExpr, want) {
			out = append(out, "arm64-go matrix lost leg "+want)
		}
	}
	return out
}

// publishSmokeFindings checks the post-publish-smoke arm64 leg.
func publishSmokeFindings(src []byte) []string {
	w, err := parseArmWorkflow(src)
	if err != nil {
		return []string{"docker-publish.yml does not parse: " + err.Error()}
	}
	job, ok := w.Jobs["post-publish-smoke"]
	if !ok {
		return []string{"docker-publish.yml has no post-publish-smoke job"}
	}
	var out []string
	if job.RunsOn != "${{ matrix.runner }}" {
		out = append(out, fmt.Sprintf("post-publish-smoke runs-on %v, want the matrix runner", job.RunsOn))
	}
	inc, _ := job.Strategy.Matrix["include"].([]any)
	arm := false
	for _, e := range inc {
		if m, ok := e.(map[string]any); ok && m["runner"] == armRunner && m["arch"] == "arm64" {
			arm = true
		}
	}
	if !arm {
		out = append(out, "post-publish-smoke matrix has no arm64 leg on "+armRunner)
	}
	return out
}

func readWorkflowFile(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(workflowsDirRel, name))
	require.NoError(t, err)
	return b
}

func TestARM64WorkflowsAreReallyArm64(t *testing.T) {
	assert.Empty(t, armTestsFindings(readWorkflowFile(t, "arm64-tests.yml")))
	assert.Empty(t, publishSmokeFindings(readWorkflowFile(t, "docker-publish.yml")))
}

func TestARM64FindingsMutations(t *testing.T) {
	arm := string(readWorkflowFile(t, "arm64-tests.yml"))
	pub := string(readWorkflowFile(t, "docker-publish.yml"))

	cases := []struct {
		name   string
		fn     func([]byte) []string
		src    string
		mutate func(string) string
		want   string
	}{
		{"runner swapped to amd64", armTestsFindings, arm,
			func(s string) string { return strings.Replace(s, "runs-on: "+armRunner, "runs-on: ubuntu-latest", 1) }, "runs-on"},
		{"race flag added", armTestsFindings, arm,
			func(s string) string {
				return strings.Replace(s, "go test \"${pkgs[@]}\"", "go test -race \"${pkgs[@]}\"", 1)
			}, "-race"},
		{"coverage added", armTestsFindings, arm,
			func(s string) string {
				return strings.Replace(s, "go test \"${pkgs[@]}\"", "go test -coverprofile=c.out \"${pkgs[@]}\"", 1)
			}, "coverage"},
		{"workflow_call dropped", armTestsFindings, arm,
			func(s string) string { return strings.Replace(s, "  workflow_call:\n", "", 1) }, "workflow_call"},
		{"controllers leg dropped", armTestsFindings, arm,
			func(s string) string { return strings.Replace(s, `"rest","controllers"`, `"rest"`, 1) }, "controllers"},
		{"job renamed", armTestsFindings, arm,
			func(s string) string { return strings.Replace(s, "  arm64-go:", "  other:", 1) }, "no arm64-go job"},
		{"unparseable", armTestsFindings, "a: [",
			func(s string) string { return s }, "does not parse"},
		{"smoke runs-on pinned amd64", publishSmokeFindings, pub,
			func(s string) string {
				return strings.Replace(s, "runs-on: ${{ matrix.runner }}\n    strategy:\n      fail-fast: false\n      matrix:\n        include:\n          - arch: amd64",
					"runs-on: ubuntu-latest\n    strategy:\n      fail-fast: false\n      matrix:\n        include:\n          - arch: amd64", 1)
			}, "want the matrix runner"},
		{"smoke arm leg dropped", publishSmokeFindings, pub,
			func(s string) string {
				return strings.Replace(s, "            runner: "+armRunner, "            runner: ubuntu-latest", 1)
			}, "no arm64 leg"},
		{"smoke job missing", publishSmokeFindings, pub,
			func(s string) string { return strings.Replace(s, "  post-publish-smoke:", "  renamed-smoke:", 1) }, "no post-publish-smoke job"},
		{"smoke unparseable", publishSmokeFindings, "a: [",
			func(s string) string { return s }, "does not parse"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mutated := c.mutate(c.src)
			if c.src != "a: [" {
				require.NotEqual(t, c.src, mutated, "mutation must change the source (stale needle)")
			}
			got := strings.Join(c.fn([]byte(mutated)), "\n")
			assert.Contains(t, got, c.want)
		})
	}
}
