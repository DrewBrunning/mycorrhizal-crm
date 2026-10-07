// Command rawtimecheck runs the internal/lint/rawtime analyzer: it reports
// time.Now / time.Since / time.Until in the controllers, services and
// middleware packages outside the reason-bearing allowlist (issue #1494) —
// time-dependent logic must read an injected clock.Clock so tests can pin it.
//
//	cd backend && go run ./cmd/rawtimecheck ./...
//
// Test files are skipped by the analyzer. CI runs it in unit-tests.yml's
// backend-checks job (the required `Backend (Go)` check); the pre-commit hook
// runs it for staged backend/ files.
package main

import (
	"mycorrhizal/internal/lint/rawtime"

	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(rawtime.Analyzer) } // # pragma: no cover — singlechecker.Main parses os.Args and calls os.Exit itself; the analyzer logic it drives is tested directly in internal/lint/rawtime
