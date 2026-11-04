package agenteval

import (
	"strings"
	"testing"
	"time"
)

func promotionFixture(t *testing.T) (Artifact, string, Profile, Spec, []Suite) {
	t.Helper()
	profile := Profile{Command: "agent run {prompt}", Name: "agent", Model: "model", Version: "1.2.3"}
	spec := Spec{Name: "demo", Prompt: "do it", BaseRef: sha, Suite: "demo"}
	suites := []Suite{{Name: "demo", Commands: []string{"true"}}}
	artifact := Artifact{
		Task:         spec.Name,
		BaseRef:      spec.BaseRef,
		Profile:      "pinned",
		AgentName:    profile.Name,
		AgentModel:   profile.Model,
		AgentVersion: profile.Version,
		AgentCommand: RedactCommand(ExpandPrompt(profile.Command, spec.Prompt)),
		Status:       "passed",
		Context:      &RunResult{Success: true, SuiteCommands: []CommandResult{{Passed: true}}},
	}
	return artifact, "pinned", profile, spec, suites
}

func TestSummarizeArtifactPromotesVerifiedSummary(t *testing.T) {
	artifact, profileName, profile, spec, suites := promotionFixture(t)
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	summary, err := SummarizeArtifact(artifact, profileName, profile, spec, suites, "https://example.invalid/run", now)
	if err != nil {
		t.Fatal(err)
	}
	if !summary.Verified || summary.PromotedAt != "2026-11-03T12:00:00Z" || summary.PromotedFrom != "https://example.invalid/run" {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.ConfigHash != ConfigFingerprint(profile, spec, suites) || !summary.Success {
		t.Fatalf("summary = %+v", summary)
	}
}

func TestSummarizeArtifactRejectsUnmeasuredRuns(t *testing.T) {
	artifact, profileName, profile, spec, suites := promotionFixture(t)
	now := time.Now()

	cases := []struct {
		name   string
		mutate func(*Artifact)
		want   string
	}{
		{"failed", func(a *Artifact) { a.Status = "failed" }, "not a passing run"},
		{"no context", func(a *Artifact) { a.Context = nil }, "not a passing run"},
		{"wrong profile", func(a *Artifact) { a.Profile = "other" }, "profile"},
		{"wrong task", func(a *Artifact) { a.Task = "other" }, "task"},
		{"wrong base", func(a *Artifact) { a.BaseRef = strings.Repeat("b", 40) }, "base ref"},
		{"wrong command", func(a *Artifact) { a.AgentCommand = "other" }, "command"},
		{"wrong version", func(a *Artifact) { a.AgentVersion = "9.9.9" }, "agent identity"},
		{"content check not run", func(a *Artifact) {
			a.Context.ContentChecksRun = 0
			spec.ContentChecks = []ContentCheck{{Name: "x", Paths: []string{"docs/**"}, ContainsAll: []string{"x"}}}
		}, "did not run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copyArtifact := artifact
			copyContext := *artifact.Context
			copyArtifact.Context = &copyContext
			tc.mutate(&copyArtifact)
			_, err := SummarizeArtifact(copyArtifact, profileName, profile, spec, suites, "", now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSummarizeArtifactRejectsContentFindings(t *testing.T) {
	artifact, profileName, profile, spec, suites := promotionFixture(t)
	artifact.Context.ContentCheckFindings = []string{"missing text"}
	if _, err := SummarizeArtifact(artifact, profileName, profile, spec, suites, "", time.Now()); err == nil || !strings.Contains(err.Error(), "content check findings") {
		t.Fatalf("err = %v", err)
	}
}
