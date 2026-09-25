// Package citest is a shared seam for "skip when a required external tool
// is missing" tests. Skipping on absence is right for a developer machine
// that hasn't installed the reference/tool the test needs — it keeps `go
// test ./...` green there. But several CI jobs are *supposed* to provide
// that tool (install python3+vobject, build the calcard binary, run `make`,
// stand up a real CardDAV/vdirsyncer server), and a broken provisioning step
// in one of those jobs turns the corresponding gate into a silent no-op:
// the leg reports green having tested nothing.
//
// RequireReferencesEnvVar names the env var those CI jobs set so that
// specific failure mode becomes a hard failure instead. It lives in its own
// package (mirroring internal/dbtest and internal/rfctest) so that importing
// `testing` stays out of any package's production code — every caller here
// is itself test-only infrastructure.
package citest

import (
	"os"
	"testing"
)

// RequireReferencesEnvVar, when set to a non-empty value, means the calling
// CI job promises every external tool/reference these tests need is
// present. SkipOrRequire then turns a missing one into a hard failure.
const RequireReferencesEnvVar = "MYCORRHIZAL_REQUIRE_REFERENCES"

// SkipOrRequire is t.Skip(reason), unless RequireReferencesEnvVar is set, in
// which case it is t.Fatalf: the caller's environment promised this tool or
// reference would be available, and it wasn't.
func SkipOrRequire(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv(RequireReferencesEnvVar) != "" {
		t.Fatalf("%s is set, but a required tool/reference is unavailable: %s", RequireReferencesEnvVar, reason) // # pragma: no cover — t.Fatalf calls runtime.Goexit; exercised via the subprocess in TestSkipOrRequire_FailsWhenEnvVarSet, whose coverage isn't merged into this process's profile
	}
	t.Skip(reason)
}
