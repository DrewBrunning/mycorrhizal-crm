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

// Issue #1778: next_due is the calendar day in the reminder zone (the zone
// `now` carries), never the UTC date. A baseline at 03:00Z on 23 Aug is 22:00
// on 22 Aug in Chicago, so a 30-day interval is due 21 Sep there; deriving the
// day in UTC instead would give 22 Sep.
func TestCalendarOnlyNextDue_UsesReminderZoneDay(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, chicago)
	last := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)

	nextDue := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		return string(raw["next_due"])
	}

	assert.JSONEq(t, `"2026-09-21"`, nextDue(deriveHealth(&last, 30, now)))
	decay := ComputeDataDecayHealth(&models.DataDecayPolicy{IntervalDays: 30, LastVerifiedAt: &last}, now)
	assert.JSONEq(t, `"2026-09-21"`, nextDue(decay))

	// East of UTC the derived local midnight is the previous UTC day, so this
	// also catches a marshal that converts to UTC before formatting. 13:00Z on
	// 22 Aug is 01:00 on 23 Aug in Auckland (NZST): due 22 Sep there.
	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	nowNZ := time.Date(2026, 10, 1, 9, 0, 0, 0, auckland)
	lastNZ := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	assert.JSONEq(t, `"2026-09-22"`, nextDue(deriveHealth(&lastNZ, 30, nowNZ)))
	decayNZ := ComputeDataDecayHealth(&models.DataDecayPolicy{IntervalDays: 30, LastVerifiedAt: &lastNZ}, nowNZ)
	assert.JSONEq(t, `"2026-09-22"`, nextDue(decayNZ))
}
