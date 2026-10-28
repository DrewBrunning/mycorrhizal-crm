package reasoncode

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// DefaultAllowlist is the allowlist path relative to the backend module root,
// where cmd/reasoncodecheck runs.
const DefaultAllowlist = "internal/lint/reasoncode/allowlist.txt"

// Run is the body of cmd/reasoncodecheck: it loads the package patterns,
// counts Analyzer diagnostics per file (relative to the working directory),
// and either rewrites the allowlist (-write-allowlist) or compares against it.
// It returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("reasoncodecheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	allowPath := fs.String("allowlist", DefaultAllowlist, "counted allowlist file")
	write := fs.Bool("write-allowlist", false, "rewrite the allowlist from the current counts")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	patterns := fs.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	counts, err := Count(patterns)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "reasoncodecheck:", err)
		return 2
	}

	if *write {
		f, err := os.Create(*allowPath)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "reasoncodecheck:", err)
			return 2
		}
		werr := WriteAllowlist(f, counts)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			_, _ = fmt.Fprintln(stderr, "reasoncodecheck:", werr)
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "reasoncodecheck: wrote %s (%d files)\n", *allowPath, len(counts))
		return 0
	}

	f, err := os.Open(*allowPath)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "reasoncodecheck:", err)
		return 2
	}
	defer func() { _ = f.Close() }()
	allow, err := ParseAllowlist(f)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "reasoncodecheck:", err)
		return 2
	}
	problems := Compare(counts, allow)
	for _, p := range problems {
		_, _ = fmt.Fprintln(stderr, p)
	}
	if len(problems) > 0 {
		return 1
	}
	return 0
}

// Count loads the patterns and returns the diagnostic count per file.
func Count(patterns []string) (map[string]int, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	pkgs, err := packages.Load(&packages.Config{Mode: packages.LoadAllSyntax}, patterns...)
	if err != nil {
		return nil, err
	}
	var loadErr error
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		if len(p.Errors) > 0 && loadErr == nil {
			loadErr = fmt.Errorf("%s: %v", p.PkgPath, p.Errors[0])
		}
	})
	if loadErr != nil {
		return nil, loadErr
	}
	graph, err := checker.Analyze([]*analysis.Analyzer{Analyzer}, pkgs, nil)
	if err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, act := range graph.Roots {
		if act.Err != nil {
			return nil, act.Err
		}
		for _, d := range act.Diagnostics {
			name := act.Package.Fset.Position(d.Pos).Filename
			if rel, err := filepath.Rel(cwd, name); err == nil {
				name = rel
			}
			counts[filepath.ToSlash(name)]++
		}
	}
	return counts, nil
}
