package compatci

// Issue #1906: backend/.golangci.yml is the enforcement surface for the
// golangci-lint linters. A linter quietly dropped from that file stops running
// in the required Backend (Go) job and the pre-commit hook, and nothing else
// fails — so the adopted set and the recorded rejections are pinned here.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type lintConfig struct {
	Linters struct {
		Default    string   `yaml:"default"`
		Enable     []string `yaml:"enable"`
		Exclusions struct {
			Rules []struct {
				Path    string   `yaml:"path"`
				Linters []string `yaml:"linters"`
			} `yaml:"rules"`
		} `yaml:"exclusions"`
	} `yaml:"linters"`
}

func readLintConfig(t *testing.T) (lintConfig, string) {
	t.Helper()
	path := filepath.Join(workflowsDirRel, "..", "..", "backend", ".golangci.yml")
	b, err := os.ReadFile(path)
	require.NoError(t, err, "backend/.golangci.yml must exist")
	var cfg lintConfig
	require.NoError(t, yaml.Unmarshal(b, &cfg), "backend/.golangci.yml must parse")
	return cfg, string(b)
}

func TestGolangciConfigEnablesAdoptedLinters(t *testing.T) {
	cfg, raw := readLintConfig(t)
	assert.Equal(t, "none", cfg.Linters.Default,
		"linters.default must stay none: only the reviewed explicit set may run")

	enabled := make(map[string]bool, len(cfg.Linters.Enable))
	for _, l := range cfg.Linters.Enable {
		enabled[l] = true
	}

	// The #1906 spike adopted these on top of the pre-existing set. Removing
	// one is a deliberate gate-weakening change, not an incidental edit.
	for _, l := range []string{
		"govet", "gosec", "bodyclose", "errcheck", "errorlint",
		"ineffassign", "staticcheck", "unused",
		"exhaustive", "rowserrcheck", "sqlclosecheck", "nilerr",
		"bidichk", "durationcheck", "copyloopvar", "unconvert", "misspell",
	} {
		assert.True(t, enabled[l], "%s must be enabled (issue #1906 adoption)", l)
	}

	// Rejected in the same spike, each with a recorded reason in the header.
	for _, l := range []string{"forcetypeassert", "noctx", "contextcheck"} {
		assert.False(t, enabled[l], "%s was deliberately rejected in issue #1906", l)
	}

	// The noctx/contextcheck deferral must name its owner issue.
	assert.Contains(t, raw, "#1939",
		"the noctx/contextcheck deferral must link the context-propagation issue (#1939)")
	assert.Contains(t, raw, "forcetypeassert",
		"the header must record why forcetypeassert is not enabled")
}

func TestGolangciConfigTestExemptionsMatchThePolicy(t *testing.T) {
	cfg, _ := readLintConfig(t)

	var testExempt map[string]bool
	for _, rule := range cfg.Linters.Exclusions.Rules {
		if rule.Path != `(_test\.go)$` {
			continue
		}
		testExempt = make(map[string]bool, len(rule.Linters))
		for _, l := range rule.Linters {
			testExempt[l] = true
		}
	}
	require.NotNil(t, testExempt, "the test-file exclusion rule must exist")

	// Correctness linters stay off tests: an intentional partial switch or a
	// deliberately leaked rows fixture would otherwise need a nolint per case.
	for _, l := range []string{
		"errcheck", "errorlint", "ineffassign", "staticcheck", "unused",
		"exhaustive", "rowserrcheck", "sqlclosecheck", "nilerr", "durationcheck",
	} {
		assert.True(t, testExempt[l], "%s must stay exempt on test files", l)
	}

	// These were fixed mechanically across the whole tree, tests included, so
	// they keep guarding test code (see the config header).
	for _, l := range []string{"copyloopvar", "unconvert", "misspell"} {
		assert.False(t, testExempt[l], "%s must run over test files too", l)
	}
}

// Guard the comment marker itself: the header is the record of what was
// rejected and why, which the issue's acceptance criteria require.
func TestGolangciConfigKeepsTheRejectedLinterRecord(t *testing.T) {
	_, raw := readLintConfig(t)
	assert.True(t, strings.Contains(raw, "Deliberately NOT enabled:"),
		"the header must keep its Deliberately NOT enabled section")
	assert.True(t, strings.Contains(raw, "588 findings on main"),
		"the forcetypeassert rejection must keep its measured finding count")
}
