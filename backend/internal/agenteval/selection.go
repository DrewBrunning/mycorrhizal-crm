package agenteval

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"
)

// TriggerPair is one declared ambiguous pair from triggers.yaml. A pair is
// either two skills, or a skill and a platform context file.
type TriggerPair struct {
	Skills      []string `yaml:"skills,omitempty" json:"skills,omitempty"`
	Skill       string   `yaml:"skill,omitempty" json:"skill,omitempty"`
	ContextFile string   `yaml:"context_file,omitempty" json:"context_file,omitempty"`
}

// TriggerPrompt is one versioned selection prompt.
type TriggerPrompt struct {
	Prompt string   `yaml:"prompt" json:"prompt"`
	Expect string   `yaml:"expect" json:"expect"`
	Not    []string `yaml:"not,omitempty" json:"not,omitempty"`
}

// TriggerSet is the parsed testdata/agent-eval/triggers.yaml.
type TriggerSet struct {
	Version        int             `yaml:"version" json:"version"`
	AmbiguousPairs []TriggerPair   `yaml:"ambiguous_pairs,omitempty" json:"ambiguous_pairs,omitempty"`
	Prompts        []TriggerPrompt `yaml:"prompts" json:"prompts"`
}

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// LoadTriggerSet reads and validates the selection prompt set. Every named
// skill must resolve under .agents/skills, and every declared context file must
// exist in the repository.
func LoadTriggerSet(root, path string) (TriggerSet, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) // #nosec G304 -- fixed repository path
	if err != nil {
		return TriggerSet{}, fmt.Errorf("read trigger set: %w", err)
	}
	var set TriggerSet
	if err := yaml.Unmarshal(body, &set); err != nil {
		return TriggerSet{}, fmt.Errorf("parse trigger set: %w", err)
	}
	if set.Version < 1 {
		return TriggerSet{}, fmt.Errorf("trigger set version = %d, want >= 1", set.Version)
	}
	if len(set.Prompts) == 0 {
		return TriggerSet{}, fmt.Errorf("trigger set has no prompts")
	}
	seen := map[string]bool{}
	for i, p := range set.Prompts {
		prompt := strings.TrimSpace(p.Prompt)
		if prompt == "" {
			return TriggerSet{}, fmt.Errorf("prompt %d is empty", i)
		}
		if seen[prompt] {
			return TriggerSet{}, fmt.Errorf("duplicate prompt %q", prompt)
		}
		seen[prompt] = true
		if err := requireSkill(root, p.Expect); err != nil {
			return TriggerSet{}, fmt.Errorf("prompt %d: %w", i, err)
		}
		for _, other := range p.Not {
			if other == p.Expect {
				return TriggerSet{}, fmt.Errorf("prompt %d lists its expected skill %q as a non-match", i, other)
			}
			if err := requireSkill(root, other); err != nil {
				return TriggerSet{}, fmt.Errorf("prompt %d: %w", i, err)
			}
		}
	}
	for i, pair := range set.AmbiguousPairs {
		if len(pair.Skills) == 0 && pair.Skill == "" {
			return TriggerSet{}, fmt.Errorf("ambiguous pair %d names neither skills nor a skill", i)
		}
		for _, name := range pair.Skills {
			if err := requireSkill(root, name); err != nil {
				return TriggerSet{}, fmt.Errorf("ambiguous pair %d: %w", i, err)
			}
		}
		if pair.Skill != "" {
			if err := requireSkill(root, pair.Skill); err != nil {
				return TriggerSet{}, fmt.Errorf("ambiguous pair %d: %w", i, err)
			}
		}
		if pair.ContextFile != "" {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(pair.ContextFile))); err != nil { // #nosec G304 -- declared context path
				return TriggerSet{}, fmt.Errorf("ambiguous pair %d context_file %q: %w", i, pair.ContextFile, err)
			}
		}
	}
	return set, nil
}

func requireSkill(root, name string) error {
	if !skillNameRe.MatchString(name) {
		return fmt.Errorf("skill name %q is not a lowercase kebab-case name", name)
	}
	p := filepath.Join(root, ".agents", "skills", name, "SKILL.md")
	if _, err := os.Stat(p); err != nil { // #nosec G304 -- validated skill name
		return fmt.Errorf("skill %q: %w", name, err)
	}
	return nil
}

// SelectionOutput is the JSON the agent is asked to write. Both fields are
// repo-relative paths; bare skill names are also accepted for skills.
type SelectionOutput struct {
	Skills       []string `json:"skills"`
	ContextFiles []string `json:"context_files,omitempty"`
}

// SelectionOutcome is one prompt's result.
type SelectionOutcome struct {
	Prompt       string        `json:"prompt"`
	Expect       string        `json:"expect"`
	Not          []string      `json:"not,omitempty"`
	Skills       []string      `json:"skills,omitempty"`
	ContextFiles []string      `json:"context_files,omitempty"`
	Unknown      []string      `json:"unknown,omitempty"`
	Unexpected   []string      `json:"unexpected,omitempty"`
	Ambiguous    []string      `json:"ambiguous,omitempty"`
	RepoChanges  []string      `json:"repo_changes,omitempty"`
	Matched      bool          `json:"matched"`
	Agent        CommandResult `json:"agent"`
	Status       string        `json:"status"` // passed | failed | error
	Reason       string        `json:"reason,omitempty"`
}

// SelectionArtifact is the JSON contract the workflow uploads.
type SelectionArtifact struct {
	Version      int                `json:"version"`
	Triggers     string             `json:"triggers"`
	BaseRef      string             `json:"base_ref"`
	AgentName    string             `json:"agent_name,omitempty"`
	AgentModel   string             `json:"agent_model,omitempty"`
	AgentVersion string             `json:"agent_version,omitempty"`
	AgentCommand string             `json:"agent_command"`
	Status       string             `json:"status"` // passed | failed | error
	Reason       string             `json:"reason,omitempty"`
	StartedAt    time.Time          `json:"started_at"`
	FinishedAt   time.Time          `json:"finished_at"`
	DurationMS   int64              `json:"duration_ms"`
	Outcomes     []SelectionOutcome `json:"outcomes"`
	Passed       int                `json:"passed"`
	Total        int                `json:"total"`
}

// NormalizeSelection turns the agent's reported paths into skill names and
// context-file paths, recording anything that matches neither.
func NormalizeSelection(raw SelectionOutput) (skills, contextFiles, unknown []string) {
	add := func(dst *[]string, v string) {
		if v != "" && !contains(*dst, v) {
			*dst = append(*dst, v)
		}
	}
	for _, v := range append(append([]string{}, raw.Skills...), raw.ContextFiles...) {
		v = strings.TrimSpace(filepath.ToSlash(v))
		v = strings.TrimPrefix(v, "./")
		if v == "" || filepath.IsAbs(v) || strings.HasPrefix(v, "../") {
			add(&unknown, v)
			continue
		}
		if isContextFile(v) {
			add(&contextFiles, v)
			continue
		}
		if name, ok := skillFromPath(v); ok {
			add(&skills, name)
			continue
		}
		add(&unknown, v)
	}
	sort.Strings(skills)
	sort.Strings(contextFiles)
	sort.Strings(unknown)
	return skills, contextFiles, unknown
}

func isContextFile(path string) bool {
	base := filepath.Base(path)
	return base == "AGENTS.md" || base == "CLAUDE.md"
}

func skillFromPath(path string) (string, bool) {
	const prefix = ".agents/skills/"
	if strings.HasPrefix(path, prefix) && strings.HasSuffix(path, "/SKILL.md") {
		name := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/SKILL.md")
		if skillNameRe.MatchString(name) && !strings.Contains(name, "/") {
			return name, true
		}
	}
	if skillNameRe.MatchString(path) {
		return path, true
	}
	return "", false
}

// ScoreSelection applies the expect/not rules to a normalized selection.
func ScoreSelection(out *SelectionOutcome, pairs []TriggerPair) {
	out.Matched = contains(out.Skills, out.Expect)
	for _, other := range out.Not {
		if contains(out.Skills, other) {
			out.Unexpected = append(out.Unexpected, other)
		}
	}
	for _, pair := range pairs {
		if len(pair.Skills) == 2 && contains(out.Skills, pair.Skills[0]) && contains(out.Skills, pair.Skills[1]) {
			out.Ambiguous = append(out.Ambiguous, pair.Skills[0]+" + "+pair.Skills[1])
		}
		if pair.Skill != "" && pair.ContextFile != "" && contains(out.Skills, pair.Skill) && contains(out.ContextFiles, pair.ContextFile) {
			out.Ambiguous = append(out.Ambiguous, pair.Skill+" + "+pair.ContextFile)
		}
	}
	switch {
	case !out.Matched:
		out.Reason = "expected skill " + out.Expect + " was not selected"
	case len(out.Unexpected) > 0:
		out.Reason = "non-matching skill(s) selected: " + strings.Join(out.Unexpected, ", ")
	case len(out.RepoChanges) > 0:
		out.Reason = "selection run changed repository files"
	}
}

// SelectionMarkdown renders the human summary.
func SelectionMarkdown(a SelectionArtifact) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent trigger selection\n\n")
	fmt.Fprintf(&b, "- Status: **%s**\n", a.Status)
	if a.Reason != "" {
		fmt.Fprintf(&b, "- Reason: %s\n", a.Reason)
	}
	fmt.Fprintf(&b, "- Base ref: `%s`\n", a.BaseRef)
	fmt.Fprintf(&b, "- Agent: %s / %s / %s\n", a.AgentName, a.AgentModel, a.AgentVersion)
	fmt.Fprintf(&b, "- Command: `%s`\n", a.AgentCommand)
	fmt.Fprintf(&b, "- Selected correctly: %d/%d\n", a.Passed, a.Total)
	for _, o := range a.Outcomes {
		fmt.Fprintf(&b, "\n## %s (%s)\n\n", o.Expect, o.Status)
		fmt.Fprintf(&b, "- Prompt: %s\n", o.Prompt)
		fmt.Fprintf(&b, "- Selected: %s\n", emptyDash(o.Skills))
		if len(o.ContextFiles) > 0 {
			fmt.Fprintf(&b, "- Context files: %s\n", strings.Join(o.ContextFiles, ", "))
		}
		if len(o.Unexpected) > 0 {
			fmt.Fprintf(&b, "- Unexpected: %s\n", strings.Join(o.Unexpected, ", "))
		}
		if len(o.Ambiguous) > 0 {
			fmt.Fprintf(&b, "- Ambiguous pair also selected: %s\n", strings.Join(o.Ambiguous, ", "))
		}
		if len(o.RepoChanges) > 0 {
			fmt.Fprintf(&b, "- Repo changes: %s\n", strings.Join(o.RepoChanges, ", "))
		}
		if o.Reason != "" {
			fmt.Fprintf(&b, "- Reason: %s\n", o.Reason)
		}
	}
	return b.String()
}

func emptyDash(v []string) string {
	if len(v) == 0 {
		return "-"
	}
	return strings.Join(v, ", ")
}
