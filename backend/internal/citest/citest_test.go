package citest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSkipOrRequire_SkipsWhenEnvVarUnset(t *testing.T) {
	t.Setenv(RequireReferencesEnvVar, "")

	var sub *testing.T
	t.Run("inner", func(inner *testing.T) {
		sub = inner
		SkipOrRequire(inner, "reference tool not installed")
	})

	if !sub.Skipped() {
		t.Fatal("expected SkipOrRequire to skip when the env var is unset")
	}
	if sub.Failed() {
		t.Fatal("expected SkipOrRequire not to fail the test when merely skipping")
	}
}

// helperProcessEnvVar, when set, tells TestHelperProcess_FailsWhenRequired
// to actually run instead of being a no-op -- it's invoked only as a
// subprocess by TestSkipOrRequire_FailsWhenEnvVarSet below.
const helperProcessEnvVar = "CITEST_RUN_HELPER_PROCESS"

// TestHelperProcess_FailsWhenRequired is not meant to run as part of the
// normal suite (it's an intentional Fatalf); TestSkipOrRequire_FailsWhenEnvVarSet
// re-invokes `go test` targeting just this test, in a subprocess, so the
// Fatalf can be observed via exit code without failing this package's own
// `go test` run.
func TestHelperProcess_FailsWhenRequired(t *testing.T) {
	if os.Getenv(helperProcessEnvVar) == "" {
		t.Skip("only runs as a subprocess of TestSkipOrRequire_FailsWhenEnvVarSet")
	}
	SkipOrRequire(t, "reference tool not installed")
}

func TestSkipOrRequire_FailsWhenEnvVarSet(t *testing.T) {
	cmd := exec.Command("go", "test", "-run", "^TestHelperProcess_FailsWhenRequired$", "-v", ".")
	cmd.Env = append(os.Environ(),
		helperProcessEnvVar+"=1",
		RequireReferencesEnvVar+"=1",
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected the subprocess to fail (SkipOrRequire must Fatalf when %s is set); output:\n%s", RequireReferencesEnvVar, out)
	}
	if !strings.Contains(string(out), RequireReferencesEnvVar+" is set, but a required tool/reference is unavailable") {
		t.Fatalf("subprocess output missing the expected failure message:\n%s", out)
	}
}
