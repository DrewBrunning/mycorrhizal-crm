package controllers

import (
	"encoding/csv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1796 / ADR 0023 amendment: the flat CSV is a full-fidelity export, so
// it carries a trailing "Target Closeness" column (empty for no target).
func TestExportCSV_TargetClosenessColumn(t *testing.T) {
	f := seedCSVFullFidelity(t)
	require.NoError(t, f.db.Exec("UPDATE contacts SET target_closeness = ? WHERE id = ?", "close", f.ada.ID).Error)

	body := csvFidelityGet(t, f.router, "/export")
	section := body[:strings.Index(body, "\n=== RELATIONSHIPS ===")]
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(section, "=== CONTACTS ===\n"))).ReadAll()
	require.NoError(t, err)

	header := rows[0]
	col := -1
	for i, h := range header {
		if h == "Target Closeness" {
			col = i
		}
	}
	require.GreaterOrEqual(t, col, 0, "missing Target Closeness column in %v", header)

	got := map[string]string{}
	for _, r := range rows[1:] {
		got[r[1]] = r[col]
	}
	assert.Equal(t, "close", got["Ada"])
	assert.Equal(t, "", got["Bob"], "no target is an empty cell")
}
