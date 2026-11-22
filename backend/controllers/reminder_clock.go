package controllers

import (
	"regexp"
	"strings"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"

	"github.com/gin-gonic/gin"
)

// The controllers' source of "now" is the request's injected clock
// (internal/clock, issue #1494): every handler that decides which calendar day
// is "today" must go through reminderNow (or viewerNow, for display-only
// reads that accept ?tz) below, never through a raw
// time.Now() whose zone is the server's own local zone. Tests pin an instant
// by installing a clock.Fake on the router (clock.Install) — no package-level
// swap, so nothing can leak across tests.
// reminderNow returns the current instant localized to the operator's single
// reminder clock (REMINDER_TIME + REMINDER_TIMEZONE) — the zone every
// day-boundary decision in the product is made in (docs/adrs/0015-temporal-
// semantics.md Rule 4). "Today" in the digest, the timeline, and the cadence
// health derivation all mean "today in this zone"; the interactive endpoints
// used to call time.Now() bare and silently use the server's local zone
// instead, which could disagree with the email by a day on a host whose local
// zone differs from REMINDER_TIMEZONE. DATE-02 (issue #483) aligned them.
//
// This is the one zone-aware step for date-only / partial values: the stored
// YYYY-MM-DD or --MM-DD is never itself converted — only the question "which
// calendar day is today" is answered in the reminder zone.
func reminderNow(c *gin.Context) time.Time {
	cfg := currentConfig(c)
	return clock.FromContext(c).Now().In(cfg.GetReminderLocation())
}

// viewerTZPattern is the cheap allowlist in front of time.LoadLocation: IANA
// names are letters, digits, '_', '+', '-' and '/' separators. It keeps path
// tricks ("../x", absolute paths, NULs) and oversized input away from the
// zoneinfo lookup. 64 covers the longest IANA name with room to spare.
var viewerTZPattern = regexp.MustCompile(`^[A-Za-z0-9_+\-]+(/[A-Za-z0-9_+\-]+)*$`)

const viewerTZMaxLen = 64

// parseViewerTZ resolves the optional ?tz=<IANA zone> display parameter
// (issue #2762, docs/adrs/0015-temporal-semantics.md Rule 4). An absent or
// literally empty value means "no viewer zone" (nil, nil); a whitespace-only,
// oversized, unknown, or "Local" value is an error. "Local" is refused
// because it names the server process's own zone, which is exactly the
// ambiguity this parameter exists to avoid.
func parseViewerTZ(raw string) (*time.Location, bool) {
	if raw == "" {
		return nil, true
	}
	name := strings.TrimSpace(raw)
	if name == "" || len(name) > viewerTZMaxLen || !viewerTZPattern.MatchString(name) || strings.EqualFold(name, "Local") {
		return nil, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	return loc, true
}

// viewerNow is reminderNow for display-only endpoints: when the request
// carries a valid ?tz it answers "which day is today" in the viewer's zone,
// otherwise in the operator's reminder zone, unchanged. An invalid zone
// aborts the request with 400 and the validation_timezone_invalid reason and
// returns ok=false. Scheduled reminders, emails and ICS/CalDAV output never
// call this: they stay on REMINDER_TIMEZONE (Rule 4).
func viewerNow(c *gin.Context) (time.Time, bool) {
	loc, ok := parseViewerTZ(c.Query("tz"))
	if !ok {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("tz", "must be an IANA time zone name").WithReason(apperrors.ReasonValidationTimezoneInvalid, nil))
		return time.Time{}, false
	}
	if loc == nil {
		return reminderNow(c), true
	}
	return clock.FromContext(c).Now().In(loc), true
}
