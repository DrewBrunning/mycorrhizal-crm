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

// Issue #1823: an activity's next_due is its UTC calendar day + the interval,
// never that day after conversion into the reminder zone. A 03:00Z activity on
// 23 Aug is still the 23rd (22:00 on the 22nd in Chicago), so 30 days later is
// 22 Sep. Auckland shows the mirror case: 13:00Z on 22 Aug is already the 23rd
// there, but the activity remains dated the 22nd, so it is due 21 Sep.
func TestCalendarOnlyNextDue_ActivityUsesUTCDay(t *testing.T) {
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

	assert.JSONEq(t, `"2026-09-22"`, nextDue(deriveHealth(&last, 30, now)))

	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	nowNZ := time.Date(2026, 10, 1, 9, 0, 0, 0, auckland)
	lastNZ := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	assert.JSONEq(t, `"2026-09-21"`, nextDue(deriveHealth(&lastNZ, 30, nowNZ)))
}

// Data decay's baseline is a real instant (the verify click / CreatedAt), not
// an activity's date-only value, so it keeps the reminder-zone calendar day.
// This is deliberately different from deriveHealth above (ADR 0015's
// 2026-10-21 amendment); the date-only marshal is still exercised here.
func TestCalendarOnlyNextDue_DataDecayUsesReminderZoneInstantDay(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, chicago)
	verified := time.Date(2026, 8, 23, 3, 0, 0, 0, time.UTC)

	nextDue := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		var raw map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(b, &raw))
		return string(raw["next_due"])
	}

	decay := ComputeDataDecayHealth(&models.DataDecayPolicy{IntervalDays: 30, LastVerifiedAt: &verified}, now)
	assert.JSONEq(t, `"2026-09-21"`, nextDue(decay))

	// 13:00Z on 22 Aug is 01:00 on 23 Aug in Auckland (NZST): due 22 Sep.
	auckland, err := time.LoadLocation("Pacific/Auckland")
	require.NoError(t, err)
	nowNZ := time.Date(2026, 10, 1, 9, 0, 0, 0, auckland)
	verifiedNZ := time.Date(2026, 8, 22, 13, 0, 0, 0, time.UTC)
	decayNZ := ComputeDataDecayHealth(&models.DataDecayPolicy{IntervalDays: 30, LastVerifiedAt: &verifiedNZ}, nowNZ)
	assert.JSONEq(t, `"2026-09-22"`, nextDue(decayNZ))
}
