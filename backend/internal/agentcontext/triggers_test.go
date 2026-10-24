package agentcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/internal/mdref"

	yaml "go.yaml.in/yaml/v3"
)

type triggerSet struct {
	Version        int `yaml:"version"`
	AmbiguousPairs []struct {
		Skills      []string `yaml:"skills"`
		Skill       string   `yaml:"skill"`
		ContextFile string   `yaml:"context_file"`
	} `yaml:"ambiguous_pairs"`
	Prompts []struct {
		Prompt string   `yaml:"prompt"`
		Expect string   `yaml:"expect"`
		Not    []string `yaml:"not"`
	} `yaml:"prompts"`
}

func TestTriggerPromptSetIsComplete(t *testing.T) {
	root, err := FindRepoRoot(".")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "testdata", "agent-eval", "triggers.yaml")) // #nosec G304 -- fixed repository path
	if err != nil {
		t.Fatal(err)
	}
	var set triggerSet
	if err := yaml.Unmarshal(body, &set); err != nil {
		t.Fatal(err)
	}
	if set.Version < 1 {
		t.Fatalf("trigger set version = %d", set.Version)
	}

	skills, findings := discoverSkills(root, SkillsRoot)
	if len(findings) > 0 {
		t.Fatalf("skill discovery findings: %v", findings)
	}
	names := map[string]bool{}
	for _, s := range skills {
		names[s.Name] = true
	}
	if len(names) == 0 {
		t.Fatal("no skills discovered")
	}

	perSkill := map[string]int{}
	seen := map[string]bool{}
	for i, p := range set.Prompts {
		if strings.TrimSpace(p.Prompt) == "" {
			t.Errorf("prompt %d is empty", i)
		}
		if seen[p.Prompt] {
			t.Errorf("duplicate prompt: %q", p.Prompt)
		}
		seen[p.Prompt] = true
		if !names[p.Expect] {
			t.Errorf("prompt %d expects unknown skill %q", i, p.Expect)
			continue
		}
		perSkill[p.Expect]++
		for _, other := range p.Not {
			if other == p.Expect {
				t.Errorf("prompt %d lists its expected skill as a non-match", i)
			}
			if !names[other] {
				t.Errorf("prompt %d names unknown non-matching skill %q", i, other)
			}
		}
	}
	for name := range names {
		if perSkill[name] < 2 || perSkill[name] > 3 {
			t.Errorf("skill %q has %d trigger prompts; want 2-3", name, perSkill[name])
		}
	}

	for i, pair := range set.AmbiguousPairs {
		if pair.Skill != "" {
			if !names[pair.Skill] {
				t.Errorf("ambiguous pair %d: unknown skill %q", i, pair.Skill)
			}
			if !mdref.PathExists(root, pair.ContextFile) {
				t.Errorf("ambiguous pair %d: context file missing: %q", i, pair.ContextFile)
			}
			continue
		}
		if len(pair.Skills) != 2 {
			t.Errorf("ambiguous pair %d must name two skills", i)
			continue
		}
		for _, s := range pair.Skills {
			if !names[s] {
				t.Errorf("ambiguous pair %d: unknown skill %q", i, s)
			}
		}
	}
}
