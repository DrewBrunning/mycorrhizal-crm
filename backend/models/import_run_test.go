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
