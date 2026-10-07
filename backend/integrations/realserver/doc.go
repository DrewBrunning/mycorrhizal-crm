// Package realserver holds the real-server contract tests (issue #1490): the
// suite that proves Mycorrhizal speaks each external integration's protocol
// correctly against the *actual* upstream server, not against a hand-written
// httptest fake that only encodes our reading of that protocol (the "shared
// misconception" failure mode ADR-0003 names for formats).
//
// Everything here is test-only. Each test locates its server through an
// environment variable (see the env* constants in helpers_test.go) and goes
// through internal/citest.SkipOrRequire: a developer machine without the
// server skips, but .github/workflows/integration-real-servers.yml sets
// MYCORRHIZAL_REQUIRE_REFERENCES=1, so a broken provisioning step fails the
// job instead of turning the gate into a green no-op.
//
// The servers are defined under stack/ and documented in
// docs/development/testing.md ("Real-server integration contract tests").
package realserver
