// Command agenteval runs one representative task per Tier 1 suite in a
// disposable worktree and scores it against the same Suites commands the
// skills and CI declare (issue #1959). It is dispatch-only, never a merge or
// release gate.
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

	"mycorrhizal/internal/agentcontext"
	"mycorrhizal/internal/agenteval"
)

func main() { // # pragma: no cover — thin os.Exit wrapper over run
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agenteval", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var task, tasksDir, rootFlag, outDir, agentCommand, agentName, agentModel, agentVersion, baseRef string
	var noControl bool
	fs.StringVar(&task, "task", "", "task name under testdata/agent-eval (required)")
	fs.StringVar(&tasksDir, "tasks-dir", "testdata/agent-eval", "task directory, repo-relative")
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
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if task == "" {
		fmt.Fprintln(stderr, "agenteval: -task is required")
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
		AgentName:    agentName,
		AgentModel:   agentModel,
		AgentVersion: agentVersion,
		RunControl:   !noControl,
	})
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
	switch artifact.Status {
	case "passed", "skipped":
		return 0
	default:
		return 1
	}
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
