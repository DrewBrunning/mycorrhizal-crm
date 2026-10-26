// Package agenteval is the deterministic half of the dispatch-only agent
// evaluation harness (issue #1959): task-spec parsing, scope and
// generated-artifact classification, suite-command aggregation, and the
// context-vs-control delta. A task can also declare expect_no_change for a
// refusal case: any changed file then fails the run. The CLI in cmd/agenteval
// owns the disposable worktrees and subprocesses.
package agenteval

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	yaml "go.yaml.in/yaml/v3"
)

// DefaultTimeout bounds one agent invocation or one suite command.
const DefaultTimeout = 20 * time.Minute

// AllowedRequires is the closed set a task may require.
var AllowedRequires = map[string]bool{"go": true, "node": true, "yarn": true, "java": true, "docker": true, "gh": true}

var commitSHARe = regexp.MustCompile(`^[0-9a-f]{40}([0-9a-f]{24})?$`)

// Spec is one task definition from testdata/agent-eval/<task>/spec.yaml.
type Spec struct {
	Name             string   `yaml:"name"`
	Prompt           string   `yaml:"prompt"`
	BaseRef          string   `yaml:"base_ref"`
	Suite            string   `yaml:"suite"`
	ExpectPaths      []string `yaml:"expect_paths"`
	ForbidPaths      []string `yaml:"forbid_paths"`
	Timeout          string   `yaml:"timeout"`
	Requires         []string `yaml:"requires"`
	Control          string   `yaml:"control"`
	AgentCommand     string   `yaml:"agent_command"`
	AgentName        string   `yaml:"agent_name"`
	AgentModel       string   `yaml:"agent_model"`
	AgentVersion     string   `yaml:"agent_version"`
	SeedPatch        string   `yaml:"seed_patch"`
	ExpectedFindings []string `yaml:"expected_findings"`
	ExpectedAny      []string `yaml:"expected_any"`
	ExpectNoChange   bool     `yaml:"expect_no_change"`

	// TaskDir is the absolute task directory containing spec.yaml.
	TaskDir string `yaml:"-"`
	// TimeoutValue is the parsed Timeout, or DefaultTimeout.
	TimeoutValue time.Duration `yaml:"-"`
}

// LoadTask parses a task directory under root/tasksDir/name.
func LoadTask(root, tasksDir, name string, suites []string) (Spec, error) {
	dir := filepath.Join(root, filepath.FromSlash(tasksDir), name)
	body, err := os.ReadFile(filepath.Join(dir, "spec.yaml")) // #nosec G304 -- task dir is chosen by the operator, not request input
	if err != nil {
		return Spec{}, fmt.Errorf("read task spec: %w", err)
	}
	return parseSpec(body, dir, name, suites)
}

func parseSpec(body []byte, dir, name string, suites []string) (Spec, error) {
	var spec Spec
	if err := yaml.Unmarshal(body, &spec); err != nil {
		return Spec{}, fmt.Errorf("parse spec.yaml: %w", err)
	}
	if spec.Name == "" {
		return Spec{}, errors.New("spec.name is required")
	}
	if spec.Name != name {
		return Spec{}, fmt.Errorf("spec.name %q must equal the task directory %q", spec.Name, name)
	}
	if spec.Prompt == "" {
		prompt, err := os.ReadFile(filepath.Join(dir, "task.md")) // #nosec G304 -- task dir from the operator
		if err != nil {
			return Spec{}, fmt.Errorf("spec.prompt is empty and task.md is unreadable: %w", err)
		}
		spec.Prompt = strings.TrimSpace(string(prompt))
	}
	if strings.TrimSpace(spec.Prompt) == "" {
		return Spec{}, errors.New("spec.prompt or task.md is required")
	}
	if err := ValidateBaseRef(spec.BaseRef); err != nil {
		return Spec{}, fmt.Errorf("spec.base_ref: %w", err)
	}
	if spec.Suite == "" {
		return Spec{}, errors.New("spec.suite is required")
	}
	if !contains(suites, spec.Suite) {
		return Spec{}, fmt.Errorf("spec.suite %q is not in internal/agentcontext.Suites()", spec.Suite)
	}
	if len(spec.ExpectPaths) == 0 && !spec.ExpectNoChange {
		return Spec{}, errors.New("spec.expect_paths must name at least one allowed path unless expect_no_change is true")
	}
	for _, pattern := range append(append([]string{}, spec.ExpectPaths...), spec.ForbidPaths...) {
		if _, err := globRegexp(pattern); err != nil {
			return Spec{}, fmt.Errorf("invalid path glob %q: %w", pattern, err)
		}
	}
	spec.TimeoutValue = DefaultTimeout
	if spec.Timeout != "" {
		d, err := time.ParseDuration(spec.Timeout)
		if err != nil || d <= 0 {
			return Spec{}, fmt.Errorf("spec.timeout %q is not a positive Go duration", spec.Timeout)
		}
		spec.TimeoutValue = d
	}
	for _, req := range spec.Requires {
		if !AllowedRequires[req] {
			return Spec{}, fmt.Errorf("spec.requires %q is not one of go/node/yarn/java/docker/gh", req)
		}
	}
	if strings.TrimSpace(spec.AgentCommand) == "" {
		return Spec{}, errors.New("spec.agent_command is required")
	}
	if spec.SeedPatch != "" {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(spec.SeedPatch))); err != nil {
			return Spec{}, fmt.Errorf("spec.seed_patch %q is unreadable: %w", spec.SeedPatch, err)
		}
	}
	if spec.Control != "" {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(spec.Control))); err != nil {
			return Spec{}, fmt.Errorf("spec.control %q is unreadable: %w", spec.Control, err)
		}
	}
	spec.TaskDir = dir
	return spec, nil
}

// ValidateBaseRef enforces the pinned-commit rule.
func ValidateBaseRef(ref string) error {
	if !commitSHARe.MatchString(ref) {
		return fmt.Errorf("base ref %q must be a full commit SHA, never a moving branch or tag", ref)
	}
	return nil
}

func contains(haystack []string, needle string) bool {
	for _, v := range haystack {
		if v == needle {
			return true
		}
	}
	return false
}

// MissingRequires returns the required tools not present on PATH, in spec order.
func MissingRequires(spec Spec, lookPath func(string) (string, error)) []string {
	var missing []string
	for _, req := range spec.Requires {
		if _, err := lookPath(req); err != nil {
			missing = append(missing, req)
		}
	}
	return missing
}

// ExpandPrompt substitutes the task prompt into the agent command template.
// `{prompt}` is replaced with a shell-single-quoted prompt; a command without
// the placeholder receives no prompt (a deliberate dry-run/no-op command).
func ExpandPrompt(command, prompt string) string {
	return strings.ReplaceAll(command, "{prompt}", shellQuote(prompt))
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

var credentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(--?(?:api[-_]?key|token|secret|password)[= ]+)[^\s]+`),
	regexp.MustCompile(`(?i)((?:api[-_]?key|token|secret|password)=)[^\s]+`),
	regexp.MustCompile(`(?i)(\b[A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD)=)[^\s]+`),
	regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`),
}

// RedactCommand removes credential-shaped values before a command is written
// to the artifact. It is not a substitute for not passing secrets on argv.
func RedactCommand(command string) string {
	for _, re := range credentialPatterns {
		command = re.ReplaceAllString(command, "${1}[redacted]")
	}
	return command
}

// globRegexp compiles a repo-relative glob. `**` spans directories; `*` and
// `?` do not cross `/`.
func globRegexp(pattern string) (*regexp.Regexp, error) {
	if strings.ContainsAny(pattern, "[]") {
		return nil, errors.New("character classes are not supported")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(pattern[i])))
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}

// MatchPath reports whether a repo-relative path matches a glob.
func MatchPath(pattern, path string) bool {
	re, err := globRegexp(pattern)
	return err == nil && re.MatchString(path)
}

// SortStrings returns a sorted copy.
func SortStrings(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}
