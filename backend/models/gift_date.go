package models

import "time"

// GiftCalendarDay normalizes a gift date to the calendar day it denotes,
// stored as that day's UTC midnight -- the same shape Activity.Date and
// Note.Date use (ADR 0015 category 2; Gift.Date was reclassified from an
// instant to a calendar day by the ADR 0015 amendment for the v1.5.0 review).
//
// A gift date is read as an interaction calendar day by the score and cadence
// derivation (interactionCalendarDay, issue #1736/#1823), so every write path
// stores it in that shape:
//
//   - a value already at exactly 00:00:00Z is a calendar day and is kept;
//   - any other instant (an older client's local midnight, "mark given now")
//     is read in loc and that wall-clock date is kept. loc is the reminder zone
//     for API writes; nil reads the value in its own offset (imports, whose
//     RFC 3339 values carry the writer's offset).
func GiftCalendarDay(t time.Time, loc *time.Location) time.Time {
	u := t.UTC()
	if u.Hour() == 0 && u.Minute() == 0 && u.Second() == 0 && u.Nanosecond() == 0 {
		return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
	}
	in := t
	if loc != nil {
		in = t.In(loc)
	}
	return time.Date(in.Year(), in.Month(), in.Day(), 0, 0, 0, 0, time.UTC)
}

// GiftCalendarDayPtr is GiftCalendarDay for the nullable column (nil stays nil).
func GiftCalendarDayPtr(t *time.Time, loc *time.Location) *time.Time {
	if t == nil {
		return nil
	}
	d := GiftCalendarDay(*t, loc)
	return &d
}
