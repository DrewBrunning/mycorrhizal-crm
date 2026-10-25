package agenteval

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// selectionReportFile is where the agent is asked to record what it opened.
	// It is excluded from the repo-change check because it is the measurement.
	selectionReportFile = ".agent-eval-selection.json"
	// selectionInstruction is appended to every prompt. It is a fixed,
	// model-independent footer so the prompts themselves stay unchanged.
	selectionInstruction = "\n\nWhen you have finished, write " + selectionReportFile +
		" containing exactly one JSON object: " +
		`{"skills":["<repo-relative SKILL.md paths you opened>"],"context_files":["<repo-relative AGENTS.md or CLAUDE.md paths you relied on>"]}. ` +
		"Do not modify any other file."
)

// SelectionOptions configures one selection pass over a trigger set.
type SelectionOptions struct {
	Root          string
	BaseRef       string
	TriggersPath  string
	Set           TriggerSet
	AgentCommand  string
	AgentName     string
	AgentModel    string
	AgentVersion  string
	Timeout       time.Duration
	CommandRunner func(ctx context.Context, dir, command string, timeout time.Duration) CommandResult
}

// ResolveCommit resolves a ref to a full commit SHA and rejects option-shaped
// refs before they reach git.
func ResolveCommit(root, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		ref = "HEAD"
	}
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\n") {
		return "", fmt.Errorf("invalid base ref %q", ref)
	}
	out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", ref+"^{commit}").Output() // #nosec G204 -- ref is validated above
	if err != nil {
		return "", fmt.Errorf("resolve base ref %q: %w", ref, err)
	}
	sha := strings.TrimSpace(string(out))
	if err := ValidateBaseRef(sha); err != nil {
		return "", err
	}
	return sha, nil
}

// RunSelection runs every trigger prompt in a disposable worktree and scores
// which skill the agent selected. It is selection-only: it does not run the
// task's suite and it does not use a stripped-context control.
func RunSelection(ctx context.Context, opts SelectionOptions) (SelectionArtifact, error) {
	start := time.Now()
	artifact := SelectionArtifact{
		Version:      opts.Set.Version,
		Triggers:     opts.TriggersPath,
		BaseRef:      opts.BaseRef,
		AgentName:    opts.AgentName,
		AgentModel:   opts.AgentModel,
		AgentVersion: opts.AgentVersion,
		AgentCommand: RedactCommand(opts.AgentCommand),
		StartedAt:    start.UTC(),
	}
	finish := func() {
		artifact.FinishedAt = time.Now().UTC()
		artifact.DurationMS = time.Since(start).Milliseconds()
		artifact.Total = len(artifact.Outcomes)
		artifact.Passed = 0
		for _, o := range artifact.Outcomes {
			if o.Status == "passed" {
				artifact.Passed++
			}
		}
		if artifact.Status == "error" {
			return
		}
		switch {
		case artifact.Total == 0:
			artifact.Status = "error"
			artifact.Reason = "trigger set has no prompts"
		case artifact.Passed == artifact.Total:
			artifact.Status = "passed"
		default:
			artifact.Status = "failed"
		}
	}
	if opts.CommandRunner == nil {
		opts.CommandRunner = runShellCommand
	}
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}
	if strings.TrimSpace(opts.AgentCommand) == "" {
		artifact.Status = "error"
		artifact.Reason = "agent command is required"
		finish()
		return artifact, fmt.Errorf("selection: -agent-command is required")
	}
	if err := ValidateBaseRef(opts.BaseRef); err != nil {
		artifact.Status = "error"
		artifact.Reason = err.Error()
		finish()
		return artifact, err
	}

	for _, prompt := range opts.Set.Prompts {
		outcome, err := runSelectionPrompt(ctx, opts, prompt)
		if err != nil {
			artifact.Outcomes = append(artifact.Outcomes, outcome)
			artifact.Status = "error"
			artifact.Reason = err.Error()
			finish()
			return artifact, err
		}
		artifact.Outcomes = append(artifact.Outcomes, outcome)
	}
	finish()
	return artifact, nil
}

func runSelectionPrompt(ctx context.Context, opts SelectionOptions, prompt TriggerPrompt) (SelectionOutcome, error) {
	out := SelectionOutcome{Prompt: prompt.Prompt, Expect: prompt.Expect, Not: prompt.Not, Status: "failed"}
	tmp, err := os.MkdirTemp("", "agent-select-*")
	if err != nil {
		return out, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	wt := filepath.Join(tmp, "worktree")
	if err := addWorktree(ctx, opts.Root, wt, opts.BaseRef); err != nil {
		return out, err
	}
	defer removeWorktree(opts.Root, wt)

	baseline, err := changedFiles(ctx, wt)
	if err != nil {
		return out, err
	}
	command := ExpandPrompt(opts.AgentCommand, prompt.Prompt+selectionInstruction)
	out.Agent = opts.CommandRunner(ctx, wt, command, opts.Timeout)

	changed, err := changedFiles(ctx, wt)
	if err != nil {
		return out, err
	}
	for _, path := range subtract(changed, baseline) {
		if path != selectionReportFile {
			out.RepoChanges = append(out.RepoChanges, path)
		}
	}
	sort.Strings(out.RepoChanges)

	if out.Agent.ExitCode != 0 || out.Agent.TimedOut {
		out.Reason = fmt.Sprintf("agent exited %d", out.Agent.ExitCode)
		if out.Agent.TimedOut {
			out.Reason = "agent timed out"
		}
		return out, nil
	}

	reportPath := filepath.Join(wt, selectionReportFile)
	info, err := os.Stat(reportPath)
	if err != nil {
		out.Reason = "agent did not write " + selectionReportFile
		return out, nil
	}
	if info.Size() > 1<<20 {
		out.Reason = selectionReportFile + " is larger than 1 MiB"
		return out, nil
	}
	body, err := os.ReadFile(reportPath) // #nosec G304 -- fixed filename in our worktree
	if err != nil {
		out.Reason = "agent did not write " + selectionReportFile
		return out, nil
	}
	var raw SelectionOutput
	if err := json.Unmarshal(body, &raw); err != nil {
		out.Reason = "parse " + selectionReportFile + ": " + err.Error()
		return out, nil
	}
	out.Skills, out.ContextFiles, out.Unknown = NormalizeSelection(raw)
	ScoreSelection(&out, opts.Set.AmbiguousPairs)
	if out.Reason == "" && len(out.RepoChanges) > 0 {
		out.Reason = "selection run changed repository files"
	}
	if out.Reason == "" {
		out.Status = "passed"
	}
	return out, nil
}
