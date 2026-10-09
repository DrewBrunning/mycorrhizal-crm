package controllers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
)

// maxCursorFuzzUnix is 9999-12-31T23:59:59Z, the last instant Go's
// RFC3339Nano formatter writes with a four-digit year. The fuzzed timestamp
// is folded into [0, maxCursorFuzzUnix) before encoding because a year past
// 9999 formats to a five-digit year that time.Parse(time.RFC3339Nano, ...)
// rejects — a fixture-shape limitation of the RFC3339 4-digit-year grammar,
// not a cursor bug, so it would be a false positive here.
const maxCursorFuzzUnix = int64(253402300799)

// cursorFuzzTime folds arbitrary fuzzed seconds/nanoseconds into a canonical,
// RFC3339-representable UTC instant.
func cursorFuzzTime(sec, nsec int64) time.Time {
	sec = ((sec % maxCursorFuzzUnix) + maxCursorFuzzUnix) % maxCursorFuzzUnix
	ns := int64(time.Second)
	nsec = ((nsec % ns) + ns) % ns
	return time.Unix(sec, nsec).UTC()
}

// FuzzCursors fuzzes the opaque pagination cursors (issue #1626). These are
// client-controlled: a cursor arrives verbatim in ?cursor= / ?since= and is
// base64-decoded before any of it is trusted. The property is two-sided:
//
//   - a well-formed cursor round-trips: Decode(Encode(c)) == c;
//   - a malformed one is rejected with an error and never a zero-value
//     cursor that would silently restart paging from the beginning.
//
// EncodeCursor/DecodeCursor, EncodeNameCursor/DecodeNameCursor and
// EncodePositionCursor/DecodePositionCursor are the three wire shapes
// (helpers.go); parseCursorID re-types a decoded ID to a uint PK column and
// must agree with strconv.ParseUint.
func FuzzCursors(f *testing.F) {
	base := time.Date(2026, 3, 1, 12, 0, 0, 123456789, time.UTC)
	f.Add(EncodeCursor(base, uint(7)), "7", "Ada Lovelace", 3, base.Unix(), int64(base.Nanosecond()))
	f.Add(EncodeNameCursor("lovelace, ada", "7"), "7", "lovelace, ada", 0, int64(0), int64(0))
	f.Add(EncodePositionCursor(-4, "def"), "abc", "name|with|pipes", -4, int64(0), int64(0))
	f.Add("", "", "", 0, int64(0), int64(0))
	f.Add("not-base64!!!", "", "", 0, int64(0), int64(0))
	f.Add("////", "0", "2020-01-01T00:00:00Z", 0, int64(0), int64(0))

	f.Fuzz(func(t *testing.T, raw, id, sortName string, position int, sec, nsec int64) {
		// raw is attacker-controlled: each decoder must return either a
		// usable cursor or an error — never both nil (a silently-restarting
		// zero cursor) and never a value alongside an error.
		if cur, err := DecodeCursor(raw); (cur == nil) == (err == nil) {
			t.Fatalf("DecodeCursor(%q) = (%v, %v); want exactly one of value/error", raw, cur, err)
		}
		if cur, err := DecodeNameCursor(raw); (cur == nil) == (err == nil) {
			t.Fatalf("DecodeNameCursor(%q) = (%v, %v); want exactly one of value/error", raw, cur, err)
		}
		if cur, err := DecodePositionCursor(raw); (cur == nil) == (err == nil) {
			t.Fatalf("DecodePositionCursor(%q) = (%v, %v); want exactly one of value/error", raw, cur, err)
		}

		ts := cursorFuzzTime(sec, nsec)

		// (updated_at, id): SplitN(_, "|", 2) means the id may contain the
		// delimiter, but not be empty.
		if id != "" {
			cur, err := DecodeCursor(EncodeCursor(ts, id))
			if err != nil {
				t.Fatalf("DecodeCursor(EncodeCursor(%v, %q)) = %v", ts, id, err)
			}
			if cur.ID != id || !cur.UpdatedAt.Equal(ts) {
				t.Fatalf("time cursor round trip: got (%v, %q), want (%v, %q)", cur.UpdatedAt, cur.ID, ts, id)
			}
		}

		// (position, id): also SplitN(_, "|", 2), so any non-empty id
		// round-trips regardless of embedded delimiters.
		if id != "" {
			cur, err := DecodePositionCursor(EncodePositionCursor(position, id))
			if err != nil {
				t.Fatalf("DecodePositionCursor(EncodePositionCursor(%d, %q)) = %v", position, id, err)
			}
			if cur.ID != id || cur.Position != position {
				t.Fatalf("position cursor round trip: got (%d, %q), want (%d, %q)", cur.Position, cur.ID, position, id)
			}
		}

		// (sort_name, id): the separator is the LAST "|" (so a sort name may
		// itself contain one, but the trailing id may not), and a sort name
		// that parses as RFC3339Nano is deliberately rejected so a
		// time-based cursor cannot be reinterpreted as a name.
		if sortName != "" && id != "" && !strings.Contains(id, "|") {
			_, tsErr := time.Parse(time.RFC3339Nano, sortName)
			cur, err := DecodeNameCursor(EncodeNameCursor(sortName, id))
			if tsErr == nil {
				if err == nil {
					t.Fatalf("DecodeNameCursor accepted a timestamp-shaped sort name %q", sortName)
				}
			} else {
				if err != nil {
					t.Fatalf("DecodeNameCursor(EncodeNameCursor(%q, %q)) = %v", sortName, id, err)
				}
				if cur.SortName != sortName || cur.ID != id {
					t.Fatalf("name cursor round trip: got (%q, %q), want (%q, %q)", cur.SortName, cur.ID, sortName, id)
				}
			}
		}

		// parseCursorID must agree with strconv.ParseUint: never ok=true for
		// a non-numeric id, and the value must match when it parses.
		n, numErr := strconv.ParseUint(id, 10, 64)
		got, ok := parseCursorID(&Cursor{ID: id})
		if ok {
			// parseUintID narrows the parsed value to uint; only compare
			// where that narrowing is lossless (all supported targets are
			// 64-bit, but the guard keeps this correct on a 32-bit one).
			if numErr != nil || (n <= uint64(^uint(0)) && uint64(got) != n) {
				t.Fatalf("parseCursorID(%q) = (%d, true); ParseUint err=%v value=%d", id, got, numErr, n)
			}
		} else if numErr == nil {
			t.Fatalf("parseCursorID(%q) rejected a numeric id", id)
		}
	})
}

// FuzzParseTimelineParams fuzzes the timeline endpoint's query parser
// (controllers/timeline_controller.go, issue #1626). It lives in this
// package because parseTimelineParams is unexported and services cannot
// import controllers without a cycle — the services-side
// FuzzTimelineCursorAndParams covers DecodeTimelineCursor, and this covers
// the surrounding controls that decoder is wired into.
//
// Property: on success the parser has never fallen back silently — the limit
// is clamped to [1, maxLimit], the order is asc|desc, every type token is
// known and deduplicated, and the bucket is a known one. A malformed cursor,
// unknown type or unknown bucket is an error (400), never a silent default.
func FuzzParseTimelineParams(f *testing.F) {
	gin.SetMode(gin.TestMode)
	base := encodeTimelineCursor(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC), models.TimelineTypeNote, "7")
	f.Add("7", "asc", base, "note,activity", "last_7_days")
	f.Add("1000", "", "not-a-cursor", "note,bogus", "forever")
	f.Add("", "", "", "", "")
	f.Add("-3", "desc", "!!", "note,note", "all")
	f.Add("25", "desc", base, "note,gift,life_event", "this_year")

	f.Fuzz(func(t *testing.T, limitRaw, orderRaw, cursorRaw, typeRaw, bucketRaw string) {
		q := url.Values{}
		if limitRaw != "" {
			q.Set("limit", limitRaw)
		}
		if orderRaw != "" {
			q.Set("order", orderRaw)
		}
		if cursorRaw != "" {
			q.Set("cursor", cursorRaw)
		}
		if typeRaw != "" {
			q.Set("type", typeRaw)
		}
		if bucketRaw != "" {
			q.Set("bucket", bucketRaw)
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?"+q.Encode(), nil)

		limit, order, cur, types, bucket, appErr := parseTimelineParams(c)
		if appErr != nil {
			return
		}
		if limit < 1 || limit > maxLimit {
			t.Fatalf("limit %d out of [1, %d]", limit, maxLimit)
		}
		if n, err := strconv.Atoi(limitRaw); err == nil && n > maxLimit && limit != maxLimit {
			t.Fatalf("limit %d not clamped to %d", n, maxLimit)
		}
		if order != "asc" && order != "desc" {
			t.Fatalf("order %q is neither asc nor desc", order)
		}
		if len(types) == 0 {
			t.Fatalf("success returned no types")
		}
		seen := make(map[string]bool, len(types))
		for _, ty := range types {
			if _, ok := models.TimelineTypeRank(ty); !ok {
				t.Fatalf("success returned unknown type %q", ty)
			}
			if seen[ty] {
				t.Fatalf("success returned duplicate type %q", ty)
			}
			seen[ty] = true
		}
		if bucket == "" {
			t.Fatalf("success returned an empty bucket")
		}
		if cur != nil {
			if _, ok := models.TimelineTypeRank(cur.Type); !ok {
				t.Fatalf("success returned a cursor with unknown type %q", cur.Type)
			}
		}
	})
}
