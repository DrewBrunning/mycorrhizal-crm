package agenteval

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// agentVersionRe is deliberately permissive about the surrounding text: agent
// CLIs print "opencode 1.19.4", just "1.19.4", or a build suffix. The first
// semantic version is the measurement.
var agentVersionRe = regexp.MustCompile(`\b(\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?)\b`)

// CheckAgentVersion runs a profile's version command and verifies the installed
// agent matches the pinned profile. A profile is only comparable while the CLI
// version is fixed; this turns a silent npx/toolchain resolution drift into a
// visible error before any model call is made.
func CheckAgentVersion(ctx context.Context, dir, declared, command string, timeout time.Duration, run func(context.Context, string, string, time.Duration) CommandResult) (string, error) {
	if strings.TrimSpace(command) == "" {
		return "", nil
	}
	if strings.TrimSpace(declared) == "" {
		return "", fmt.Errorf("agent version probe %q has no declared version", command)
	}
	result := run(ctx, dir, command, timeout)
	if result.TimedOut {
		return "", fmt.Errorf("agent version probe timed out: %s", command)
	}
	if result.ExitCode != 0 {
		return "", fmt.Errorf("agent version probe exited %d: %s", result.ExitCode, command)
	}
	match := agentVersionRe.FindStringSubmatch(result.Output)
	if match == nil {
		return "", fmt.Errorf("agent version probe %q did not report a version", command)
	}
	observed := match[1]
	if observed != declared {
		return observed, fmt.Errorf("agent CLI version drift: profile pins %s, installed %s", declared, observed)
	}
	return observed, nil
}
