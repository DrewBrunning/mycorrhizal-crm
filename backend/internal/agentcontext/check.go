// Package agentcontext validates the repository's agent-context tree: the
// AGENTS.md file set and aliases (issue #1955), the skill contract and
// reference integrity, the Suites registry, static path/command resolution,
// and fact parity (issue #1956).
//
// It is deliberately read-only and static: it never executes a command parsed
// out of prose and never rewrites a file.
package agentcontext

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mycorrhizal/internal/mdref"
)

// atImport is the one-line fallback an alias may use instead of a symlink.
const atImport = "@AGENTS.md"

// excludedDirs are build/vendor/VCS directories that never carry a tracked
// context file. Skipping them keeps the parity walk cheap and avoids flagging
// a dependency's own AGENTS.md.
var excludedDirs = map[string]bool{
	".git":         true,
	".gradle":      true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"out":          true,
	"target":       true,
	"vendor":       true,
}

// Check validates root's agent-context files and returns every finding. A
// clean tree returns nil.
func Check(root string) []mdref.Finding {
	var findings []mdref.Finding
	registered := map[string]bool{}
	var contextFiles []contextFile
	for _, af := range Registry() {
		if af.AliasOf == "" {
			registered[af.Path] = true
			findings = append(findings, checkAgentFile(root, af)...)
			abs := filepath.Join(root, filepath.FromSlash(af.Path))
			if body, err := os.ReadFile(abs); err == nil { // #nosec G304 -- af.Path comes from the fixed registry
				contextFiles = append(contextFiles, contextFile{Path: af.Path, Text: string(body)})
			}
			continue
		}
		findings = append(findings, checkAlias(root, af)...)
	}
	findings = append(findings, checkParity(root, registered)...)

	skills, skillFindings := checkSkills(root)
	findings = append(findings, skillFindings...)
	findings = append(findings, checkSuites(root, skills, Suites())...)
	findings = append(findings, checkFacts(root, Facts())...)
	for _, skill := range skills {
		abs := filepath.Join(root, filepath.FromSlash(skill.File))
		if body, err := os.ReadFile(abs); err == nil { // #nosec G304 -- skill.File is a discovered repo path
			contextFiles = append(contextFiles, contextFile{Path: skill.File, Text: string(body)})
		}
		for _, ref := range skill.References {
			abs := filepath.Join(root, filepath.FromSlash(ref))
			if body, err := os.ReadFile(abs); err == nil { // #nosec G304 -- ref is a discovered repo path
				contextFiles = append(contextFiles, contextFile{Path: ref, Text: string(body)})
			}
		}
	}
	findings = append(findings, checkReferences(root, contextFiles)...)
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].File != findings[j].File {
			return findings[i].File < findings[j].File
		}
		if findings[i].Line != findings[j].Line {
			return findings[i].Line < findings[j].Line
		}
		return findings[i].Message < findings[j].Message
	})
	return findings
}

func checkAgentFile(root string, af AgentFile) []mdref.Finding {
	abs := filepath.Join(root, filepath.FromSlash(af.Path))
	if !mdref.PathExists(root, abs) {
		return []mdref.Finding{{
			File:    af.Path,
			Line:    1,
			Message: "registered AGENTS.md is missing from disk",
		}}
	}
	body, err := os.ReadFile(abs) // #nosec G304 -- af.Path comes from the fixed registry, never request input
	if err != nil {
		return []mdref.Finding{{
			File:    af.Path,
			Line:    1,
			Message: fmt.Sprintf("unreadable: %v", err),
		}}
	}
	if n := lineCount(body); n > af.MaxLines {
		return []mdref.Finding{{
			File:    af.Path,
			Line:    af.MaxLines + 1,
			Message: fmt.Sprintf("exceeds hard maximum of %d lines (%d)", af.MaxLines, n),
		}}
	}
	return nil
}

func checkAlias(root string, af AgentFile) []mdref.Finding {
	abs := filepath.Join(root, filepath.FromSlash(af.Path))
	target := filepath.Join(root, filepath.FromSlash(af.AliasOf))
	info, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []mdref.Finding{{
				File:    af.Path,
				Line:    1,
				Message: "registered CLAUDE.md alias is missing from disk",
			}}
		}
		return []mdref.Finding{{
			File:    af.Path,
			Line:    1,
			Message: fmt.Sprintf("unreadable: %v", err),
		}}
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(abs)
		if err != nil {
			return []mdref.Finding{{ // # pragma: no cover — Lstat just reported a symlink; Readlink fails only in a filesystem race
				File:    af.Path,
				Line:    1,
				Message: fmt.Sprintf("unreadable symlink: %v", err),
			}}
		}
		resolved := link
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(abs), resolved)
		}
		if _, err := os.Stat(resolved); err != nil {
			return []mdref.Finding{{
				File:    af.Path,
				Line:    1,
				Message: fmt.Sprintf("alias target missing: %s", filepath.ToSlash(link)),
			}}
		}
		if filepath.Clean(resolved) != filepath.Clean(target) {
			return []mdref.Finding{{
				File:    af.Path,
				Line:    1,
				Message: fmt.Sprintf("alias resolves to %s, want %s", filepath.ToSlash(link), af.AliasOf),
			}}
		}
		return nil
	}
	body, err := os.ReadFile(abs) // #nosec G304 -- af.Path comes from the fixed registry, never request input
	if err != nil {
		return []mdref.Finding{{
			File:    af.Path,
			Line:    1,
			Message: fmt.Sprintf("unreadable: %v", err),
		}}
	}
	if !isAtImport(string(body)) {
		return []mdref.Finding{{
			File:    af.Path,
			Line:    1,
			Message: fmt.Sprintf("alias must be a symlink to %s or exactly the one-line @AGENTS.md import", af.AliasOf),
		}}
	}
	return nil
}

// isAtImport reports whether body is exactly the one-line @AGENTS.md import,
// allowing a single trailing newline (or CRLF).
func isAtImport(body string) bool {
	body = strings.TrimSuffix(body, "\n")
	body = strings.TrimSuffix(body, "\r")
	return body == atImport
}

func checkParity(root string, registered map[string]bool) []mdref.Finding {
	onDisk, err := WalkAgentFiles(root)
	if err != nil {
		return []mdref.Finding{{
			File:    "AGENTS.md",
			Line:    1,
			Message: fmt.Sprintf("could not walk the repository: %v", err),
		}}
	}
	var findings []mdref.Finding
	for _, rel := range onDisk {
		if registered[rel] {
			continue
		}
		findings = append(findings, mdref.Finding{
			File:    rel,
			Line:    1,
			Message: "unregistered AGENTS.md — add it to internal/agentcontext.Registry()",
		})
	}
	return findings
}

// WalkAgentFiles returns every AGENTS.md under root, repo-relative and sorted,
// skipping VCS, dependency, and build directories.
func WalkAgentFiles(root string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && excludedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() != "AGENTS.md" {
			return nil
		}
		rel, rerr := filepath.Rel(root, p)
		if rerr != nil {
			return rerr // # pragma: no cover — p comes from WalkDir under root, so Rel cannot fail
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(found)
	return found, nil
}

// Explain returns the AGENTS.md files that would load for target, in load
// order: the repo root first, then each ancestor directory down to the one
// containing target. target may be absolute or repo-relative.
func Explain(root, target string) ([]string, error) {
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, filepath.FromSlash(target))
	}
	rel, err := filepath.Rel(root, filepath.Clean(target))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%s is outside the repository root %s", target, root)
	}
	var chain []string
	if mdref.PathExists(root, "AGENTS.md") {
		chain = append(chain, "AGENTS.md")
	}
	dir := filepath.Dir(rel)
	if dir == "." {
		return chain, nil
	}
	cur := ""
	for _, part := range strings.Split(filepath.ToSlash(dir), "/") {
		cur = filepath.ToSlash(filepath.Join(cur, part))
		candidate := cur + "/AGENTS.md"
		if mdref.PathExists(root, candidate) {
			chain = append(chain, candidate)
		}
	}
	return chain, nil
}

// FindRepoRoot walks up from start until it finds the directory containing
// both .git (a directory or a worktree's .git file) and the root AGENTS.md.
// Deliberately not backend/go.mod: go.mod lives in backend/, which would stop
// the walk early.
func FindRepoRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err // # pragma: no cover — Abs fails only if the process cwd was deleted
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil &&
			mdref.PathExists(dir, "AGENTS.md") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not locate repository root (no .git and AGENTS.md) from %s", start)
		}
		dir = parent
	}
}

func lineCount(body []byte) int {
	if len(body) == 0 {
		return 0
	}
	n := bytes.Count(body, []byte("\n"))
	if body[len(body)-1] != '\n' {
		n++
	}
	return n
}
