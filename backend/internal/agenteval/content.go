package agenteval

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ContentCheck is a task-specific invariant over the files the agent changed.
// Suite commands prove a build/docs gate still passes; a content check proves
// the change actually contains the behavior the prompt asked for. Paths and
// Files are mutually exclusive: Paths checks the concatenation of every changed
// matching file, while Files checks each named file independently.
type ContentCheck struct {
	Name          string   `yaml:"name"`
	Paths         []string `yaml:"paths,omitempty"`
	Files         []string `yaml:"files,omitempty"`
	ContainsAll   []string `yaml:"contains_all,omitempty"`
	ContainsAny   []string `yaml:"contains_any,omitempty"`
	Forbids       []string `yaml:"forbids,omitempty"`
	CaseSensitive bool     `yaml:"case_sensitive,omitempty"`
}

const maxContentCheckFileBytes = 1 << 20

// ValidateContentChecks rejects checks that cannot be scored deterministically.
func ValidateContentChecks(checks []ContentCheck) error {
	seen := map[string]bool{}
	for i, check := range checks {
		if strings.TrimSpace(check.Name) == "" {
			return fmt.Errorf("check %d has no name", i)
		}
		if seen[check.Name] {
			return fmt.Errorf("check %q is declared twice", check.Name)
		}
		seen[check.Name] = true
		if (len(check.Paths) == 0) == (len(check.Files) == 0) {
			return fmt.Errorf("check %q must set exactly one of paths or files", check.Name)
		}
		if len(check.ContainsAll) == 0 && len(check.ContainsAny) == 0 && len(check.Forbids) == 0 {
			return fmt.Errorf("check %q has no contains_all, contains_any or forbids assertion", check.Name)
		}
		for _, pattern := range check.Paths {
			if _, err := globRegexp(pattern); err != nil {
				return fmt.Errorf("check %q path %q: %w", check.Name, pattern, err)
			}
		}
		for _, file := range check.Files {
			clean := filepath.ToSlash(filepath.Clean(file))
			if file == "" || filepath.IsAbs(file) || clean != file || clean == ".." || strings.HasPrefix(clean, "../") {
				return fmt.Errorf("check %q file %q must be a normalized repo-relative path", check.Name, file)
			}
		}
		for _, want := range append(append(append([]string{}, check.ContainsAll...), check.ContainsAny...), check.Forbids...) {
			if strings.TrimSpace(want) == "" {
				return fmt.Errorf("check %q has an empty assertion", check.Name)
			}
		}
	}
	return nil
}

// CheckContent scores every content check against the changed files. Findings
// are human-readable and deterministic.
func CheckContent(root string, changed []string, checks []ContentCheck) []string {
	var findings []string
	for _, check := range checks {
		if len(check.Files) > 0 {
			for _, file := range check.Files {
				body, err := readContentFile(root, file)
				if err != nil {
					findings = append(findings, fmt.Sprintf("content check %q: %s: %v", check.Name, file, err))
					continue
				}
				findings = append(findings, checkContentBody(check, file, body)...)
			}
			continue
		}

		var matched []string
		seen := map[string]bool{}
		for _, path := range changed {
			path = filepath.ToSlash(path)
			if seen[path] {
				continue
			}
			for _, pattern := range check.Paths {
				if MatchPath(pattern, path) {
					matched = append(matched, path)
					seen[path] = true
					break
				}
			}
		}
		if len(matched) == 0 {
			findings = append(findings, fmt.Sprintf("content check %q: no changed file matched %s", check.Name, strings.Join(check.Paths, ", ")))
			continue
		}
		sort.Strings(matched)
		var body strings.Builder
		for _, path := range matched {
			content, err := readContentFile(root, path)
			if err != nil {
				findings = append(findings, fmt.Sprintf("content check %q: %s: %v", check.Name, path, err))
				continue
			}
			body.WriteString(content)
			body.WriteString("\n")
		}
		findings = append(findings, checkContentBody(check, "", body.String())...)
	}
	sort.Strings(findings)
	return findings
}

func readContentFile(root, rel string) (string, error) {
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("unreadable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	if info.Size() > maxContentCheckFileBytes {
		return "", fmt.Errorf("larger than %d bytes", maxContentCheckFileBytes)
	}
	body, err := os.ReadFile(path) // #nosec G304 -- repo-relative task path under the eval worktree
	if err != nil {
		return "", err
	}
	if bytes.IndexByte(body, 0) >= 0 {
		return "", fmt.Errorf("not a text file")
	}
	return string(body), nil
}

func checkContentBody(check ContentCheck, file, body string) []string {
	haystack := body
	if !check.CaseSensitive {
		haystack = strings.ToLower(haystack)
	}
	match := func(want string) bool {
		needle := want
		if !check.CaseSensitive {
			needle = strings.ToLower(needle)
		}
		return strings.Contains(haystack, needle)
	}
	prefix := check.Name
	if file != "" {
		prefix = check.Name + " [" + file + "]"
	}
	var findings []string
	for _, want := range check.ContainsAll {
		if !match(want) {
			findings = append(findings, fmt.Sprintf("content check %q: missing text %q", prefix, want))
		}
	}
	if len(check.ContainsAny) > 0 {
		found := false
		for _, want := range check.ContainsAny {
			if match(want) {
				found = true
				break
			}
		}
		if !found {
			findings = append(findings, fmt.Sprintf("content check %q: none of %s", prefix, strings.Join(check.ContainsAny, ", ")))
		}
	}
	for _, want := range check.Forbids {
		if match(want) {
			findings = append(findings, fmt.Sprintf("content check %q: forbidden text %q", prefix, want))
		}
	}
	return findings
}
