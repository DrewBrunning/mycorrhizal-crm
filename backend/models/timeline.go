package models

import "time"

// Timeline event types — the six raw types a contact's timeline merges
// (T66, docs/fork-plan/tickets/110-T66-contact-timeline-bounded-view-and-
// explorer.md). They mirror the data model 1:1, matching the web timeline's
// discriminated union (ContactTimeline.tsx) and the type-filter vocabulary
// fixed by the T66/T78 platform split (design decision 1: revisit only if a
// second external-activity source ever ships alongside Immich).
const (
	TimelineTypeNote             = "note"
	TimelineTypeActivity         = "activity"
	TimelineTypeCompletion       = "completion"
	TimelineTypeLifeEvent        = "life_event"
	TimelineTypeExternalActivity = "external_activity"
	TimelineTypeGift             = "gift"
)

// TimelineTypes is the complete set of timeline event types, in the canonical
// order used for the merged sort's type tiebreak and the cursor's type
// component. Any deterministic order works; this one matches the web
// timeline's own union ordering so a page boundary sorts identically
// client- and server-side.
var TimelineTypes = []string{
	TimelineTypeNote,
	TimelineTypeActivity,
	TimelineTypeCompletion,
	TimelineTypeLifeEvent,
	TimelineTypeExternalActivity,
	TimelineTypeGift,
}

// TimelineTypeRank returns the canonical position of a timeline event type in
// TimelineTypes, and false when the token is not one of the six types. The
// rank is the tiebreak for two events that share a date but belong to
// different tables.
func TimelineTypeRank(typ string) (int, bool) {
	for i, t := range TimelineTypes {
		if t == typ {
			return i, true
		}
	}
	return 0, false
}

// Timeline recency buckets — the fixed filter set T66's design decision 2
// settled ("recent-ish vs. everything", never a free-form range picker):
// last 7 / 30 / 90 days, this year, all time. The web explorer (T78) matches
// this vocabulary exactly.
const (
	TimelineBucketLast7Days  = "last_7_days"
	TimelineBucketLast30Days = "last_30_days"
	TimelineBucketLast90Days = "last_90_days"
	TimelineBucketThisYear   = "this_year"
	TimelineBucketAll        = "all"
)

// TimelineItem is one entry in a contact's merged timeline (T66): the raw
// entity under Data, plus the normalized sort key the merge and cursor are
// built on. ID is always the entity's primary key as a string — the six
// tables mix uint (gorm.Model) and UUID-string PKs, and the cursor carries
// the id as a string for exactly that reason.
//
// Data is deliberately heterogeneous (the six entity types); clients switch
// on Type before reading it.
type TimelineItem struct {
	Type string      `json:"type"`
	ID   string      `json:"id"`
	Date time.Time   `json:"date"`
	Data interface{} `json:"data"`
}
