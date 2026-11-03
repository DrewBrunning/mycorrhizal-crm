package agenteval

import (
	"fmt"
	"strings"
	"time"
)

// SummarizeArtifact turns a downloaded eval artifact into a baseline entry.
// Promotion is only accepted when the artifact is a passing run of the same
// profile, prompt, base ref and content checks the committed configuration now
// describes. This is the measured alternative to hand-editing config_hash while
// keeping older success counts.
func SummarizeArtifact(artifact Artifact, profileName string, profile Profile, spec Spec, suites []Suite, source string, now time.Time) (RunSummary, error) {
	if artifact.Status != "passed" || artifact.Context == nil || !artifact.Context.Success {
		return RunSummary{}, fmt.Errorf("artifact is not a passing run (status %q)", artifact.Status)
	}
	if artifact.Profile != profileName {
		return RunSummary{}, fmt.Errorf("artifact profile %q does not match %q", artifact.Profile, profileName)
	}
	if artifact.Task != spec.Name {
		return RunSummary{}, fmt.Errorf("artifact task %q does not match %q", artifact.Task, spec.Name)
	}
	if artifact.BaseRef != spec.BaseRef {
		return RunSummary{}, fmt.Errorf("artifact base ref %s does not match the committed spec %s", artifact.BaseRef, spec.BaseRef)
	}
	wantCommand := RedactCommand(ExpandPrompt(profile.Command, spec.Prompt))
	if artifact.AgentCommand != wantCommand {
		return RunSummary{}, fmt.Errorf("artifact command does not match the committed profile and prompt; re-run the task")
	}
	if artifact.AgentName != profile.Name || artifact.AgentModel != profile.Model || artifact.AgentVersion != profile.Version {
		return RunSummary{}, fmt.Errorf("artifact agent identity does not match the committed profile")
	}
	if len(spec.ContentChecks) > 0 && artifact.Context.ContentChecksRun != len(spec.ContentChecks) {
		return RunSummary{}, fmt.Errorf("artifact did not run the %d committed content check(s); re-run the task", len(spec.ContentChecks))
	}
	if len(artifact.Context.ContentCheckFindings) > 0 {
		return RunSummary{}, fmt.Errorf("artifact has content check findings: %s", strings.Join(artifact.Context.ContentCheckFindings, "; "))
	}
	summary := SummarizeRun(artifact.Status, artifact.Context, ConfigFingerprint(profile, spec, suites))
	summary.Verified = true
	summary.PromotedAt = now.UTC().Format(time.RFC3339)
	summary.PromotedFrom = source
	return summary, nil
}
