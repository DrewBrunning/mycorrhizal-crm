package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionalDateJSON(t *testing.T) {
	var in ActivityInput
	require.NoError(t, json.Unmarshal([]byte(`{"title":"t","date":"2026-05-12T00:00:00Z"}`), &in))
	assert.False(t, in.EndDate.Set, "absent key stays unset")
	assert.True(t, in.EndDate.IsZero())

	for _, raw := range []string{`null`, `""`} {
		var in ActivityInput
		require.NoError(t, json.Unmarshal([]byte(`{"end_date":`+raw+`}`), &in))
		assert.True(t, in.EndDate.Set)
		assert.Nil(t, in.EndDate.Value, raw)
	}

	require.NoError(t, json.Unmarshal([]byte(`{"end_date":"2026-05-15T00:00:00Z"}`), &in))
	require.NotNil(t, in.EndDate.Value)
	assert.Equal(t, time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC), *in.EndDate.Value)

	assert.Error(t, json.Unmarshal([]byte(`{"end_date":12}`), &in))
	assert.Error(t, json.Unmarshal([]byte(`{"end_date":"soon"}`), &in))

	// Marshal: unset is omitted, cleared is null, a value is RFC 3339.
	out, err := json.Marshal(ActivityInput{Title: "t"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "end_date")
	out, err = json.Marshal(ActivityInput{EndDate: OptionalDate{Set: true}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"end_date":null`)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	out, err = json.Marshal(ActivityInput{EndDate: OptionalDate{Set: true, Value: &end}})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"end_date":"2026-05-15T00:00:00Z"`)
}

func TestActivityEndBeforeStartComparesUTCDays(t *testing.T) {
	start := time.Date(2026, 5, 12, 23, 0, 0, 0, time.UTC)
	same := time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 5, 11, 0, 0, 0, 0, time.UTC)
	assert.False(t, ActivityEndBeforeStart(start, nil))
	assert.False(t, ActivityEndBeforeStart(start, &same), "same UTC day is a valid zero-length range")
	assert.True(t, ActivityEndBeforeStart(start, &before))
}
