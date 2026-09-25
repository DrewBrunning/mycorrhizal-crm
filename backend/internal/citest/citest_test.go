package citest

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const helperProcessEnvVar = "MYCORRHIZAL_CITEST_HELPER"

// TestSkipOrRequireHelperProcess is not a real test: it is re-executed as a
// subprocess (via `go test -run`) by the two tests below, so
// SkipOrRequire's t.Fatalf path can be observed by exit code without
// failing this package's own test run the way calling it in-process would.
func TestSkipOrRequireHelperProcess(t *testing.T) {
	if os.Getenv(helperProcessEnvVar) == "" {
		t.Skip("only runs as a subprocess helper; see TestSkipOrRequire_*")
	}
	SkipOrRequire(t, "tool unavailable for this test")
}

// TestSkipOrRequire_SkipsWhenEnvUnset is the default-developer-machine
// path: no RequireReferencesEnvVar set, so a missing tool is a skip.
func TestSkipOrRequire_SkipsWhenEnvUnset(t *testing.T) {
	out, err := runHelperProcess(t, false)
	require.NoErrorf(t, err, "output:\n%s", out)
	assert.Contains(t, string(out), "SKIP")
}

// TestSkipOrRequire_FailsWhenEnvSet proves SkipOrRequire actually turns a
// missing tool into a hard failure once the CI env var is set.
func TestSkipOrRequire_FailsWhenEnvSet(t *testing.T) {
	out, err := runHelperProcess(t, true)
	require.Errorf(t, err, "expected the subprocess to fail; output:\n%s", out)
	assert.Contains(t, string(out), "FAIL")
}

func runHelperProcess(t *testing.T, requireReferences bool) ([]byte, error) {
	t.Helper()
	// #nosec G204 -- os.Args[0] is this test binary re-executing itself with a fixed -test.run pattern, never request-controlled
	cmd := exec.Command(os.Args[0], "-test.run=^TestSkipOrRequireHelperProcess$", "-test.v")
	cmd.Env = append(os.Environ(), helperProcessEnvVar+"=1")
	if requireReferences {
		cmd.Env = append(cmd.Env, RequireReferencesEnvVar+"=1")
	} else {
		cmd.Env = append(cmd.Env, RequireReferencesEnvVar+"=")
	}
	return cmd.CombinedOutput()
}
