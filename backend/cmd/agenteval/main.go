// Command agenteval runs one representative task per suite in a disposable
// worktree and scores it against the same Suites commands the skills and CI
// declare (issue #1959). It is never a merge or release gate: a dispatch run is
// an experiment, and the weekly collector compares a pinned profile against the
// committed testdata/agent-eval/baseline.json (issue #2030).
//
// Usage:
//
//	cd backend && go run ./cmd/agenteval -task database-migration
//	cd backend && go run ./cmd/agenteval -task api-contract-change \
//	    -agent-command 'opencode run {prompt}'
//
// The task spec (testdata/agent-eval/<task>/spec.yaml) pins a commit SHA, the
// suite, the expected/forbidden paths, and the agent command template. Scoring
// also records gate-integrity findings: added test skips, lint/coverage
// suppressions, removed test declarations, and deleted test files. Output is a
// JSON artifact plus a Markdown summary under agent-eval-results/.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"mycorrhizal/internal/agentcontext"
	"mycorrhizal/internal/agenteval"
)

func main() { // # pragma: no cover — thin os.Exit wrapper over run
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agenteval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var task, tasksDir, rootFlag, outDir, agentCommand, agentName, agentModel, agentVersion, baseRef, mode, triggersPath, timeoutFlag, skill string
	var profileName, profilesPath, baselinePath, baselineOut string
	var noControl, requireBaseline, forceBaseline bool
	fs.StringVar(&mode, "mode", "task", "task or select (trigger-selection pass)")
	fs.StringVar(&task, "task", "", "task name under testdata/agent-eval (required in task mode)")
	fs.StringVar(&profileName, "profile", "", "pinned agent profile from testdata/agent-eval/agents.yaml")
	fs.StringVar(&profilesPath, "profiles", "testdata/agent-eval/agents.yaml", "agent profile file, repo-relative")
	fs.StringVar(&baselinePath, "baseline", "", "committed baseline to compare against (requires -profile)")
	fs.StringVar(&baselineOut, "write-baseline", "", "write this run into the named baseline file (requires -profile)")
	fs.BoolVar(&requireBaseline, "require-baseline", false, "fail if the profile/task has no committed baseline entry")
	fs.BoolVar(&forceBaseline, "force-baseline", false, "write a baseline even when the run did not pass")
	fs.StringVar(&tasksDir, "tasks-dir", "testdata/agent-eval", "task directory, repo-relative")
	fs.StringVar(&triggersPath, "triggers", "testdata/agent-eval/triggers.yaml", "trigger prompt set, repo-relative (select mode)")
	fs.StringVar(&timeoutFlag, "timeout", "5m", "per-prompt timeout in select mode")
	fs.StringVar(&skill, "skill", "", "only run prompts expecting this skill in select mode")
	fs.StringVar(&rootFlag, "root", "", "repository root (default: walk up to .git and AGENTS.md)")
	fs.StringVar(&outDir, "out", "agent-eval-results", "artifact directory, repo-relative or absolute")
	fs.StringVar(&baseRef, "base-ref", "", "override the spec's pinned commit SHA (workflow passes the dispatch commit)")
	fs.StringVar(&agentCommand, "agent-command", "", "override the spec's agent command template ({prompt} is substituted)")
	fs.StringVar(&agentName, "agent-name", "", "agent name recorded in the artifact")
	fs.StringVar(&agentModel, "agent-model", "", "model id recorded in the artifact")
	fs.StringVar(&agentVersion, "agent-version", "", "agent CLI version recorded in the artifact")
	fs.BoolVar(&noControl, "no-control", false, "skip the stripped-context control run (not recommended)")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: agenteval -task NAME [-agent-command CMD] [-out DIR] [-no-control]")
		fmt.Fprintln(stderr, "       agenteval -mode select -agent-command CMD [-triggers PATH] [-base-ref REF]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if mode != "task" && mode != "select" {
		fmt.Fprintln(stderr, "agenteval: -mode must be task or select")
		return 2
	}
	if mode == "task" && task == "" {
		fmt.Fprintln(stderr, "agenteval: -task is required")
		return 2
	}
	if mode == "select" && (profileName != "" || baselinePath != "" || baselineOut != "" || requireBaseline) {
		fmt.Fprintln(stderr, "agenteval: -profile and baseline flags are only supported in task mode")
		return 2
	}
	if (baselinePath != "" || baselineOut != "") && profileName == "" {
		fmt.Fprintln(stderr, "agenteval: -baseline and -write-baseline require -profile (results are only comparable per pinned profile)")
		return 2
	}
	if requireBaseline && baselinePath == "" {
		fmt.Fprintln(stderr, "agenteval: -require-baseline requires -baseline")
		return 2
	}
	if profileName != "" && (agentCommand != "" || agentName != "" || agentModel != "" || agentVersion != "") {
		fmt.Fprintln(stderr, "agenteval: explicit agent flags cannot be combined with -profile; edit the profile instead")
		return 2
	}
	root := rootFlag
	if root == "" {
		found, err := agentcontext.FindRepoRoot(".")
		if err != nil {
			fmt.Fprintln(stderr, "agenteval:", err)
			return 1
		}
		root = found
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		fmt.Fprintln(stderr, "agenteval:", err) // # pragma: no cover — Abs fails only if cwd was deleted
		return 1                                // # pragma: no cover
	}
	if mode == "select" {
		return runSelect(abs, triggersPath, baseRef, outDir, agentCommand, agentName, agentModel, agentVersion, timeoutFlag, skill, stdout, stderr)
	}

	var loadedProfile agenteval.Profile
	if profileName != "" {
		profile, err := agenteval.LoadProfile(abs, profilesPath, profileName)
		if err != nil {
			fmt.Fprintln(stderr, "agenteval:", err)
			return 1
		}
		loadedProfile = profile
		if agentCommand == "" {
			agentCommand = profile.Command
		}
		if agentName == "" {
			agentName = profile.Name
		}
		if agentModel == "" {
			agentModel = profile.Model
		}
		if agentVersion == "" {
			agentVersion = profile.Version
		}
	}
	if loadedProfile.DryRun && (baselinePath != "" || baselineOut != "" || requireBaseline) {
		fmt.Fprintln(stderr, "agenteval: dry-run profiles cannot be compared with a baseline")
		return 2
	}
	if missing, err := agenteval.VerifyGeneratorTargets(abs, agenteval.GeneratedArtifacts); err != nil {
		fmt.Fprintln(stderr, "agenteval:", err)
		return 1
	} else if len(missing) > 0 {
		fmt.Fprintf(stderr, "agenteval: generated-artifact map names missing Makefile target(s): %s\n", strings.Join(missing, ", "))
		return 1
	}

	suites := agentcontext.Suites()
	names := make([]string, 0, len(suites))
	local := make([]agenteval.Suite, 0, len(suites))
	for _, s := range suites {
		names = append(names, s.Name)
		local = append(local, agenteval.Suite{Name: s.Name, Commands: s.Commands})
	}
	spec, err := agenteval.LoadTask(abs, tasksDir, task, names)
	if err != nil {
		fmt.Fprintln(stderr, "agenteval:", err)
		return 1
	}
	if baseRef != "" {
		if err := agenteval.ValidateBaseRef(baseRef); err != nil {
			fmt.Fprintln(stderr, "agenteval: -base-ref:", err)
			return 2
		}
		spec.BaseRef = baseRef
	}

	artifact, runErr := agenteval.Run(context.Background(), agenteval.Options{
		Root:         abs,
		Spec:         spec,
		Suites:       local,
		AgentCommand: agentCommand,
		AgentProfile: profileName,
		AgentName:    agentName,
		AgentModel:   agentModel,
		AgentVersion: agentVersion,
		RunControl:   !noControl,
	})
	effectiveProfile := agenteval.Profile{Command: agentCommand, Name: agentName, Model: agentModel, Version: agentVersion}
	if effectiveProfile.Command == "" {
		effectiveProfile.Command = spec.AgentCommand
	}
	if effectiveProfile.Name == "" {
		effectiveProfile.Name = spec.AgentName
	}
	if effectiveProfile.Model == "" {
		effectiveProfile.Model = spec.AgentModel
	}
	if effectiveProfile.Version == "" {
		effectiveProfile.Version = spec.AgentVersion
	}
	configHash := agenteval.ConfigFingerprint(effectiveProfile, spec, local)
	current := agenteval.SummarizeRun(artifact.Status, artifact.Context, configHash)
	if baselinePath != "" {
		base, err := agenteval.LoadBaseline(resolvePath(abs, baselinePath))
		if err != nil {
			fmt.Fprintln(stderr, "agenteval: baseline:", err)
			return 1
		}
		comparison := agenteval.CompareBaseline(base, profileName, task, current)
		artifact.Baseline = &comparison
	}
	jsonPath, mdPath, writeErr := writeArtifact(abs, outDir, artifact)
	if writeErr != nil {
		fmt.Fprintln(stderr, "agenteval: write artifact:", writeErr)
	}
	if artifact.Status != "skipped" {
		fmt.Fprint(stdout, agenteval.Markdown(artifact))
	}
	if jsonPath != "" {
		fmt.Fprintf(stdout, "\nArtifacts: %s, %s\n", jsonPath, mdPath)
	}
	if runErr != nil {
		fmt.Fprintln(stderr, "agenteval:", runErr)
		return 1
	}
	if requireBaseline && artifact.Baseline != nil && !artifact.Baseline.Present {
		fmt.Fprintln(stderr, "agenteval: no committed baseline entry for", profileName, "/", task)
		return 1
	}
	if artifact.Baseline != nil && artifact.Baseline.Regressed() {
		return 1
	}
	if baselineOut != "" {
		if (artifact.Status != "passed" || !current.Success) && !forceBaseline {
			fmt.Fprintln(stderr, "agenteval: refusing to write a baseline from a non-passing run (use -force-baseline to override)")
			return 1
		}
		out := resolvePath(abs, baselineOut)
		base, err := agenteval.LoadBaseline(out)
		if err != nil {
			fmt.Fprintln(stderr, "agenteval: baseline:", err)
			return 1
		}
		agenteval.RecordBaseline(&base, profileName, task, current)
		if err := agenteval.WriteBaseline(out, base); err != nil {
			fmt.Fprintln(stderr, "agenteval: write baseline:", err)
			return 1
		}
	}
	switch artifact.Status {
	case "passed", "skipped":
		return 0
	default:
		return 1
	}
}

func resolvePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, filepath.FromSlash(path))
}

func runSelect(abs, triggersPath, baseRef, outDir, agentCommand, agentName, agentModel, agentVersion, timeoutFlag, skill string, stdout, stderr io.Writer) int {
	if strings.TrimSpace(agentCommand) == "" {
		fmt.Fprintln(stderr, "agenteval: -agent-command is required in select mode")
		return 2
	}
	if !strings.Contains(agentCommand, "{prompt}") {
		fmt.Fprintln(stderr, "agenteval: -agent-command in select mode must contain {prompt}")
		return 2
	}
	timeout, err := time.ParseDuration(timeoutFlag)
	if err != nil || timeout <= 0 {
		fmt.Fprintln(stderr, "agenteval: -timeout must be a positive Go duration")
		return 2
	}
	if baseRef != "" {
		if err := agenteval.ValidateBaseRef(baseRef); err != nil {
			fmt.Fprintln(stderr, "agenteval: -base-ref:", err)
			return 2
		}
	}
	set, err := agenteval.LoadTriggerSet(abs, triggersPath)
	if err != nil {
		fmt.Fprintln(stderr, "agenteval:", err)
		return 1
	}
	if skill != "" {
		var filtered []agenteval.TriggerPrompt
		for _, prompt := range set.Prompts {
			if prompt.Expect == skill {
				filtered = append(filtered, prompt)
			}
		}
		if len(filtered) == 0 {
			fmt.Fprintln(stderr, "agenteval: -skill", skill, "matches no trigger prompt")
			return 1
		}
		set.Prompts = filtered
	}
	sha, err := agenteval.ResolveCommit(abs, baseRef)
	if err != nil {
		fmt.Fprintln(stderr, "agenteval: -base-ref:", err)
		return 2
	}
	artifact, runErr := agenteval.RunSelection(context.Background(), agenteval.SelectionOptions{
		Root:         abs,
		BaseRef:      sha,
		TriggersPath: triggersPath,
		Set:          set,
		AgentCommand: agentCommand,
		AgentName:    agentName,
		AgentModel:   agentModel,
		AgentVersion: agentVersion,
		Timeout:      timeout,
	})
	jsonPath, mdPath, writeErr := writeSelectionArtifact(abs, outDir, artifact)
	if writeErr != nil {
		fmt.Fprintln(stderr, "agenteval: write artifact:", writeErr)
	}
	fmt.Fprint(stdout, agenteval.SelectionMarkdown(artifact))
	if jsonPath != "" {
		fmt.Fprintf(stdout, "\nArtifacts: %s, %s\n", jsonPath, mdPath)
	}
	if runErr != nil {
		fmt.Fprintln(stderr, "agenteval:", runErr)
		return 1
	}
	switch artifact.Status {
	case "passed":
		return 0
	default:
		return 1
	}
}

func writeSelectionArtifact(root, outDir string, artifact agenteval.SelectionArtifact) (string, string, error) {
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(root, filepath.FromSlash(outDir))
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return "", "", err
	}
	stamp := artifact.StartedAt.Format("20060102T150405Z")
	base := "selection-" + stamp
	jsonPath := filepath.Join(outDir, base+".json")
	mdPath := filepath.Join(outDir, base+".md")
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil { // #nosec G306 -- uploaded eval evidence, not secret
		return "", "", err
	}
	if err := os.WriteFile(mdPath, []byte(agenteval.SelectionMarkdown(artifact)), 0o644); err != nil { // #nosec G306 -- uploaded eval evidence, not secret
		return "", "", err
	}
	return jsonPath, mdPath, nil
}

func writeArtifact(root, outDir string, artifact agenteval.Artifact) (string, string, error) {
	if !filepath.IsAbs(outDir) {
		outDir = filepath.Join(root, filepath.FromSlash(outDir))
	}
	if err := os.MkdirAll(outDir, 0o750); err != nil {
		return "", "", err
	}
	stamp := artifact.StartedAt.Format("20060102T150405Z")
	base := fmt.Sprintf("%s-%s", artifact.Task, stamp)
	jsonPath := filepath.Join(outDir, base+".json")
	mdPath := filepath.Join(outDir, base+".md")
	body, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil { // #nosec G306 -- uploaded eval evidence, not secret
		return "", "", err
	}
	if err := os.WriteFile(mdPath, []byte(agenteval.Markdown(artifact)), 0o644); err != nil { // #nosec G306 -- uploaded eval evidence, not secret
		return "", "", err
	}
	return jsonPath, mdPath, nil
}
