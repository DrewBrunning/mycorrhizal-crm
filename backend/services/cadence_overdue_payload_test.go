package services

import (
	"encoding/json"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1778: the cadence.overdue webhook payload carries next_due as a
// date-only YYYY-MM-DD (same calendar-day semantics as the REST health), not
// an RFC 3339 instant. Reads the raw marshalled JSON.
func TestBuildOverduePayload_NextDueIsDateOnly(t *testing.T) {
	last := time.Date(2026, 8, 22, 15, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	health := deriveHealth(&last, 30, now)
	require.NotNil(t, health.NextDue)

	payload := buildOverduePayload(models.CadencePolicy{EntityID: "uid-1", TargetIntervalDays: 30}, health, 7)
	b, err := json.Marshal(payload)
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &raw))
	assert.JSONEq(t, `"2026-09-21"`, string(raw["next_due"]))
}
