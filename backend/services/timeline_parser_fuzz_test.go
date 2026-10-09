package services

import (
	"strings"
	"testing"
	"time"

	"mycorrhizal/models"
)

// maxTimelineFuzzUnix is 9999-12-31T23:59:59Z, the last instant Go's
// RFC3339Nano formatter writes with a four-digit year. Fuzzed timestamps are
// folded into [0, maxTimelineFuzzUnix) so the encoded cursor stays inside the
// RFC3339 grammar time.Parse can read back (a five-digit year is not a
// timeline bug, just a fixture-shape limit).
const maxTimelineFuzzUnix = int64(253402300799)

// timelineFuzzTime folds arbitrary fuzzed seconds/nanoseconds into a
// canonical, RFC3339-representable UTC instant.
func timelineFuzzTime(sec, nsec int64) time.Time {
	sec = ((sec % maxTimelineFuzzUnix) + maxTimelineFuzzUnix) % maxTimelineFuzzUnix
	ns := int64(time.Second)
	nsec = ((nsec % ns) + ns) % ns
	return time.Unix(sec, nsec).UTC()
}

// isUintPKTimelineType reports whether a timeline type's backing table uses a
// uint (gorm.Model) primary key. resolveTimelineCursorIDs validates a numeric
// id only for these three; the other three carry UUID-string PKs.
func isUintPKTimelineType(typ string) bool {
	switch typ {
	case models.TimelineTypeNote, models.TimelineTypeActivity, models.TimelineTypeCompletion:
		return true
	default:
		return false
	}
}

// FuzzTimelineCursorAndParams fuzzes the timeline's opaque cursor (issue
// #1626): DecodeTimelineCursor decodes a client-supplied ?cursor= that
// encodes base64url("<RFC3339Nano date>|<type>|<id>"). The property is the
// same two-sided one the other cursor parsers carry — a well-formed cursor
// round-trips, a malformed or unknown-typed one is rejected — plus the
// timeline-specific rule that a uint-PK type's id must actually be numeric
// when resolveTimelineCursorIDs re-types it for SQL (a malformed numeric id
// fails loudly rather than silently matching nothing).
//
// The "limit is clamped" half of the issue's property lives in
// controllers.parseTimelineParams, which services cannot import without a
// cycle; it is covered by controllers/cursor_fuzz_test.go's
// FuzzParseTimelineParams instead. Unknown type is rejected here, by
// DecodeTimelineCursor.
func FuzzTimelineCursorAndParams(f *testing.F) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 1, time.UTC)
	f.Add(EncodeTimelineCursor(base, models.TimelineTypeGift, "abc"), "gift", "abc", base.Unix(), int64(base.Nanosecond()))
	f.Add(EncodeTimelineCursor(base, models.TimelineTypeNote, "42"), "note", "42", base.Unix(), int64(base.Nanosecond()))
	f.Add(EncodeTimelineCursor(base, "bogus", "1"), "bogus", "1", int64(0), int64(0))
	f.Add("not-base64", "note", "1", int64(0), int64(0))
	f.Add("", "", "", int64(0), int64(0))

	f.Fuzz(func(t *testing.T, raw, typ, id string, sec, nsec int64) {
		// raw is attacker-controlled: exactly one of value/error, never a
		// zero-value cursor pretending to be a valid position.
		if cur, err := DecodeTimelineCursor(raw); (cur == nil) == (err == nil) {
			t.Fatalf("DecodeTimelineCursor(%q) = (%v, %v); want exactly one of value/error", raw, cur, err)
		}

		// The type component cannot contain the "|" delimiter or the split
		// shifts; the id is the trailing component and may.
		if typ == "" || strings.Contains(typ, "|") || id == "" {
			return
		}
		date := timelineFuzzTime(sec, nsec)
		_, known := models.TimelineTypeRank(typ)
		cur, err := DecodeTimelineCursor(EncodeTimelineCursor(date, typ, id))
		if !known {
			if err == nil {
				t.Fatalf("DecodeTimelineCursor accepted unknown type %q", typ)
			}
			return
		}
		if err != nil {
			t.Fatalf("DecodeTimelineCursor(EncodeTimelineCursor(%v, %q, %q)) = %v", date, typ, id, err)
		}
		if cur.Type != typ || cur.ID != id || !cur.Date.Equal(date) {
			t.Fatalf("timeline cursor round trip: got (%v, %q, %q), want (%v, %q, %q)", cur.Date, cur.Type, cur.ID, date, typ, id)
		}

		_, err = resolveTimelineCursorIDs(models.TimelineTypes, cur)
		if isUintPKTimelineType(typ) {
			if _, ok := parseUintID(id); !ok {
				if err == nil {
					t.Fatalf("resolveTimelineCursorIDs accepted a non-numeric id %q for uint-PK type %q", id, typ)
				}
			} else if err != nil {
				t.Fatalf("resolveTimelineCursorIDs(%q) rejected numeric id %q: %v", typ, id, err)
			}
		} else if err != nil {
			t.Fatalf("resolveTimelineCursorIDs rejected a string-PK cursor of type %q: %v", typ, err)
		}
	})
}
