package services

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"time"

	"mycorrhizal/models"

	"gorm.io/gorm"
)

// Operational error aggregation, issue #426.
//
// Individual failures (a CardDAV auth rejection, an SMTP timeout, a SQLITE_BUSY,
// a Gotify 401) reach an operator only as separate rows in the operational-event
// timeline (system_events, issue #424). A recurring problem — the same cause
// failing 17 times in a day — is invisible without manual forensics.
//
// AggregateOperationalErrors folds the failure rows of system_events over a
// rolling window into one bucket per *cause*: (component, normalized error).
// normalizeErrorCause masks the parts of an error string that vary run to run
// (ids, counts, hosts, URLs, timestamps) so "carddav auth rejected for
// subscription 4821 (HTTP 401)" and "... 9137 (HTTP 403)" collapse to one
// bucket while "database is locked (SQLITE_BUSY)" stays its own.
//
// Like ComputeSubsystemHealth (issue #427) this is a sibling read-side fold
// over the same stream: no table, no migration, no second write path. It is
// derived on read, so it survives a restart and can never drift from the events
// it summarizes.

const (
	// errAggRecurringThreshold is the count at which a bucket is flagged
	// Recurring — the "a single transient failure should not raise an alarm,
	// but a cause that repeats should be prominent" rule from the issue. The
	// clients use it only for emphasis; the count is always reported.
	errAggRecurringThreshold = 3

	// errAggMaxEventIDs caps the per-bucket EventIDs list (the exact
	// system_events rows behind the bucket, for the timeline drill-down). A
	// bucket with more sets EventIDsTruncated.
	errAggMaxEventIDs = 500

	// errAggCauseMaxLen caps the normalized cause key.
	errAggCauseMaxLen = 200
)

// ErrorBucket is one aggregated operational-error cause over the window.
type ErrorBucket struct {
	// Component is the system_events.component the failures share.
	Component string `json:"component"`
	// Cause is the normalized error string — the low-cardinality bucket key.
	Cause string `json:"cause"`
	// SampleError is the most recent *raw* error string in the bucket, so an
	// operator still sees a real instance (already sanitized and length-capped
	// by RecordSystemEvent).
	SampleError string `json:"sample_error"`
	// EventTypes is the sorted distinct set of event_type values in the bucket
	// (usually one, e.g. sync_failed).
	EventTypes []string `json:"event_types"`

	Count     int  `json:"count"`
	Recurring bool `json:"recurring"`

	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`

	// EventIDs are the system_events row ids in the bucket (capped at
	// errAggMaxEventIDs), for the timeline's ?ids= drill-down.
	EventIDs          []uint `json:"event_ids"`
	EventIDsTruncated bool   `json:"event_ids_truncated"`
}

// causeMasks is the ordered list of substitutions normalizeErrorCause applies.
// Order matters: timestamps and UUIDs before the generic hex/number masks, URLs
// and hostnames before the bare-number mask so a port or an IP is not
// half-masked. Each entry replaces its match with a stable placeholder.
var causeMasks = []struct {
	re   *regexp.Regexp
	repl string
}{
	// RFC3339-ish timestamps (the input is already lower-cased).
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}[t ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:z|[+-]\d{2}:?\d{2})?`), "<ts>"},
	// UUIDs.
	{regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "<uuid>"},
	// URLs.
	{regexp.MustCompile(`https?://[^\s"')]+`), "<url>"},
	// Hostnames (with an optional :port) — at least one dot and an alpha TLD,
	// so a bare IPv4 falls through to the next mask instead.
	{regexp.MustCompile(`\b(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}(?::\d+)?\b`), "<host>"},
	// IPv4 with an optional :port.
	{regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b`), "<ip>"},
	// Unix-ish paths (two or more segments).
	{regexp.MustCompile(`(?:/[\w.-]+){2,}/?`), "<path>"},
	// Hex literals and long hex runs (object ids, hashes).
	{regexp.MustCompile(`\b0x[0-9a-f]+\b`), "<hex>"},
	{regexp.MustCompile(`\b[0-9a-f]{16,}\b`), "<hex>"},
	// Quoted substrings.
	{regexp.MustCompile(`"[^"]*"`), "<v>"},
	{regexp.MustCompile(`'[^']*'`), "<v>"},
	// Any remaining digit run (counts, HTTP status codes, ids, "30s", "500ms").
	// Not boundary-anchored on purpose: a number glued to a unit still varies
	// run to run, and the issue's "SMTP timeout after 30s" / "after 5s" must
	// collapse to one cause.
	{regexp.MustCompile(`\d+`), "<n>"},
}

var causeWhitespace = regexp.MustCompile(`\s+`)

// normalizeErrorCause collapses a raw error string to a low-cardinality "cause"
// key by masking the parts that vary run to run. It is the one piece of this
// feature with real judgement, so it is unit-tested directly against the
// issue's examples (services/error_aggregation_test.go).
func normalizeErrorCause(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, m := range causeMasks {
		s = m.re.ReplaceAllString(s, m.repl)
	}
	s = strings.TrimSpace(causeWhitespace.ReplaceAllString(s, " "))
	return truncateCause(s, errAggCauseMaxLen)
}

func truncateCause(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// AggregateOperationalErrors returns one ErrorBucket per (component, normalized
// cause) for every system_events failure row with a non-empty error string that
// occurred at or after `since`, plus the total number of source rows folded.
// Buckets are sorted by Count desc, then LastSeen desc.
//
// A failure event with no error string carries no cause to aggregate and is
// excluded (the only known producer is the restore-drill row-count-mismatch
// path, which sets `detail` rather than `error`).
func AggregateOperationalErrors(ctx context.Context, db *gorm.DB, since time.Time) ([]ErrorBucket, int, error) {
	var rows []models.SystemEvent
	err := db.WithContext(ctx).
		Where("error <> '' AND occurred_at >= ?", since).
		Order("occurred_at ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}

	type bucketKey struct{ component, cause string }
	byKey := map[bucketKey]*ErrorBucket{}
	typeSeen := map[bucketKey]map[string]struct{}{}
	var order []bucketKey

	for _, r := range rows {
		k := bucketKey{r.Component, normalizeErrorCause(r.Error)}
		b := byKey[k]
		if b == nil {
			b = &ErrorBucket{
				Component: r.Component,
				Cause:     k.cause,
				FirstSeen: r.OccurredAt,
			}
			byKey[k] = b
			typeSeen[k] = map[string]struct{}{}
			order = append(order, k)
		}
		b.Count++
		b.LastSeen = r.OccurredAt
		// rows are ordered oldest-first, so the final write is the newest raw
		// error.
		b.SampleError = r.Error
		if _, ok := typeSeen[k][r.EventType]; !ok {
			typeSeen[k][r.EventType] = struct{}{}
			b.EventTypes = append(b.EventTypes, r.EventType)
		}
		if len(b.EventIDs) < errAggMaxEventIDs {
			b.EventIDs = append(b.EventIDs, r.ID)
		} else {
			b.EventIDsTruncated = true
		}
	}

	out := make([]ErrorBucket, 0, len(order))
	for _, k := range order {
		b := byKey[k]
		b.Recurring = b.Count >= errAggRecurringThreshold
		sort.Strings(b.EventTypes)
		out = append(out, *b)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out, len(rows), nil
}
