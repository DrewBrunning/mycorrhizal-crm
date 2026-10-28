// Command reasoncodecheck runs the internal/lint/reasoncode analyzer as a
// counted ratchet (issue #2098, ADR 0038): a user-facing apperrors constructor
// (ErrInvalidInput, ErrConflict, ...) must be chained with .WithReason(...) so
// clients can localize it from reason_code. Sites not yet migrated are
// allowlisted per file by count; a file may not gain sites, and the count must
// be lowered as sites are migrated.
//
//	cd backend && go run ./cmd/reasoncodecheck ./...
//	cd backend && go run ./cmd/reasoncodecheck -write-allowlist ./...
//
// Test files are skipped. CI runs it in unit-tests.yml's backend-checks job;
// the pre-commit hook runs it for staged backend/ files.
package main

import (
	"os"

	"mycorrhizal/internal/lint/reasoncode"
)

func main() { os.Exit(reasoncode.Run(os.Args[1:], os.Stdout, os.Stderr)) } // # pragma: no cover — thin os.Exit wrapper; reasoncode.Run is tested directly in internal/lint/reasoncode
