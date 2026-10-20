package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// CalendarDate is a time.Time that serializes as a date-only RFC 3339
// full-date ("YYYY-MM-DD"), for values that are a calendar day rather than an
// instant — derived due dates such as cadence/data-decay `next_due`.
//
// Why it exists (issue #1778): those values are computed as midnight in the
// reminder zone, and a plain time.Time marshals them as an RFC 3339 instant.
// On a server running in UTC that is `…T00:00:00Z`, which a client in a zone
// west of UTC renders as the *previous* day (web converts timestamps to the
// local zone; see formatDateWithFormat's #962 note). A date-only string has
// no zone to shift, so every client reads the same calendar digits.
//
// The date written is the calendar date in the value's own location (the
// zone the derivation already normalized to), never converted to UTC.
type CalendarDate struct {
	time.Time
}

// NewCalendarDate wraps t as a CalendarDate.
func NewCalendarDate(t time.Time) CalendarDate { return CalendarDate{Time: t} }

// NewCalendarDatePtr wraps t, or returns nil for a nil input.
func NewCalendarDatePtr(t *time.Time) *CalendarDate {
	if t == nil {
		return nil
	}
	return &CalendarDate{Time: *t}
}

const calendarDateLayout = "2006-01-02"

// MarshalJSON writes the value's own-location calendar date as "YYYY-MM-DD".
func (d CalendarDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Format(calendarDateLayout))
}

// UnmarshalJSON accepts a date-only string, or (leniently, for payloads
// written by an older server) a full RFC 3339 timestamp, whose own calendar
// digits are kept.
func (d *CalendarDate) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if t, err := time.Parse(calendarDateLayout, s); err == nil {
		d.Time = t
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("calendar date %q: want YYYY-MM-DD", s)
	}
	d.Time = t
	return nil
}
