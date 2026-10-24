// Command contextcheck validates the AGENTS.md hierarchy (issue #1955): every
// registered file exists, CLAUDE.md aliases resolve, files stay within the
// 200-line hard maximum, and the registry matches the AGENTS.md files on disk.
//
// It validates only the AGENTS.md files. Skill/reference/Suite checks land in a
// later stage. It is read-only: it never executes a command parsed out of prose
// and never rewrites a file.
//
// Output is one "path:line: message" finding per line; any finding exits 1 and
// a clean tree exits 0. -list prints the registry and budgets, and
// -explain <path> prints the AGENTS.md files that would load for a file.
//
// The command runs from the repository root or any subdirectory: it resolves
// the root by walking up to the directory containing .git and the root
// AGENTS.md. Use -root to point at a different tree (tests, fixtures).
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mycorrhizal/internal/agentcontext"
	"mycorrhizal/internal/mdref"
)

func main() { // # pragma: no cover — thin os.Exit wrapper over run, which tests exercise
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("contextcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var rootFlag, explain string
	var list bool
	fs.StringVar(&rootFlag, "root", "", "repository root (default: walk up to .git and AGENTS.md)")
	fs.StringVar(&explain, "explain", "", "print the AGENTS.md files that load for `path`")
	fs.BoolVar(&list, "list", false, "list registered files and budgets")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: contextcheck [-root DIR] [-list] [-explain PATH]")
		fmt.Fprintln(stderr, "files may be between the 100-line target and the 200-line hard maximum; only the hard maximum fails")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	root := rootFlag
	if root == "" {
		found, err := agentcontext.FindRepoRoot(".")
		if err != nil {
			fmt.Fprintln(stderr, "contextcheck:", err)
			return 1
		}
		root = found
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(stderr, "contextcheck:", err) // # pragma: no cover — Abs fails only if the process cwd was deleted
		return 1                                   // # pragma: no cover — see above
	}

	if list {
		for _, af := range agentcontext.Registry() {
			if af.AliasOf != "" {
				fmt.Fprintf(stdout, "%s\talias=%s\n", af.Path, af.AliasOf)
				continue
			}
			fmt.Fprintf(stdout, "%s\ttarget=%d\tmax=%d\n", af.Path, af.TargetLines, af.MaxLines)
		}
		return 0
	}

	if explain != "" {
		chain, err := agentcontext.Explain(abs, resolveUserPath(abs, explain))
		if err != nil {
			fmt.Fprintln(stderr, "contextcheck:", err)
			return 1
		}
		for _, p := range chain {
			fmt.Fprintln(stdout, p)
		}
		return 0
	}

	findings := agentcontext.Check(abs)
	for _, f := range findings {
		fmt.Fprintln(stdout, f.String())
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

// resolveUserPath accepts a repo-relative path from the root, a path relative
// to the current directory (running from a subdirectory), or an absolute path.
// A repo-relative path wins when both exist.
func resolveUserPath(root, target string) string {
	if filepath.IsAbs(target) {
		return filepath.Clean(target)
	}
	repoRelative := filepath.Join(root, filepath.FromSlash(target))
	if mdref.PathExists(root, repoRelative) {
		return repoRelative
	}
	if cwd, err := os.Getwd(); err == nil {
		fromCWD := filepath.Join(cwd, filepath.FromSlash(target))
		if rel, err := filepath.Rel(root, fromCWD); err == nil &&
			rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fromCWD
		}
	}
	return repoRelative
}
