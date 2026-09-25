// Command gormerrcheck runs the internal/lint/gormerr analyzer: it reports
// GORM finisher calls (Create, Save, Updates, Delete, First, Find, Exec, ...)
// whose *gorm.DB result — and so its .Error — is discarded (CLAUDE.md
// backend trap #4). errcheck cannot see these because GORM returns *gorm.DB,
// not error.
//
//	cd backend && go run ./cmd/gormerrcheck ./...
//
// Test files are skipped by the analyzer. CI runs it in unit-tests.yml's
// backend-checks job; the pre-commit hook runs it for staged backend/ files.
package main

import (
	"mycorrhizal/internal/lint/gormerr"

	"golang.org/x/tools/go/analysis/singlechecker"
)

func main() { singlechecker.Main(gormerr.Analyzer) }
