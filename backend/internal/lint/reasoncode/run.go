package reasoncode

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"mycorrhizal/internal/versionpolicy"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"
)

// ZeroFromRelease is the first release that may not carry any unmigrated
// user-facing error site (issue #2093, milestone v1.6.0). From this release
// onward release.yml runs reasoncodecheck with -release-tag, and a non-empty
// allowlist blocks the tag. Releases before it only need the ratchet to hold.
const ZeroFromRelease = "v1.6.0"

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
	releaseTag := fs.String("release-tag", "", "release being cut (vX.Y.Z[-rc.N]); from "+ZeroFromRelease+" onward the allowlist must be empty")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	zeroRequired := false
	if *releaseTag != "" {
		var err error
		zeroRequired, err = versionpolicy.AtLeastTag(*releaseTag, ZeroFromRelease)
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "reasoncodecheck: -release-tag:", err)
			return 2
		}
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
	if zeroRequired {
		if left := totalSites(allow); left > 0 {
			problems = append(problems, fmt.Sprintf(
				"release %s requires every user-facing error to carry a reason code (issue #2093, ADR 0038), but %d unmigrated site(s) in %d file(s) remain in %s; migrate them (issues #2099-#2105) and lower the allowlist to empty",
				*releaseTag, left, len(allow), *allowPath))
		}
	}
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

func totalSites(allow map[string]int) int {
	total := 0
	for _, n := range allow {
		total += n
	}
	return total
}
