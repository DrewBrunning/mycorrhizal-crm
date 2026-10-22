package controllers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreatedCursorEncodeDecodeRoundTrip proves an issue-#1762 created_at
// cursor survives the base64url round trip with both timestamp and id intact,
// including the timestamp's offset.
func TestCreatedCursorEncodeDecodeRoundTrip(t *testing.T) {
	for _, id := range []any{uint(7), uint64(42), "1f2e3d4c-5b6a-7890-abcd-ef1234567890"} {
		ts := time.Date(2026, 8, 2, 18, 7, 19, 402476499, time.FixedZone("CDT", -5*3600))
		cur, err := DecodeCreatedCursor(EncodeCreatedCursor(ts, id))
		require.NoError(t, err)
		assert.True(t, ts.Equal(cur.UpdatedAt), "timestamp must survive the round trip")
		_, off := cur.UpdatedAt.Zone()
		assert.Equal(t, -5*3600, off, "timestamp offset must survive the round trip")
		assert.Equal(t, fmt.Sprint(id), cur.ID, "id must survive the round trip")
	}
}

// TestCreatedCursorDecodeRejectsMalformed pins the created_at cursor's 400
// paths, including the cross-shape case: an updated_at cursor has no
// "created:" prefix and must not decode as a created_at cursor.
func TestCreatedCursorDecodeRejectsMalformed(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		errFrag string
	}{
		{"!!!not-base64url!!!", "cursor is not valid base64url"},
		{EncodeCursor(time.Date(2026, 8, 2, 18, 7, 19, 0, time.UTC), uint(7)), "expected a created_at-sorted cursor"},
		{encodeRawURL("created:"), "cursor is malformed"},                       // missing timestamp and id
		{encodeRawURL("created:2026-08-02T18:07:19Z|"), "cursor is malformed"},  // missing id
		{encodeRawURL("created:|7"), "cursor is malformed"},                     // missing timestamp
		{encodeRawURL("created:not-a-time|7"), "cursor timestamp is malformed"}, // bad timestamp
	} {
		_, err := DecodeCreatedCursor(tc.raw)
		assert.ErrorContains(t, err, tc.errFrag, "cursor %q should fail to decode", tc.raw)
	}
}
