package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFeedBeforeCreateGeneratesUUID pins the UUID-PK contract (ADR 0030): a
// new Feed with no id gets one, and an explicitly-set id is preserved (the
// account-bundle/import path may carry an existing id).
func TestFeedBeforeCreateGeneratesUUID(t *testing.T) {
	f := &Feed{}
	require.NoError(t, f.BeforeCreate(nil))
	assert.NotEmpty(t, f.ID)

	kept := &Feed{ID: "00000000-0000-4000-8000-000000000001"}
	require.NoError(t, kept.BeforeCreate(nil))
	assert.Equal(t, "00000000-0000-4000-8000-000000000001", kept.ID)
}

func TestFeedTableName(t *testing.T) {
	assert.Equal(t, "feeds", Feed{}.TableName())
}
