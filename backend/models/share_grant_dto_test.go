package models

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNullableTime_PresenceAndNull(t *testing.T) {
	var absent ShareGrantPatchInput
	require.NoError(t, json.Unmarshal([]byte(`{}`), &absent))
	assert.False(t, absent.ExpiresAt.Set)

	var null ShareGrantPatchInput
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":null}`), &null))
	assert.True(t, null.ExpiresAt.Set)
	assert.Nil(t, null.ExpiresAt.Value)

	var set ShareGrantPatchInput
	require.NoError(t, json.Unmarshal([]byte(`{"expires_at":"2026-11-07T12:00:00Z"}`), &set))
	require.NotNil(t, set.ExpiresAt.Value)
	assert.True(t, set.ExpiresAt.Value.Equal(time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)))

	var bad ShareGrantPatchInput
	assert.Error(t, json.Unmarshal([]byte(`{"expires_at":"not a time"}`), &bad))
}

func TestNullableTime_MarshalRoundTrip(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	b, err := json.Marshal(NullableTime{Set: true, Value: &now})
	require.NoError(t, err)
	assert.JSONEq(t, `"2026-11-07T12:00:00Z"`, string(b))
	b, err = json.Marshal(NullableTime{})
	require.NoError(t, err)
	assert.Equal(t, "null", string(b))
}
