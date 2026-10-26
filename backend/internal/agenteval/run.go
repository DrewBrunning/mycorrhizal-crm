package agenteval

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options configures one task run.
type Options struct {
	Root          string
	Spec          Spec
	Suites        []Suite
	AgentCommand  string
	AgentName     string
	AgentModel    string
	AgentVersion  string
	RunControl    bool
	LookPath      func(string) (string, error)
	CommandRunner func(ctx context.Context, dir, command string, timeout time.Duration) CommandResult
}

// Suite is the subset of agentcontext.Suite the harness needs, kept local so
// the deterministic package does not depend on the validator package.
type Suite struct {
	Name     string
	Commands []string
}

// Run executes one task in disposable worktrees. It never writes to the
// caller's working tree; only git's worktree metadata is touched.
func Run(ctx context.Context, opts Options) (Artifact, error) {
	if opts.LookPath == nil {
		opts.LookPath = exec.LookPath
	}
	if opts.CommandRunner == nil {
		opts.CommandRunner = runShellCommand
	}
	command := opts.Spec.AgentCommand
	if opts.AgentCommand != "" {
		command = opts.AgentCommand
	}
	command = ExpandPrompt(command, opts.Spec.Prompt)
	artifact := Artifact{
		Task:         opts.Spec.Name,
		BaseRef:      opts.Spec.BaseRef,
		AgentName:    firstNonEmpty(opts.AgentName, opts.Spec.AgentName),
		AgentModel:   firstNonEmpty(opts.AgentModel, opts.Spec.AgentModel),
		AgentVersion: firstNonEmpty(opts.AgentVersion, opts.Spec.AgentVersion),
		AgentCommand: RedactCommand(command),
		StartedAt:    time.Now().UTC(),
	}
	start := time.Now()
	finish := func() {
		artifact.FinishedAt = time.Now().UTC()
		artifact.DurationMS = time.Since(start).Milliseconds()
	}

	if missing := MissingRequires(opts.Spec, opts.LookPath); len(missing) > 0 {
		artifact.Status = "skipped"
		artifact.Reason = "missing required tool(s): " + strings.Join(missing, ", ")
		finish()
		return artifact, nil
	}

	tmp, err := os.MkdirTemp("", "agent-eval-*")
	if err != nil {
		artifact.Status = "error"
		artifact.Reason = err.Error()
		finish()
		return artifact, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	contextWt := filepath.Join(tmp, "context")
	contextRun, _, err := runWorktree(ctx, opts, contextWt, false, command)
	if err != nil {
		artifact.Status = "error"
		artifact.Reason = err.Error()
		finish()
		return artifact, err
	}
	artifact.Context = &contextRun
	finalizeRun(&contextRun, opts.Spec)

	if opts.RunControl {
		controlWt := filepath.Join(tmp, "control")
		controlRun, stripped, err := runWorktree(ctx, opts, controlWt, true, command)
		if err != nil {
			artifact.Status = "error"
			artifact.Reason = err.Error()
			finish()
			return artifact, err
		}
		artifact.Stripped = stripped
		artifact.Control = &controlRun
		finalizeRun(&controlRun, opts.Spec)
		delta := Compare(&contextRun, &controlRun)
		artifact.Delta = &delta
	}

	if contextRun.Success {
		artifact.Status = "passed"
	} else {
		artifact.Status = "failed"
	}
	finish()
	return artifact, nil
}

func runWorktree(ctx context.Context, opts Options, wt string, control bool, command string) (RunResult, []string, error) {
	var result RunResult
	if err := addWorktree(ctx, opts.Root, wt, opts.Spec.BaseRef); err != nil {
		return result, nil, err
	}
	defer removeWorktree(opts.Root, wt)

	if opts.Spec.SeedPatch != "" {
		patch := filepath.Join(opts.Spec.TaskDir, filepath.FromSlash(opts.Spec.SeedPatch))
		if err := applySeedPatch(ctx, wt, patch); err != nil {
			return result, nil, err
		}
	}
	var stripped []string
	var err error
	if control {
		stripped, err = stripContext(wt)
		if err != nil {
			return result, stripped, fmt.Errorf("strip control context: %w", err)
		}
	}
	evalBase, err := snapshotBase(ctx, wt)
	if err != nil {
		return result, stripped, err
	}

	agent := opts.CommandRunner(ctx, wt, command, opts.Spec.TimeoutValue)
	result.Agent = agent
	changed, err := changedFiles(ctx, wt, evalBase)
	if err != nil {
		return result, stripped, err
	}
	result.ChangedFiles = changed
	result.ScopeViolations = ClassifyPaths(result.ChangedFiles, opts.Spec.ExpectPaths, opts.Spec.ForbidPaths)
	result.HandEditedGenerated = DetectHandEdits(result.ChangedFiles, GeneratedArtifacts)
	result.HeadMoved, err = headMoved(ctx, wt, evalBase)
	if err != nil {
		return result, stripped, err
	}
	patch, scanWarnings, truncated, err := worktreePatch(ctx, wt, evalBase)
	if err != nil {
		return result, stripped, err
	}
	deleted, err := deletedFiles(ctx, wt, evalBase)
	if err != nil {
		return result, stripped, err
	}
	renamed, err := renamedFiles(ctx, wt, evalBase)
	if err != nil {
		return result, stripped, err
	}
	violations, detectorWarnings := DetectIntegrityViolations(patch, deleted, renamed)
	if truncated {
		violations = append(violations, "integrity scan truncated: diff is too large to verify")
	}
	result.IntegrityViolations = append(result.IntegrityViolations, violations...)
	result.IntegrityWarnings = append(result.IntegrityWarnings, scanWarnings...)
	result.IntegrityWarnings = append(result.IntegrityWarnings, detectorWarnings...)
	if len(opts.Spec.ExpectedFindings) > 0 {
		result.ExpectedFindingsOK, result.MissingFindings = FindingsSatisfied(opts.Spec.ExpectedFindings, agent.Output)
	}
	if results, ok := runSuite(ctx, opts, wt); ok {
		result.SuiteCommands = results
	}
	return result, stripped, nil
}

func runSuite(ctx context.Context, opts Options, wt string) ([]CommandResult, bool) {
	var suite *Suite
	for i := range opts.Suites {
		if opts.Suites[i].Name == opts.Spec.Suite {
			suite = &opts.Suites[i]
			break
		}
	}
	if suite == nil {
		return nil, false
	}
	var out []CommandResult
	for _, command := range suite.Commands {
		out = append(out, opts.CommandRunner(ctx, wt, command, opts.Spec.TimeoutValue))
	}
	return out, true
}

func finalizeRun(r *RunResult, spec Spec) {
	if r.Agent.ExitCode != 0 || r.Agent.TimedOut {
		r.Status = "failed"
	} else {
		r.Status = "passed"
	}
	Score(r)
	if !r.Success {
		r.Status = "failed"
	}
}

func addWorktree(ctx context.Context, root, wt, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "add", "--detach", wt, ref) // #nosec G204 -- ref is a validated commit SHA
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git worktree add %s: %v: %s", ref, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func removeWorktree(root, wt string) {
	_ = exec.Command("git", "-C", root, "worktree", "remove", "--force", wt).Run() // #nosec G204 -- wt is a temp dir this process created
	_ = exec.Command("git", "-C", root, "worktree", "prune").Run()                 // #nosec G204 -- fixed git command, root is the caller repo
}

// applySeedPatch applies a patch with --3way. A patch that does not apply fails
// loudly before the agent runs.
func applySeedPatch(ctx context.Context, wt, patch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", wt, "apply", "--3way", patch) // #nosec G204 -- patch is a repo task file
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("seed patch does not apply (%s): %v: %s", filepath.Base(patch), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stripContext removes the skills tree, nested AGENTS.md/CLAUDE.md files, and
// the root Project map routing section from a control worktree. It returns the
// paths it removed or changed.
func stripContext(wt string) ([]string, error) {
	var stripped []string
	for _, p := range []string{".agents/skills", ".claude/skills"} {
		abs := filepath.Join(wt, filepath.FromSlash(p))
		if _, err := os.Lstat(abs); err == nil {
			if err := os.RemoveAll(abs); err != nil {
				return stripped, err
			}
			stripped = append(stripped, p)
		}
	}
	err := filepath.WalkDir(wt, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "build", "dist", "out", "target", ".gradle":
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name != "AGENTS.md" && name != "CLAUDE.md" {
			return nil
		}
		rel, err := filepath.Rel(wt, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "AGENTS.md" || rel == "CLAUDE.md" {
			return nil
		}
		if err := os.Remove(path); err != nil { // #nosec G122 -- disposable worktree created by this process; no symlink TOCTOU boundary
			return err
		}
		stripped = append(stripped, rel)
		return nil
	})
	if err != nil {
		return stripped, err
	}
	rootAgents := filepath.Join(wt, "AGENTS.md")
	body, err := os.ReadFile(rootAgents) // #nosec G304 -- our worktree
	if err == nil {
		trimmed := stripRoutingTable(string(body))
		if trimmed != string(body) {
			if err := os.WriteFile(rootAgents, []byte(trimmed), 0o644); err != nil { // #nosec G306,G703 -- rootAgents is inside our disposable worktree
				return stripped, err
			}
			stripped = append(stripped, "AGENTS.md#project-map")
		}
	}
	sort.Strings(stripped)
	return stripped, nil
}

func stripRoutingTable(root string) string {
	start := strings.Index(root, "\n## Project map")
	if start < 0 {
		if strings.HasPrefix(root, "## Project map") {
			start = 0
		} else {
			return root
		}
	}
	rest := root[start+1:]
	next := strings.Index(rest, "\n## ")
	if next < 0 {
		return root[:start] + "\n"
	}
	return root[:start] + rest[next:]
}

func subtract(current, baseline []string) []string {
	seen := map[string]bool{}
	for _, b := range baseline {
		seen[b] = true
	}
	var out []string
	for _, c := range current {
		if !seen[c] {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

const outputLimit = 1 << 20

// runShellCommand executes a command in dir and returns a bounded result.
func runShellCommand(ctx context.Context, dir, command string, timeout time.Duration) CommandResult {
	res := CommandResult{Command: RedactCommand(command)}
	start := time.Now()
	runCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, "bash", "-lc", command) // #nosec G204 -- commands come from the trusted task spec and Suite registry
	cmd.Dir = dir
	cmd.Env = os.Environ()
	var buf limitedBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()
	res.Output = buf.String()
	res.OutputTail = buf.Tail()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			res.ExitCode = exitErr.ExitCode()
		} else {
			res.ExitCode = -1
		}
		if runCtx.Err() == context.DeadlineExceeded {
			res.TimedOut = true
		}
		return res
	}
	res.Passed = true
	return res
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if n >= outputLimit {
		b.buf.Reset()
		_, _ = b.buf.Write(p[n-outputLimit:])
		return n, nil
	}
	if b.buf.Len()+n > outputLimit {
		drop := b.buf.Len() + n - outputLimit
		b.buf.Next(drop)
	}
	_, _ = b.buf.Write(p)
	return n, nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }

func (b *limitedBuffer) Tail() string {
	s := b.buf.String()
	if len(s) > 8192 {
		s = "...(truncated)...\n" + s[len(s)-8192:]
	}
	return s
}
