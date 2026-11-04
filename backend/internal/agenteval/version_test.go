package agenteval

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCheckAgentVersion(t *testing.T) {
	ok := func(output string) func(context.Context, string, string, time.Duration) CommandResult {
		return func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{ExitCode: 0, Output: output}
		}
	}
	cases := []struct {
		name     string
		declared string
		command  string
		run      func(context.Context, string, string, time.Duration) CommandResult
		want     string
		wantErr  string
	}{
		{name: "match", declared: "1.19.4", command: "agent --version", run: ok("1.19.4\n"), want: "1.19.4"},
		{name: "prerelease", declared: "1.19.4-beta.1", command: "agent --version", run: ok("opencode 1.19.4-beta.1\n"), want: "1.19.4-beta.1"},
		{name: "drift", declared: "1.19.4", command: "agent --version", run: ok("1.20.0\n"), want: "1.20.0", wantErr: "version drift"},
		{name: "no version", declared: "1.19.4", command: "agent --version", run: ok("not a version\n"), wantErr: "did not report a version"},
		{name: "non-zero exit", declared: "1.19.4", command: "agent --version", run: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{ExitCode: 1, Output: "boom"}
		}, wantErr: "probe exited 1"},
		{name: "timeout", declared: "1.19.4", command: "agent --version", run: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{TimedOut: true}
		}, wantErr: "timed out"},
		{name: "no command", declared: "1.19.4", command: "", run: ok(""), wantErr: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := CheckAgentVersion(context.Background(), t.TempDir(), tc.declared, tc.command, time.Minute, tc.run)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("observed = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRunRecordsAgentVersion(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	opts := Options{
		Root:                root,
		Spec:                spec,
		Suites:              []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand:        "true",
		AgentVersion:        "1.2.3",
		AgentVersionCommand: "agent --version",
		VersionRunner: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{ExitCode: 0, Output: "agent 1.2.3\n"}
		},
	}
	artifact, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Status != "passed" || artifact.ObservedAgentVersion != "1.2.3" || artifact.AgentVersionDrift {
		t.Fatalf("artifact = %+v", artifact)
	}
}

func TestRunFailsClosedOnAgentVersionDrift(t *testing.T) {
	root, sha := initRepo(t)
	spec := baseSpec(t, root, sha)
	opts := Options{
		Root:                root,
		Spec:                spec,
		Suites:              []Suite{{Name: "demo", Commands: []string{"true"}}},
		AgentCommand:        "true",
		AgentVersion:        "1.2.3",
		AgentVersionCommand: "agent --version",
		VersionRunner: func(context.Context, string, string, time.Duration) CommandResult {
			return CommandResult{ExitCode: 0, Output: "agent 1.3.0\n"}
		},
	}
	artifact, err := Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "version drift") {
		t.Fatalf("err = %v", err)
	}
	if artifact.Status != "error" || !artifact.AgentVersionDrift || artifact.ObservedAgentVersion != "1.3.0" {
		t.Fatalf("artifact = %+v", artifact)
	}
	if artifact.Context != nil {
		t.Fatal("agent ran despite a version drift")
	}
}

func TestCheckAgentVersionRequiresDeclaredVersion(t *testing.T) {
	_, err := CheckAgentVersion(context.Background(), t.TempDir(), "", "agent --version", time.Minute, func(context.Context, string, string, time.Duration) CommandResult {
		return CommandResult{ExitCode: 0, Output: "1.2.3"}
	})
	if err == nil || !strings.Contains(err.Error(), "no declared version") {
		t.Fatalf("err = %v", err)
	}
}
