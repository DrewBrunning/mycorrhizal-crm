// Command genschema regenerates the committed per-release schema dumps that
// back the supported-upgrade test matrix (MIG-01, issue #436; bounded by
// issue #529).
//
// The dumps live under backend/database/testdata/schemas/ — one schema-only
// SQL file per supported release (v0.6.0 and later), frozen: a historical
// schema never changes retroactively. They are generated from the current
// embedded migration chain, which is frozen and append-only, so migrations
// 000001..N are byte-identical to what the tagged release shipped.
//
// When a NEW release ships with a new migration, the workflow is:
//
//  1. add it to internal/schemafixture's SupportedReleases
//  2. run `go run ./cmd/genschema` (or `make gen-schema-fixtures`) from
//     backend/ — it regenerates every dump, and the reproducibility test
//     (TestSchemaDumpsReproduceCurrentChain) enforces step 2 in CI
//  3. review the fixture diff
//
// Exit status 0 means the dumps were regenerated in place; 1 means a release
// could not be generated; 2 means the command could not run.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/schemafixture"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

// run is split out of main so the exit paths are testable.
func run() int {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genschema:", err)
		return 2
	}

	dir := filepath.Join(root, schemafixture.SchemaDumpsDirRel)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintln(os.Stderr, "genschema:", err)
		return 2
	}

	for _, release := range schemafixture.SupportedReleases {
		dump, err := schemafixture.GenerateDump(release.Version, release.Tag)
		if err != nil {
			fmt.Fprintln(os.Stderr, "genschema:", err) // # pragma: no cover — every SupportedReleases version exists in the frozen chain, so GenerateDump cannot fail here
			return 1
		}
		path := filepath.Join(dir, schemafixture.DumpFile(release))
		if err := os.WriteFile(path, []byte(dump), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "genschema:", err)
			return 2
		}
		fmt.Printf("Generated %s (version %d)\n", schemafixture.DumpFile(release), release.Version)
	}
	fmt.Printf("Schema dumps written to %s\n", dir)
	return 0
}

// findRepoRoot walks up from the working directory until it finds the
// repository root (the directory containing backend/database/migrations), so
// the command works from backend/ (the documented invocation) and from
// anywhere below the repo.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		// # pragma: no cover — Getwd fails only when the cwd has been deleted,
		// which no test can arrange for its own process.
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "backend", "database", "migrations")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no repository root above %s (looked for backend/database/migrations)", dir)
		}
		dir = parent
	}
}
