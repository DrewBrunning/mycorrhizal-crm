package releaseworkflow

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1802: GitHub validates reusable-workflow permissions statically for
// the whole call graph, before any job starts. A nested job that declares a
// scope its caller did not grant makes the entire dispatch fail with
// `startup_failure` (no jobs, no useful annotation). The release composer is
// two levels deep, so this walks it from both entry points and asserts every
// called job's effective permissions fit its caller's grant.
//
// Effective permissions are a job's own `permissions:` block, or the
// workflow's top-level block when the job has none -- the same inheritance
// GitHub applies. None of these workflows use the read-all/write-all shorthand.
func TestReleaseCallGraphGrantsNestedPermissions(t *testing.T) {
	dir := filepath.Join(repoRoot(t), ".github", "workflows")

	var findings []string
	for _, entry := range []string{"release.yml", "docker-publish.yml"} {
		b, err := os.ReadFile(filepath.Join(dir, entry))
		require.NoError(t, err)
		doc, errs := parseDoc(entry, string(b))
		require.Empty(t, errs, "%s does not parse", entry)

		jobs := asMap(doc["jobs"])
		names := make([]string, 0, len(jobs))
		for name := range jobs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			job := asMap(jobs[name])
			callee := localCallFile(job)
			if callee == "" {
				continue
			}
			findings = append(findings, checkCallPermissions(t, dir, doc, job, callee, entry+"#"+name)...)
		}
	}
	assert.Empty(t, findings)
}

// localCallFile returns the basename of a job's local reusable-workflow call,
// or "" when the job is not one.
func localCallFile(job anyMap) string {
	const prefix = "./.github/workflows/"
	uses := asStr(job["uses"])
	if !strings.HasPrefix(uses, prefix) {
		return ""
	}
	return filepath.Base(uses)
}

func jobPermissions(doc, job anyMap) anyMap {
	if p := asMap(job["permissions"]); p != nil {
		return p
	}
	return asMap(doc["permissions"])
}

func permissionLevel(v string) int {
	switch v {
	case "read":
		return 1
	case "write":
		return 2
	default:
		return 0
	}
}

// checkCallPermissions checks one call level and recurses into any local call
// the called workflow makes, carrying that job's permissions down as the new
// ceiling.
func checkCallPermissions(t *testing.T, dir string, callerDoc, callerJob anyMap, calleeFile, chain string) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, calleeFile))
	require.NoError(t, err)
	callee, errs := parseDoc(calleeFile, string(b))
	require.Empty(t, errs, "%s does not parse", calleeFile)

	ceiling := jobPermissions(callerDoc, callerJob)
	jobs := asMap(callee["jobs"])
	names := make([]string, 0, len(jobs))
	for name := range jobs {
		names = append(names, name)
	}
	sort.Strings(names)

	var findings []string
	for _, name := range names {
		job := asMap(jobs[name])
		granted := jobPermissions(callee, job)
		for scope, want := range granted {
			got := asStr(ceiling[scope])
			if permissionLevel(asStr(want)) > permissionLevel(got) {
				if got == "" {
					got = "none"
				}
				findings = append(findings, fmt.Sprintf(
					"%s -> %s#%s declares %s: %s, but the caller grants %s: %s (GitHub fails the whole run at startup)",
					chain, calleeFile, name, scope, asStr(want), scope, got,
				))
			}
		}
		if next := localCallFile(job); next != "" {
			findings = append(findings, checkCallPermissions(t, dir, callee, job, next, chain+" -> "+calleeFile+"#"+name)...)
		}
	}
	return findings
}
