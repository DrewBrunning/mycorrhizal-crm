package agenteval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// Profile is one pinned agent configuration. Comparison across runs is only
// meaningful when the agent, model and CLI version stay fixed, so a profile
// with a moving version or a command that silently drops the prompt is
// rejected rather than treated as a dry run.
type Profile struct {
	Command string `yaml:"command" json:"command"`
	Name    string `yaml:"name" json:"name"`
	Model   string `yaml:"model" json:"model"`
	Version string `yaml:"version" json:"version"`
	DryRun  bool   `yaml:"dry_run,omitempty" json:"dry_run,omitempty"`
}

type profileFile struct {
	Version  int                `yaml:"version"`
	Profiles map[string]Profile `yaml:"profiles"`
}

var unpinnedVersions = map[string]bool{
	"":        true,
	"latest":  true,
	"main":    true,
	"master":  true,
	"head":    true,
	"dev":     true,
	"edge":    true,
	"nightly": true,
}

// LoadProfile reads testdata/agent-eval/agents.yaml and returns the named
// profile after validating that it is pinned.
func LoadProfile(root, path, name string) (Profile, error) {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path))) // #nosec G304 -- fixed repository path
	if err != nil {
		return Profile{}, fmt.Errorf("read agent profiles: %w", err)
	}
	var pf profileFile
	if err := yaml.Unmarshal(body, &pf); err != nil {
		return Profile{}, fmt.Errorf("parse agent profiles: %w", err)
	}
	if pf.Version < 1 {
		return Profile{}, fmt.Errorf("agent profile set version = %d, want >= 1", pf.Version)
	}
	profile, ok := pf.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("agent profile %q is not defined", name)
	}
	if err := profile.Validate(name); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// Validate rejects profiles that cannot be compared across runs.
func (p Profile) Validate(name string) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("agent profile %q has no name", name)
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("agent profile %q has no model", name)
	}
	if unpinnedVersions[strings.ToLower(strings.TrimSpace(p.Version))] {
		return fmt.Errorf("agent profile %q version %q is not pinned", name, p.Version)
	}
	if strings.TrimSpace(p.Command) == "" {
		return fmt.Errorf("agent profile %q has no command", name)
	}
	if strings.Contains(p.Command, "@latest") {
		return fmt.Errorf("agent profile %q command uses @latest", name)
	}
	if !p.DryRun && !strings.Contains(p.Command, "{prompt}") {
		return fmt.Errorf("agent profile %q command has no {prompt} placeholder (set dry_run: true for a no-op profile)", name)
	}
	if !p.DryRun && strings.EqualFold(strings.TrimSpace(p.Model), "none") {
		return fmt.Errorf("agent profile %q is not a dry run but has model none", name)
	}
	return nil
}
