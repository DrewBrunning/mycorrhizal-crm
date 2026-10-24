package agentcontext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/internal/mdref"
)

// codeSpan is one fenced-code line or inline-code span, with the line it
// starts on.
type codeSpan struct {
	Text string
	Line int
}

var (
	pathTokenRe  = regexp.MustCompile(`(?:^|[\s"'` + "`" + `(<=\[])([A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.@{}*?$<>:~#-]+)+)`)
	testRefRe    = regexp.MustCompile(`([A-Za-z0-9_./-]+_test\.go)#((?:Test|Fuzz|Benchmark)[A-Za-z0-9_]+)`)
	makeTargetRe = regexp.MustCompile(`(?m)^([A-Za-z0-9_-]+):`)
)

// checkReferences statically resolves the code references in AGENTS.md,
// SKILL.md, and reference files (issue #1956). It parses fenced code, inline
// code, and explicit markdown links only: free prose is never treated as a
// path.
func checkReferences(root string, files []contextFile) []mdref.Finding {
	var findings []mdref.Finding
	resolver := mdref.Resolver{Root: root}
	for _, f := range files {
		findings = append(findings, resolver.Links(mdref.File{Path: filepath.Join(root, filepath.FromSlash(f.Path)), Display: f.Path, Text: f.Text})...)
		for _, span := range extractCodeSpans(f.Text) {
			findings = append(findings, checkCodeSpan(root, f, span)...)
		}
	}
	return findings
}

// contextFile is one markdown document in the agent-context tree.
type contextFile struct {
	Path string
	Text string
}

func checkCodeSpan(root string, f contextFile, span codeSpan) []mdref.Finding {
	text := strings.TrimSpace(span.Text)
	if text == "" {
		return nil
	}
	if isCommandLine(text) {
		return checkCommandLine(root, f.Path, span.Line, text)
	}
	var findings []mdref.Finding
	for _, m := range testRefRe.FindAllStringSubmatch(text, -1) {
		file := m[1]
		fn := m[2]
		if !resolvePathExists(root, f.Path, file) {
			findings = append(findings, mdref.Finding{File: f.Path, Line: span.Line,
				Message: fmt.Sprintf("test file reference missing: %s", file)})
			continue
		}
		abs := resolvePath(root, f.Path, file)
		body, err := os.ReadFile(abs) // #nosec G304 -- path resolved from a code span under the repo root
		if err == nil && !strings.Contains(string(body), "func "+fn+"(") {
			findings = append(findings, mdref.Finding{File: f.Path, Line: span.Line,
				Message: fmt.Sprintf("test function not found in %s: %s", file, fn)})
		}
	}
	for _, m := range pathTokenRe.FindAllStringSubmatch(text, -1) {
		cand := strings.TrimRight(m[1], ".,;:])")
		if !isResolvablePathToken(cand) {
			continue
		}
		cand = trimLineColSuffix(cand)
		cand = strings.SplitN(cand, "#", 2)[0]
		if resolvePathExists(root, f.Path, cand) {
			continue
		}
		findings = append(findings, mdref.Finding{File: f.Path, Line: span.Line,
			Message: fmt.Sprintf("path reference missing: %s", cand)})
	}
	return findings
}

// isResolvablePathToken filters prose and deliberately-abbreviated tokens out
// of the path check. Only file-like or slash-terminated paths are flagged;
// words, API routes, globs, templates and variables are not paths.
func isResolvablePathToken(tok string) bool {
	if tok == "" || strings.HasPrefix(tok, "/") || strings.HasPrefix(tok, "-") {
		return false
	}
	if strings.ContainsAny(tok, "{}*?$<>|") || strings.Contains(tok, "...") || strings.Contains(tok, "//") {
		return false
	}
	if strings.HasPrefix(tok, "http") || strings.Contains(tok, "://") {
		return false
	}
	if !strings.HasSuffix(tok, "/") && !strings.Contains(filepath.Base(tok), ".") {
		return false
	}
	first := strings.SplitN(strings.TrimPrefix(tok, "./"), "/", 2)[0]
	return repoPathRoots[first]
}

// repoPathRoots are the directory names the checker treats as repository
// paths. A token whose first segment is not here (for example a package-internal
// `lib/typescript.js` citation) is prose, not a path to resolve.
var repoPathRoots = map[string]bool{
	"backend": true, "frontend": true, "android": true, "docs": true,
	".github": true, "cmd": true, "scripts": true, "testdata": true,
	"src": true, "e2e": true, "internal": true, "models": true,
	"services": true, "controllers": true, "database": true, "middleware": true,
	"config": true, "correspondence": true, "vcard3": true, "vcard4": true,
	"jscontact": true, "integrations": true, "openapi": true, "references": true,
}

func isCommandLine(text string) bool {
	t := strings.TrimSpace(strings.TrimPrefix(text, "$ "))
	return strings.HasPrefix(t, "cd ") || strings.HasPrefix(t, "go ") ||
		strings.HasPrefix(t, "make ") || strings.HasPrefix(t, "yarn ") ||
		strings.HasPrefix(t, "npx ")
}

func checkCommandLine(root, fileRel string, line int, text string) []mdref.Finding {
	cwd := root
	var findings []mdref.Finding
	for _, raw := range strings.Split(text, "&&") {
		seg := strings.TrimSpace(raw)
		if seg == "" || strings.HasPrefix(seg, "#") {
			continue
		}
		fields := strings.Fields(seg)
		if len(fields) == 0 {
			continue
		}
		if fields[0] == "cd" {
			if len(fields) < 2 {
				continue
			}
			cwd = resolveCWD(root, cwd, fields[1])
			continue
		}
		var msg string
		switch fields[0] {
		case "go":
			msg = checkGoCommand(root, cwd, fields)
		case "make":
			msg = checkMakeCommand(root, fields)
		case "yarn":
			msg = checkYarnCommand(root, fields)
		case "npx":
			msg = checkNpxCommand(root, fields)
		}
		if msg != "" {
			findings = append(findings, mdref.Finding{File: fileRel, Line: line, Message: msg})
		}
	}
	return findings
}

func resolveCWD(root, cwd, dir string) string {
	if filepath.IsAbs(dir) {
		return filepath.Clean(dir)
	}
	return filepath.Clean(filepath.Join(cwd, filepath.FromSlash(dir)))
}

func checkGoCommand(root, cwd string, fields []string) string {
	if len(fields) < 2 || fields[1] != "run" {
		return ""
	}
	for _, arg := range fields[2:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.ContainsAny(arg, "$*?{}") || strings.Contains(arg, "...") {
			return ""
		}
		probe := strings.TrimPrefix(arg, "./")
		if !strings.HasPrefix(probe, "cmd/") && !strings.Contains(probe, "/cmd/") {
			return ""
		}
		for _, candidate := range []string{
			filepath.Join(cwd, filepath.FromSlash(probe)),
			filepath.Join(root, filepath.FromSlash(probe)),
			filepath.Join(root, "backend", filepath.FromSlash(probe)),
		} {
			if mdref.PathExists(root, candidate) {
				return ""
			}
		}
		rel, _ := filepath.Rel(root, filepath.Join(cwd, filepath.FromSlash(probe)))
		return fmt.Sprintf("go run target missing: %s", filepath.ToSlash(rel))
	}
	return ""
}

func checkMakeCommand(root string, fields []string) string {
	if len(fields) < 2 {
		return ""
	}
	target := fields[1]
	if strings.HasPrefix(target, "-") || strings.ContainsAny(target, "$*?{}") {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(root, "backend", "Makefile")) // #nosec G304 -- fixed repository path
	if err != nil {
		return fmt.Sprintf("cannot read backend/Makefile: %v", err)
	}
	for _, m := range makeTargetRe.FindAllStringSubmatch(string(body), -1) {
		if m[1] == target {
			return ""
		}
	}
	return fmt.Sprintf("make target missing from backend/Makefile: %s", target)
}

func checkYarnCommand(root string, fields []string) string {
	if len(fields) < 2 {
		return ""
	}
	sub := fields[1]
	if strings.HasPrefix(sub, "-") || yarnBuiltin(sub) {
		return ""
	}
	scripts, err := packageScripts(root)
	if err != nil {
		return err.Error()
	}
	if _, ok := scripts[sub]; ok {
		return ""
	}
	return fmt.Sprintf("yarn script missing from frontend/package.json: %s", sub)
}

func yarnBuiltin(sub string) bool {
	switch sub {
	case "install", "add", "remove", "upgrade", "why", "info", "list", "config", "cache", "run":
		return true
	}
	return false
}

var npxAllowlist = map[string]bool{
	"tsc":        true,
	"vitest":     true,
	"playwright": true,
}

func checkNpxCommand(root string, fields []string) string {
	if len(fields) < 2 {
		return ""
	}
	name := fields[1]
	if strings.HasPrefix(name, "-") || npxAllowlist[name] {
		return ""
	}
	pkg, err := packageJSON(root)
	if err != nil {
		return err.Error()
	}
	if _, ok := pkg.DevDependencies[name]; ok {
		return ""
	}
	return fmt.Sprintf("npx binary is neither allowlisted nor a frontend devDependency: %s", name)
}

type packageManifest struct {
	Scripts         map[string]string `json:"scripts"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func packageJSON(root string) (packageManifest, error) {
	var pkg packageManifest
	body, err := os.ReadFile(filepath.Join(root, "frontend", "package.json")) // #nosec G304 -- fixed repository path
	if err != nil {
		return pkg, fmt.Errorf("cannot read frontend/package.json: %v", err)
	}
	if err := json.Unmarshal(body, &pkg); err != nil {
		return pkg, fmt.Errorf("cannot parse frontend/package.json: %v", err)
	}
	return pkg, nil
}

func packageScripts(root string) (map[string]string, error) {
	pkg, err := packageJSON(root)
	return pkg.Scripts, err
}

// resolvePath resolves a repo-relative path, or a path relative to the
// containing markdown file, whichever exists first.
func resolvePath(root, fileRel, p string) string {
	p = strings.TrimPrefix(p, "./")
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	for _, base := range []string{"", filepath.Dir(fileRel), "backend", "frontend"} {
		candidate := filepath.Join(root, filepath.FromSlash(filepath.Join(base, p)))
		if mdref.PathExists(root, candidate) {
			return candidate
		}
	}
	return filepath.Join(root, filepath.FromSlash(p))
}

func resolvePathExists(root, fileRel, p string) bool {
	p = strings.TrimPrefix(p, "./")
	if filepath.IsAbs(p) {
		return mdref.PathExists(root, p)
	}
	for _, base := range []string{
		"",
		filepath.Dir(fileRel),
		"backend",
		"frontend",
	} {
		candidate := filepath.ToSlash(filepath.Join(base, p))
		if mdref.FileReferenceResolves(root, candidate) {
			return true
		}
	}
	return false
}

// trimLineColSuffix removes a trailing :line or :line-col citation.
func trimLineColSuffix(s string) string {
	idx := strings.LastIndex(s, ":")
	if idx <= 0 {
		return s
	}
	suffix := s[idx+1:]
	if suffix == "" {
		return s
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && r != '-' {
			return s
		}
	}
	return s[:idx]
}

// extractCodeSpans returns fenced-code lines and inline-code spans. Markdown
// link targets are handled by mdref.Links, not here.
func extractCodeSpans(text string) []codeSpan {
	lines := strings.Split(text, "\n")
	var spans []codeSpan
	inFence := false
	fenceMarker := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inFence {
			if strings.HasPrefix(trimmed, fenceMarker) {
				inFence = false
				continue
			}
			spans = append(spans, codeSpan{Text: line, Line: i + 1})
			continue
		}
		if strings.HasPrefix(trimmed, "```") {
			inFence = true
			fenceMarker = "```"
			continue
		}
		if strings.HasPrefix(trimmed, "~~~") {
			inFence = true
			fenceMarker = "~~~"
			continue
		}
		for _, span := range inlineCodeSpans(line) {
			span.Line = i + 1
			spans = append(spans, span)
		}
	}
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Line < spans[j].Line })
	return spans
}

// inlineCodeSpans captures the contents of backtick-delimited spans on one
// line. It mirrors mdref's handling of arbitrary backtick-run lengths.
func inlineCodeSpans(line string) []codeSpan {
	runes := []rune(line)
	n := len(runes)
	var out []codeSpan
	i := 0
	for i < n {
		if runes[i] != '`' {
			i++
			continue
		}
		runLen := 0
		for i+runLen < n && runes[i+runLen] == '`' {
			runLen++
		}
		closeAt := -1
		j := i + runLen
		for j < n {
			if runes[j] == '`' {
				k := 0
				for j+k < n && runes[j+k] == '`' {
					k++
				}
				if k == runLen {
					closeAt = j
					break
				}
				j += k
				continue
			}
			j++
		}
		if closeAt < 0 {
			break
		}
		out = append(out, codeSpan{Text: string(runes[i+runLen : closeAt])})
		i = closeAt + runLen
	}
	return out
}
