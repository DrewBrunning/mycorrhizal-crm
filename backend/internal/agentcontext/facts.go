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
	"mycorrhizal/internal/schemafixture"
)

// checkFacts compares stable, load-bearing context prose with its canonical
// source (issue #1956). Absent context files (for example a skill that a later
// stage has not created yet) are skipped; a present file that disagrees is a
// finding.
func checkFacts(root string, facts []Fact) []mdref.Finding {
	var findings []mdref.Finding
	for _, fact := range facts {
		values, err := factValues(root, fact.Source)
		if err != nil {
			findings = append(findings, mdref.Finding{File: "AGENTS.md", Line: 1,
				Message: fmt.Sprintf("fact %q: cannot read canonical source %q: %v", fact.Name, fact.Source, err)})
			continue
		}
		re, err := regexp.Compile(fact.Pattern)
		if err != nil {
			findings = append(findings, mdref.Finding{File: "AGENTS.md", Line: 1,
				Message: fmt.Sprintf("fact %q: invalid pattern: %v", fact.Name, err)})
			continue
		}
		want := valueSet(values)
		for _, rel := range fact.Files {
			body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))) // #nosec G304 -- fixed registry paths, never request input
			if err != nil {
				continue
			}
			stated := map[string]bool{}
			for _, m := range re.FindAllStringSubmatch(string(body), -1) {
				for _, capture := range m[1:] {
					for _, value := range factCaptures(fact.Source, capture) {
						stated[value] = true
					}
				}
			}
			if len(stated) == 0 {
				findings = append(findings, mdref.Finding{File: rel, Line: 1,
					Message: fmt.Sprintf("fact %q: prose does not state %s from %s", fact.Name, strings.Join(values, ", "), fact.Source)})
				continue
			}
			if diff := setDifference(want, stated); len(diff) > 0 {
				findings = append(findings, mdref.Finding{File: rel, Line: 1,
					Message: fmt.Sprintf("fact %q: canonical %s says [%s]; prose states [%s] (missing %s)",
						fact.Name, fact.Source, strings.Join(values, ", "), strings.Join(keys(stated), ", "), strings.Join(diff, ", "))})
			}
			if diff := setDifference(stated, want); len(diff) > 0 {
				findings = append(findings, mdref.Finding{File: rel, Line: 1,
					Message: fmt.Sprintf("fact %q: canonical %s says [%s]; prose states extra [%s]",
						fact.Name, fact.Source, strings.Join(values, ", "), strings.Join(diff, ", "))})
			}
		}
	}
	return findings
}

func valueSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func setDifference(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// factValues is the named-reader switch. Each case reads the canonical source;
// it never trusts a copied literal.
func factValues(root, source string) ([]string, error) {
	switch source {
	case "upgrade-floor":
		return []string{schemafixture.FloorTag, fmt.Sprintf("%06d", schemafixture.FloorVersion)}, nil
	case "locale-set":
		return localeSet(root)
	case "e2e-port":
		return e2ePort(root)
	case "required-gate-jobs":
		return requiredGateJobs(root)
	}
	return nil, fmt.Errorf("unknown fact source %q", source)
}

func localeSet(root string) ([]string, error) {
	dir := filepath.Join(root, "frontend", "src", "i18n", "locales")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var codes []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".d.ts") {
			continue
		}
		codes = append(codes, strings.TrimSuffix(name, ".json"))
	}
	sort.Strings(codes)
	if len(codes) == 0 {
		return nil, fmt.Errorf("no locale json files in %s", filepath.ToSlash(dir))
	}
	return codes, nil
}

var portRe = regexp.MustCompile(`localhost:([0-9]+)`)

func e2ePort(root string) ([]string, error) {
	for _, rel := range []string{
		filepath.Join("frontend", "e2e", "global-setup.ts"),
		filepath.Join("frontend", "playwright.config.ts"),
	} {
		body, err := os.ReadFile(filepath.Join(root, rel)) // #nosec G304 -- fixed repository paths
		if err != nil {
			continue
		}
		if m := portRe.FindStringSubmatch(string(body)); m != nil {
			return []string{m[1]}, nil
		}
	}
	return nil, fmt.Errorf("no localhost port found in frontend/playwright.config.ts or frontend/e2e/global-setup.ts")
}

type ruleset struct {
	Rules []struct {
		Type       string `json:"type"`
		Parameters struct {
			RequiredStatusChecks []struct {
				Context string `json:"context"`
			} `json:"required_status_checks"`
		} `json:"parameters"`
	} `json:"rules"`
}

type gateRegistry struct {
	Gates []struct {
		Tier         string  `json:"tier"`
		Mandatory    bool    `json:"mandatory"`
		CheckContext *string `json:"check_context"`
	} `json:"gates"`
}

func requiredGateJobs(root string) ([]string, error) {
	set := map[string]bool{}
	var rs ruleset
	body, err := os.ReadFile(filepath.Join(root, ".github", "rulesets", "main-protection.json")) // #nosec G304 -- fixed repository path
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &rs); err != nil {
		return nil, fmt.Errorf("parse main-protection.json: %v", err)
	}
	for _, rule := range rs.Rules {
		if rule.Type != "required_status_checks" {
			continue
		}
		for _, check := range rule.Parameters.RequiredStatusChecks {
			set[check.Context] = true
		}
	}
	var gates gateRegistry
	body, err = os.ReadFile(filepath.Join(root, ".github", "release-gates.json")) // #nosec G304 -- fixed repository path
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &gates); err != nil {
		return nil, fmt.Errorf("parse release-gates.json: %v", err)
	}
	for _, g := range gates.Gates {
		if g.Tier == "per-pr" && g.Mandatory && g.CheckContext != nil && *g.CheckContext != "" {
			set[*g.CheckContext] = true
		}
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("no required gate contexts found")
	}
	return keys(set), nil
}

// factCaptures normalizes one regex capture into canonical values. The
// locale-set anchor naturally captures a whole list ("en, de, es"); the other
// anchors capture one value per group.
func factCaptures(source, capture string) []string {
	capture = strings.Trim(strings.TrimSpace(capture), "`")
	if capture == "" {
		return nil
	}
	if source != "locale-set" {
		return []string{capture}
	}
	var out []string
	for _, m := range localeCodeRe.FindAllStringSubmatch(capture, -1) {
		out = append(out, m[1])
	}
	return out
}

var localeCodeRe = regexp.MustCompile(`\b([a-z]{2})\b`)
