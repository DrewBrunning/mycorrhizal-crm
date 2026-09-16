// Command genmutationscope regenerates the gremlins exclude-files configs
// under backend/.gremlins/ from internal/mutationscope.Scopes (issue #915).
//
// The controllers-delete-cascade and services-import scopes narrow mutation
// testing to a few files inside much larger packages; the exclude list for
// every other file in those packages is generated here rather than
// hand-maintained, because RE2 (the regexp engine gremlins' exclude-files
// patterns use) has no negative lookahead to express "everything except
// these files" directly. TestGeneratedConfigsAreCurrent in
// internal/mutationscope fails until this is rerun after a file is added to
// or removed from a scoped package.
//
// Normal workflow:
//
//	cd backend && go run ./cmd/genmutationscope
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/internal/mutationscope"
)

func main() {
	os.Exit(run()) // # pragma: no cover — os.Exit terminates the process; tests exercise run() directly
}

func run() int {
	root, err := findBackendDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "genmutationscope:", err)
		return 2
	}
	if err := mutationscope.Write(root); err != nil {
		fmt.Fprintln(os.Stderr, "genmutationscope:", err)
		return 1
	}
	fmt.Printf("Wrote %d gremlins configs under %s\n", len(mutationscope.Scopes), filepath.Join(root, mutationscope.ConfigDir))
	return 0
}

func findBackendDir() (string, error) {
	dir, err := os.Getwd()
	if err != nil { // # pragma: no cover — Getwd fails only when the cwd has been deleted, which no test can arrange for its own process
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("backend module root (go.mod) not found above %s", dir)
		}
		dir = parent
	}
}
