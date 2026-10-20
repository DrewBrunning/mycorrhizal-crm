package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCalendarDate_MarshalsOwnLocationDateOnly(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	require.NoError(t, err)

	// Local midnight in Chicago is 05:00Z; the calendar date is the local one.
	b, err := json.Marshal(NewCalendarDate(time.Date(2026, 9, 21, 0, 0, 0, 0, chicago)))
	require.NoError(t, err)
	assert.JSONEq(t, `"2026-09-21"`, string(b))

	// UTC midnight must not render as a timestamp.
	b, err = json.Marshal(NewCalendarDate(time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)))
	require.NoError(t, err)
	assert.JSONEq(t, `"2026-09-21"`, string(b))
}

func TestCalendarDate_PointerNilAndOmitempty(t *testing.T) {
	assert.Nil(t, NewCalendarDatePtr(nil))
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	require.NotNil(t, NewCalendarDatePtr(&now))

	b, err := json.Marshal(struct {
		D *CalendarDate `json:"d,omitempty"`
	}{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(b))
}

func TestCalendarDate_Unmarshal(t *testing.T) {
	var d CalendarDate
	require.NoError(t, json.Unmarshal([]byte(`"2026-09-21"`), &d))
	assert.Equal(t, "2026-09-21", d.Format("2006-01-02"))

	// Lenient: a pre-#1778 server's RFC 3339 value keeps its own digits.
	require.NoError(t, json.Unmarshal([]byte(`"2026-09-21T00:00:00Z"`), &d))
	assert.Equal(t, "2026-09-21", d.Format("2006-01-02"))

	assert.Error(t, json.Unmarshal([]byte(`"nope"`), &d))
	assert.Error(t, json.Unmarshal([]byte(`123`), &d))
}
