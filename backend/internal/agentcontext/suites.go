package agentcontext

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/internal/mdref"
)

var (
	dodHeadingRe = regexp.MustCompile(`(?i)^#{1,6}\s+(dod|do d|definition of done)\s*$`)
	jobIDRe      = regexp.MustCompile(`^  ([A-Za-z0-9_-]+):\s*$`)
	spaceRe      = regexp.MustCompile(`\s+`)
)

// checkSuites validates the Suite registry against the workflows and every
// skill's DoD section (issue #1956): each declared job exists and contains the
// suite's CI command, and every command fence under a skill's DoD heading
// belongs to a registered suite.
func checkSuites(root string, skills []Skill, suites []Suite) []mdref.Finding {
	jobs, err := workflowJobs(root)
	if err != nil {
		return []mdref.Finding{{File: ".github/workflows", Line: 1, Message: err.Error()}}
	}
	var findings []mdref.Finding
	for _, suite := range suites {
		if len(suite.Commands) == 0 {
			findings = append(findings, mdref.Finding{File: ".github/workflows", Line: 1,
				Message: fmt.Sprintf("suite %q has no DoD commands", suite.Name)})
			continue
		}
		if suite.CICommand == "" {
			findings = append(findings, mdref.Finding{File: ".github/workflows", Line: 1,
				Message: fmt.Sprintf("suite %q has no CICommand", suite.Name)})
			continue
		}
		found := false
		for _, job := range suite.CIJobs {
			block, ok := jobs[job]
			if !ok {
				findings = append(findings, mdref.Finding{File: ".github/workflows", Line: 1,
					Message: fmt.Sprintf("suite %q names nonexistent CI job %q", suite.Name, job)})
				continue
			}
			if strings.Contains(block, suite.CICommand) {
				found = true
			}
		}
		if !found {
			findings = append(findings, mdref.Finding{File: ".github/workflows", Line: 1,
				Message: fmt.Sprintf("suite %q: command %q appears in none of its CI jobs %v", suite.Name, suite.CICommand, suite.CIJobs)})
		}
	}

	for _, skill := range skills {
		findings = append(findings, checkDoDCommands(root, skill, suites)...)
	}
	return findings
}

// checkDoDCommands maps each command fence under the SKILL.md DoD heading to a
// registered Suite.
func checkDoDCommands(root string, skill Skill, suites []Suite) []mdref.Finding {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(skill.File))) // #nosec G304 -- skill.File is a discovered repo path
	if err != nil {
		return nil // the skill read error is already reported by validateSkill
	}
	var findings []mdref.Finding
	inDoD := false
	fenceMarker := ""
	for i, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if !inDoD {
			if dodHeadingRe.MatchString(trimmed) {
				inDoD = true
			}
			continue
		}
		if fenceMarker == "" && strings.HasPrefix(trimmed, "#") {
			break
		}
		if fenceMarker != "" {
			if strings.HasPrefix(trimmed, fenceMarker) {
				fenceMarker = ""
				continue
			}
			cmd := strings.TrimSpace(trimmed)
			if cmd == "" || strings.HasPrefix(cmd, "#") {
				continue
			}
			if !commandCovered(cmd, suites) {
				findings = append(findings, mdref.Finding{File: skill.File, Line: i + 1,
					Message: fmt.Sprintf("DoD command is not registered in any Suite: %s", cmd)})
			}
			continue
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenceMarker = trimmed[:3]
		}
	}
	return findings
}

func commandCovered(cmd string, suites []Suite) bool {
	want := commandSegments(cmd)
	if len(want) == 0 {
		return true
	}
	for _, suite := range suites {
		have := map[string]bool{}
		for _, c := range suite.Commands {
			for _, seg := range commandSegments(c) {
				have[seg] = true
			}
		}
		ok := true
		for _, seg := range want {
			if !have[seg] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// commandSegments canonicalizes a shell command for comparison: it splits on
// && and prefixes each segment with the effective working directory so that
// `cd backend && go test` never equals a root-level `go test`.
func commandSegments(cmd string) []string {
	cwd := "."
	var out []string
	for _, raw := range strings.Split(cmd, "&&") {
		seg := strings.TrimSpace(raw)
		if seg == "" {
			continue
		}
		fields := strings.Fields(seg)
		if len(fields) > 0 && fields[0] == "cd" && len(fields) > 1 {
			cwd = filepath.ToSlash(filepath.Clean(fields[1]))
			continue
		}
		out = append(out, cwd+" | "+spaceRe.ReplaceAllString(seg, " "))
	}
	return out
}

// workflowJobs returns each job id's YAML block, keyed by id. A duplicate id
// across workflows keeps the first (ids are file-local in Actions).
func workflowJobs(root string) (map[string]string, error) {
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot read .github/workflows: %v", err)
	}
	jobs := map[string]string{}
	var files []string
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yml") && !strings.HasSuffix(e.Name(), ".yaml")) {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)
	for _, name := range files {
		body, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- fixed workflow directory
		if err != nil {
			return nil, fmt.Errorf("cannot read %s: %v", name, err)
		}
		for id, block := range parseJobs(string(body)) {
			if _, exists := jobs[id]; !exists {
				jobs[id] = block
			}
		}
	}
	return jobs, nil
}

func parseJobs(body string) map[string]string {
	jobs := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	inJobs := false
	current := ""
	var b strings.Builder
	flush := func() {
		if current != "" {
			jobs[current] = b.String()
			b.Reset()
		}
	}
	for scanner.Scan() {
		line := scanner.Text()
		if !inJobs {
			if strings.TrimSpace(line) == "jobs:" && !strings.HasPrefix(line, " ") {
				inJobs = true
			}
			continue
		}
		if m := jobIDRe.FindStringSubmatch(line); m != nil {
			flush()
			current = m[1]
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		if current != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	flush()
	return jobs
}
