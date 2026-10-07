package controllers

import (
	"time"

	"mycorrhizal/internal/clock"

	"github.com/gin-gonic/gin"
)

// The controllers' source of "now" is the request's injected clock
// (internal/clock, issue #1494): every handler that decides which calendar day
// is "today" must go through reminderNow below, never through a raw
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
