// Package patchcov computes changed-line coverage from a Go coverprofile and a
// unified diff (issue #1964, optional half).
//
// Codecov's patch statuses remain the authoritative, server-side gate. This is
// a local warning: it intersects the same cross-package profile the per-file
// ratchet uses with the lines the branch actually changed, so a low
// codecov/patch value can be seen before the PR opens. It deliberately does not
// fail the gate while it is new.
package patchcov

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mycorrhizal/internal/coverageratchet"
)

var hunkHeader = regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

// ParseUnifiedDiff returns the new-side line numbers touched by each file in a
// `git diff -U0` stream. Deleted files (`+++ /dev/null`) are omitted; a pure
// deletion contributes no new-side lines.
func ParseUnifiedDiff(r io.Reader) (map[string][]int, error) {
	out := map[string]map[int]bool{}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	current := ""
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "+++ "):
			name := strings.TrimPrefix(line, "+++ ")
			if name == "/dev/null" {
				current = ""
				continue
			}
			current = strings.TrimPrefix(name, "b/")
			if _, ok := out[current]; !ok {
				out[current] = map[int]bool{}
			}
		case current != "" && strings.HasPrefix(line, "@@"):
			m := hunkHeader.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("malformed hunk header %q", line)
			}
			start, err := strconv.Atoi(m[1])
			if err != nil {
				return nil, fmt.Errorf("malformed hunk start in %q: %w", line, err)
			}
			count := 1
			if m[2] != "" {
				count, err = strconv.Atoi(m[2])
				if err != nil {
					return nil, fmt.Errorf("malformed hunk count in %q: %w", line, err)
				}
			}
			for i := 0; i < count; i++ {
				out[current][start+i] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	result := make(map[string][]int, len(out))
	for file, set := range out {
		if len(set) == 0 {
			continue
		}
		lines := make([]int, 0, len(set))
		for line := range set {
			lines = append(lines, line)
		}
		sort.Ints(lines)
		result[file] = lines
	}
	return result, nil
}

// FileResult is one source file's changed-line coverage.
type FileResult struct {
	File       string
	Executable int
	Covered    int
}

// ChangedLineCoverage attributes each changed executable line to the
// coverprofile block covering it. A line outside every block (a comment, a
// non-executable declaration) is not counted. modulePrefix is the Go module
// prefix the profile uses (`mycorrhizal/`) and repoSubdir is where that module
// lives in the repository (`backend`).
func ChangedLineCoverage(blocks []coverageratchet.Block, changed map[string][]int, modulePrefix, repoSubdir string) []FileResult {
	byFile := map[string][]coverageratchet.Block{}
	for _, block := range blocks {
		rel := strings.TrimPrefix(block.File, modulePrefix)
		repoPath := rel
		if repoSubdir != "" {
			repoPath = strings.TrimSuffix(repoSubdir, "/") + "/" + rel
		}
		byFile[repoPath] = append(byFile[repoPath], block)
	}

	var results []FileResult
	for file, lines := range changed {
		fileBlocks := byFile[file]
		if len(fileBlocks) == 0 {
			continue
		}
		result := FileResult{File: file}
		for _, line := range lines {
			executable := false
			covered := false
			for _, block := range fileBlocks {
				if block.StartLine <= line && line <= block.EndLine {
					executable = true
					if block.Count > 0 {
						covered = true
						break
					}
				}
			}
			if executable {
				result.Executable++
			}
			if covered {
				result.Covered++
			}
		}
		if result.Executable == 0 {
			continue
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].File < results[j].File })
	return results
}

// Percent returns covered/executable as a percentage, or 100 when there is
// nothing executable to measure.
func Percent(results []FileResult) float64 {
	total, covered := Totals(results)
	if total == 0 {
		return 100
	}
	return float64(covered) / float64(total) * 100
}

// Totals sums the executable and covered line counts.
func Totals(results []FileResult) (executable, covered int) {
	for _, r := range results {
		executable += r.Executable
		covered += r.Covered
	}
	return executable, covered
}
