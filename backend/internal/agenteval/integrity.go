package agenteval

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Integrity violations are hard gate-weakening signals: added test skips,
// removed or deleted tests, and test files renamed away from a test name.
// Integrity warnings are weaker signals that need a human read: lint/coverage
// suppressions and build constraints added to a test file. Keeping them apart
// avoids failing a run for a reasoned, narrowly scoped suppression.
var (
	addedSkipPatterns = []struct {
		label string
		re    *regexp.Regexp
	}{
		{"test skip", regexp.MustCompile(`\b(?:t|b)\.Skip(?:f|Now)?\(|\b(?:describe|it|test)\.skip\s*\(|\bx(?:it|describe|test)\s*\(|@pytest\.mark\.skip|pytest\.skip\(|@unittest\.skip|@Ignore\b|@Disabled\b`)},
	}

	addedWarningPatterns = []struct {
		label string
		re    *regexp.Regexp
	}{
		{"lint suppression", regexp.MustCompile(`//\s*nolint\b|#nosec\b|//lint:ignore|@ts-ignore|@ts-expect-error|@ts-nocheck|eslint-disable|biome-ignore|prettier-ignore`)},
		{"coverage suppression", regexp.MustCompile(`#\s*pragma:\s*no\s*cover|istanbul ignore|stryker disable|coverage:\s*ignore`)},
	}

	removedTestPatterns = []struct {
		label string
		re    *regexp.Regexp
	}{
		{"removed test declaration", regexp.MustCompile(`^\s*(?:func\s+(?:Test|Benchmark|Fuzz)\w+\s*\(|@Test\b|def\s+test_\w+\s*\(|(?:describe|it|test)\s*\()`)},
	}

	buildConstraintRe = regexp.MustCompile(`^\s*//go:build\b|^\s*// \+build\b`)
	testFileRe        = regexp.MustCompile(`(?i)(?:_test\.go|_test\.py|(?:^|/)test_[^/]+\.py|Test\.kt|(?:^|/)[^/]+\.(?:test|spec)\.[cm]?[jt]sx?)$`)
)

// DetectIntegrityViolations reports hard violations and weaker warnings in a
// unified diff. deleted and renamed are the corresponding git status lists.
func DetectIntegrityViolations(patch string, deleted []string, renamed []RenamedFile) (violations, warnings []string) {
	seenViolation := map[string]bool{}
	seenWarning := map[string]bool{}
	addViolation := func(message string) {
		if !seenViolation[message] {
			seenViolation[message] = true
			violations = append(violations, message)
		}
	}
	addWarning := func(message string) {
		if !seenWarning[message] {
			seenWarning[message] = true
			warnings = append(warnings, message)
		}
	}

	file := ""
	minusFile := ""
	for _, line := range strings.Split(patch, "\n") {
		switch {
		case strings.HasPrefix(line, "--- a/"):
			minusFile = strings.TrimPrefix(line, "--- a/")
			continue
		case strings.HasPrefix(line, "+++ b/"):
			file = strings.TrimPrefix(line, "+++ b/")
			continue
		case strings.HasPrefix(line, "+++ /dev/null"):
			file = minusFile
			continue
		}
		if file == "" || len(line) < 2 {
			continue
		}
		switch line[0] {
		case '+':
			added := line[1:]
			testFile := isTestFile(file)
			if testFile {
				if buildConstraintRe.MatchString(added) {
					addWarning("added build constraint to test file: " + file)
					continue
				}
				if !isCommentLine(added) {
					for _, p := range addedSkipPatterns {
						if p.re.MatchString(added) {
							addViolation("added " + p.label + ": " + file)
							break
						}
					}
				}
			}
			for _, p := range addedWarningPatterns {
				if p.re.MatchString(added) {
					addWarning("added " + p.label + ": " + file)
					break
				}
			}
		case '-':
			removed := line[1:]
			if !isTestFile(file) || isCommentLine(removed) {
				continue
			}
			for _, p := range removedTestPatterns {
				if p.re.MatchString(removed) {
					addViolation(p.label + ": " + file)
					break
				}
			}
		}
	}

	for _, path := range deleted {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if isTestFile(path) {
			addViolation("deleted test file: " + path)
		}
	}
	for _, rename := range renamed {
		if isTestFile(rename.Old) && !isTestFile(rename.New) {
			addViolation("test file renamed away: " + rename.Old + " -> " + rename.New)
		}
	}

	sort.Strings(violations)
	sort.Strings(warnings)
	return violations, warnings
}

func isTestFile(path string) bool {
	return testFileRe.MatchString(filepath.ToSlash(path))
}

func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "//") ||
		strings.HasPrefix(trimmed, "/*") ||
		strings.HasPrefix(trimmed, "*") ||
		strings.HasPrefix(trimmed, "#")
}
