package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Issue #2463: Detail always yields an array, whatever the column holds.
func TestImportRunDetail_AlwaysAnArray(t *testing.T) {
	for name, raw := range map[string]string{
		"empty":     "",
		"null":      "null",
		"malformed": "{not json",
		"no issues": "[]",
	} {
		got := ImportRun{IssuesJSON: raw}.Detail()
		assert.NotNil(t, got.Issues, name)
		assert.Empty(t, got.Issues, name)
	}

	got := ImportRun{IssuesJSON: `[{"row":3,"name":"A","email":"","phone":"","status":"failed","reason":"r"}]`}.Detail()
	assert.Equal(t, []ImportRowIssue{{Row: 3, Name: "A", Status: "failed", Reason: "r"}}, got.Issues)
}

// Issue #2741: warning issues share the cap with skipped/failed ones but never
// crowd them out, and keep their concept.
func TestStoredImportIssues_WarningsNeverCrowdOutSkippedAndFailed(t *testing.T) {
	var in []ImportRowIssue
	for i := 0; i < MaxImportRunIssues; i++ {
		in = append(in, ImportRowIssue{Row: i + 1, Status: "warning", Concept: "birthday"})
	}
	in = append(in, ImportRowIssue{Row: 5000, Status: "failed", Reason: "x"})
	out := storedImportIssues(in)
	assert.Len(t, out, MaxImportRunIssues)
	assert.Equal(t, "failed", out[0].Status)
	assert.Equal(t, "birthday", out[1].Concept)
}
