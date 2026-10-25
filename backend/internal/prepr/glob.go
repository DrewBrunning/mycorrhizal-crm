// Package prepr implements the path->area selection and step planning for the
// local pre-PR gate (issue #1964).
//
// The pre-commit hook is a fast subset of CI; it deliberately excludes the full
// area suites, the coverage ratchets, and the flake-stress pass. Those are the
// failure classes that used to surface only after a PR opened. cmd/prepr reads
// .github/filters.yaml (the single source of path -> area) and plans the same
// per-area checks CI runs, so the same diff can be exercised locally first.
//
// The package is deliberately side-effect free: it parses filters, maps
// changed paths to areas, and builds an executable plan. cmd/prepr owns process
// execution so planning stays unit-testable without running a suite.
package prepr

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

var globCache sync.Map // pattern -> *regexp.Regexp

// Match reports whether path matches pattern using the same `*`, `?`, and `**`
// semantics .github/filters.yaml relies on for dorny/paths-filter. Patterns are
// repo-relative POSIX paths (git's --name-only output). A `**` matches across
// directory separators; `**/` may match no directory at all, so
// `android/**/build.gradle.kts` also matches `android/build.gradle.kts`.
func Match(pattern, path string) bool {
	re, err := compileGlob(pattern)
	if err != nil {
		return false
	}
	return re.MatchString(path)
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	if cached, ok := globCache.Load(pattern); ok {
		return cached.(*regexp.Regexp), nil
	}

	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
				continue
			}
			b.WriteString("[^/]*")
		case '?':
			b.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("$")

	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("invalid filter glob %q: %w", pattern, err)
	}
	globCache.Store(pattern, re)
	return re, nil
}
