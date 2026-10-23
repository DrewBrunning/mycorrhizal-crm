package services

import (
	"fmt"
	"math"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CadenceHealth is the DERIVED relationship-health read model —
// computed from the timeline, never persisted. There is deliberately no
// next_due column anywhere in the schema; this struct is the only place
// those values exist.
//
// Semantics:
//   - HasQualifyingInteraction is false (and NextDue/LastInteraction nil)
//     until the contact has at least one qualifying interaction — cadence
//     does NOT count from the contact's creation date. This is T19's decided
//     answer to "a contact with no qualifying interaction ever has no last".
//   - NextDue = most recent qualifying interaction's date + interval days.
//   - OverdueBy is whole calendar days past due (0 when due today, in the
//     future, or undefined). "Due today" is deliberately not overdue,
//     matching reminders' day boundary.
type CadenceHealth struct {
	HasQualifyingInteraction bool                 `json:"has_qualifying_interaction"`
	LastInteraction          *time.Time           `json:"last_interaction,omitempty"`
	NextDue                  *models.CalendarDate `json:"next_due,omitempty"`
	OverdueBy                int                  `json:"overdue_by"`
	// Ambient is always on the wire (issue #1733): true when the contact counts
	// as in regular contact without logged interactions (they live with the
	// user, or the user flagged them), in which case OverdueBy is forced to 0 --
	// an ambient contact is never overdue. AmbientReason is "household" or
	// "flag", omitted when Ambient is false.
	Ambient       bool   `json:"ambient"`
	AmbientReason string `json:"ambient_reason,omitempty"`
}

// withAmbient marks h ambient (issue #1733): never overdue, reason recorded.
// An empty reason leaves h untouched.
func (h CadenceHealth) withAmbient(reason string) CadenceHealth {
	if reason == "" {
		return h
	}
	h.Ambient = true
	h.AmbientReason = reason
	h.OverdueBy = 0
	return h
}

// calendarDaysBetween counts whole calendar days from a to b (negative when
// b precedes a), normalizing both to local midnight in a's location. Rounding
// the elapsed-hours/24 guards the DST boundary, where two local midnights are
// 23 or 25 absolute hours apart and truncation would silently drop a day —
// the same "overdue near midnight" off-by-one the ticket warns about
// (DaysUntilBirthday solves the adjacent problem; this is its DST-hardened
// sibling for arbitrary dates, including the Dec 31 → Jan 1 wrap via
// AddDate).
func calendarDaysBetween(a, b time.Time) int {
	loc := a.Location()
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	aMid := time.Date(ay, am, ad, 0, 0, 0, 0, loc)
	bMid := time.Date(by, bm, bd, 0, 0, 0, 0, loc)
	return int(math.Round(bMid.Sub(aMid).Hours() / 24))
}

// lastQualifyingInteraction returns the date of the most recent activity for
// the given contact that the policy's Qualifies() accepts, or nil when none
// exists. QualifyingTypes narrows the SQL first (when non-empty); the
// authoritative Activity.Qualifying() gate is then applied in Go via
// policy.Qualifies(), so the globally non-qualifying types (photo) can never
// sneak in through an explicit policy listing.
//
// Ownership scoping is double-sided on purpose: both the activity AND the
// contact must belong to userID. A normal write path can only link an
// activity to a contact the same user owns (CreateActivity verifies contact
// ownership), but the join table is the one place a cross-user link could
// lurk, so the derivation refuses to read it.
func lastQualifyingInteraction(db *gorm.DB, userID uint, policy *models.CadencePolicy) (*time.Time, error) {
	q := db.Model(&models.Activity{}).
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id").
		Where("c.vcard_uid = ? AND c.user_id = ? AND activities.user_id = ? AND activities.deleted_at IS NULL",
			policy.EntityID, userID, userID)
	if len(policy.QualifyingTypes) > 0 {
		q = q.Where("activities.type IN ?", policy.QualifyingTypes)
	}

	var activities []models.Activity
	if err := q.Order("activities.date DESC").Find(&activities).Error; err != nil {
		return nil, err
	}
	var best *time.Time
	for i := range activities {
		if policy.Qualifies(&activities[i]) {
			t := activities[i].Date
			best = &t
			break
		}
	}

	// Issue #1736: a given gift is a qualifying interaction of synthetic type
	// "gift"; take the later of it and the best activity.
	if policyAcceptsGifts(policy) {
		gifts, err := givenGiftDateRows(db, userID, []string{policy.EntityID}, nil)
		if err != nil {
			return nil, err
		}
		if t := firstQualifyingDate(gifts, policy); t != nil && (best == nil || t.After(*best)) {
			best = t
		}
	}
	return best, nil
}

// giftInteractionType is the synthetic interaction type a given gift counts as
// (issue #1736). Frontend/Android cadence qualifying-type suggestion lists
// mirror it (frontend trap #4): keep in sync.
const giftInteractionType = "gift"

// policyAcceptsGifts reports whether a cadence policy counts given gifts: an
// empty QualifyingTypes list means every default-qualifying type (so yes), a
// listed policy only when "gift" is listed.
func policyAcceptsGifts(p *models.CadencePolicy) bool {
	candidate := &models.Activity{Type: giftInteractionType}
	return p.Qualifies(candidate)
}

// givenGiftDateRows returns one row (date, type "gift", vcard_uid) per live
// gift with status "given" and a non-null date for each of the given contacts
// (issue #1736): the bulk, ownership-scoped gift counterpart of
// activityDateRowsQuery, in no particular order. since, when non-nil, is a
// coarse SQL prefilter (bound in UTC; the caller applies the exact calendar-day
// cut). One query regardless of len(uids).
//
// A gift whose activity_id points at a live activity linked to the same contact
// is skipped: that activity is already counted as an activity (the optional
// "gift" timeline entry a gift can be linked to, models.Gift.ActivityID), and
// counting both would double a single handover in Frequency.
//
// The join order starts from the contacts, as activityDateRowsQuery does.
func givenGiftDateRows(db *gorm.DB, userID uint, uids []string, since *time.Time) ([]activityDateRow, error) {
	q := db.Table("contacts c").
		Select("g.date AS date, ? AS type, c.vcard_uid AS vcard_uid", giftInteractionType).
		// INDEXED BY: otherwise the planner probes g through idx_gifts_user_id
		// (or status), rescanning every gift of the user once per contact --
		// O(contacts x gifts), the bulk of graph.full's time at the large
		// profile. The entity index makes it one probe per contact.
		Joins("CROSS JOIN gifts g INDEXED BY idx_gifts_entity_id ON g.entity_id = c.vcard_uid").
		Where("c.vcard_uid IN ? AND c.user_id = ? AND g.user_id = ? AND g.deleted_at IS NULL AND g.status = ? AND g.date IS NOT NULL",
			uids, userID, userID, models.GiftStatusGiven).
		Where(`(g.activity_id IS NULL OR NOT EXISTS (
			SELECT 1 FROM activities la
			JOIN activity_contacts lac ON lac.activity_id = la.id
			WHERE la.id = g.activity_id AND lac.contact_id = c.id
			  AND la.user_id = ? AND la.deleted_at IS NULL))`, userID)
	if since != nil {
		q = q.Where("g.date >= ?", since.UTC())
	}
	var rows []activityDateRow
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// mergeRowsDateDesc concatenates activity and gift rows and orders them date
// DESC (stable), the order firstQualifyingDate / groupActivityRowsByUID expect.
func mergeRowsDateDesc(a, b []activityDateRow) []activityDateRow {
	if len(b) == 0 {
		return a
	}
	merged := make([]activityDateRow, 0, len(a)+len(b))
	merged = append(merged, a...)
	merged = append(merged, b...)
	slices.SortStableFunc(merged, func(x, y activityDateRow) int { return y.Date.Compare(x.Date) })
	return merged
}

// interactionCalendarDay is the calendar day an interaction date denotes.
//
// Activity.Date and Note.Date are calendar days (ADR 0015's category 2), even
// though the wire still transports them as RFC 3339 instants: every client
// writes the picked day at 00:00:00Z. The meaningful calendar date is
// therefore the stored value's UTC date, never its date after conversion to
// the reminder zone -- issue #1823, where an activity dated 2026-10-22 became
// 2026-10-21 19:00 in America/Chicago and pulled next_due a day early.
func interactionCalendarDay(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// deriveHealth completes the health read-model from an already-resolved last
// qualifying interaction date. Shared between ComputeCadenceHealth (single-
// policy, live query) and the batch path (fetchLastInteractionDates +
// per-policy deriveHealth).
//
// The activity date supplies the calendar day as stored (interactionCalendarDay,
// UTC); `now` supplies which day is "today" in the reminder zone. The two are
// compared as calendar days, so the derivation is zone-independent for a
// date-only interaction (issue #1823).
func deriveHealth(last *time.Time, intervalDays int, now time.Time) CadenceHealth {
	if last == nil {
		return CadenceHealth{HasQualifyingInteraction: false}
	}
	nextDue := interactionCalendarDay(*last).AddDate(0, 0, intervalDays)
	health := CadenceHealth{
		HasQualifyingInteraction: true,
		LastInteraction:          last,
		NextDue:                  &models.CalendarDate{Time: nextDue},
	}
	if overdue := -calendarDaysBetween(now, nextDue); overdue > 0 {
		health.OverdueBy = overdue
	}
	return health
}

// ComputeCadenceHealth derives the health read-model for one policy against
// the real timeline. Pure — performs no writes. `now` anchors the "today"
// boundary; callers should pass a timezone-appropriate now (the reminder
// location) so the day boundary matches the user's clock.
//
// The derivation normalises the last qualifying interaction to midnight in
// the user's timezone. Without this, an activity time stored in UTC that
// crosses the user's midnight (e.g. 20:00 UTC = 09:00 next day NZ) would
// carry the wrong wall-clock date, and overdue_by would be off by one day.
func ComputeCadenceHealth(db *gorm.DB, userID uint, policy *models.CadencePolicy, now time.Time) (CadenceHealth, error) {
	last, err := lastQualifyingInteraction(db, userID, policy)
	if err != nil {
		return CadenceHealth{}, err
	}
	ambient, err := ambientByEntityID(db, userID, []string{policy.EntityID}, now)
	if err != nil {
		return CadenceHealth{}, err
	}
	return deriveHealth(last, policy.TargetIntervalDays, now).withAmbient(ambient[policy.EntityID]), nil
}

// ComputeCadenceHealthBatch is ComputeCadenceHealth for a page of policies in a
// constant number of queries (the batch last-interaction fetch, its gift union
// and one ambient lookup), keyed by policy ID. It agrees with
// ComputeCadenceHealth policy for policy; the list endpoint uses it so a page
// of N policies is not 3N queries (issue #1733/#1736 added the gift and ambient
// reads to the per-policy path).
func ComputeCadenceHealthBatch(db *gorm.DB, userID uint, policies []models.CadencePolicy, now time.Time) (map[string]CadenceHealth, error) {
	out := make(map[string]CadenceHealth, len(policies))
	if len(policies) == 0 {
		return out, nil
	}
	lastByPolicy, err := fetchLastInteractionDates(db, userID, policies)
	if err != nil {
		return nil, err
	}
	uids := make([]string, 0, len(policies))
	for i := range policies {
		uids = append(uids, policies[i].EntityID)
	}
	ambient, err := ambientByEntityID(db, userID, uids, now)
	if err != nil {
		return nil, err
	}
	for i := range policies {
		p := &policies[i]
		out[p.ID] = deriveHealth(lastByPolicy[p.ID], p.TargetIntervalDays, now).withAmbient(ambient[p.EntityID])
	}
	return out, nil
}

// activityDateRow is the light scan target for the batch activity pre-fetch.
// The VCardUID column tag is mandatory: without it GORM derives v_card_uid
// from the Go field name while the SQL alias is the raw column name vcard_uid.
type activityDateRow struct {
	Date     time.Time
	Type     string
	VCardUID string `gorm:"column:vcard_uid"`
}

// activityDateRowsQuery is the shared read behind every "latest/ count of
// qualifying activities per contact" derivation: one row (date, type,
// vcard_uid) per (live activity, linked contact) pair for the given contacts,
// ownership-scoped on both sides.
//
// The join order is FORCED to start from the contacts (CROSS JOIN is SQLite's
// documented way to pin the nested-loop order). Left to itself the planner
// drives from `activities` and re-probes the whole `vcard_uid IN (...)` list
// for every activity row -- activities x uids index probes. With a cadence
// policy on ~1.7k of 15k contacts and ~10k linked activities that was ~17M
// probes and 12s for what is a 30ms query when walked contacts-first (issue
// #1791). Callers add their own filters/ordering.
func activityDateRowsQuery(db *gorm.DB, userID uint, uids []string) *gorm.DB {
	return db.Table("contacts c").
		Select("a.date AS date, a.type AS type, c.vcard_uid AS vcard_uid").
		Joins("CROSS JOIN activity_contacts ac ON ac.contact_id = c.id").
		Joins("CROSS JOIN activities a ON a.id = ac.activity_id").
		Where("c.vcard_uid IN ? AND c.user_id = ? AND a.user_id = ? AND a.deleted_at IS NULL",
			uids, userID, userID)
}

// groupActivityRowsByUID buckets activity rows by contact vCard UID, keeping
// the input order within each bucket (so a date-DESC input stays date-DESC).
func groupActivityRowsByUID(rows []activityDateRow) map[string][]activityDateRow {
	byUID := make(map[string][]activityDateRow)
	for i := range rows {
		byUID[rows[i].VCardUID] = append(byUID[rows[i].VCardUID], rows[i])
	}
	return byUID
}

// firstQualifyingDate returns the first (most recent, given date-DESC input)
// row's date that policy.Qualifies accepts, or nil.
func firstQualifyingDate(rows []activityDateRow, policy *models.CadencePolicy) *time.Time {
	for i := range rows {
		candidate := &models.Activity{Type: rows[i].Type, Date: rows[i].Date}
		if policy.Qualifies(candidate) {
			t := rows[i].Date
			return &t
		}
	}
	return nil
}

// fetchLastInteractionDates loads the most recent qualifying interaction date
// for every policy (all must share the same userID) in a single query,
// replacing the N+1 pattern of calling lastQualifyingInteraction per policy.
// Returns a map of policy ID → most recent qualifying date (nil when none).
//
// The qualifying_types filter is pushed into SQL when every policy has a
// non-empty list (union of all listed types). If any policy uses the
// empty-list default ("every default-qualifying type counts"), the SQL filter
// is skipped and the Go-level Qualifies gate handles the filtering.
func fetchLastInteractionDates(db *gorm.DB, userID uint, policies []models.CadencePolicy) (map[string]*time.Time, error) {
	if len(policies) == 0 {
		return nil, nil
	}

	uids := make([]string, 0, len(policies))
	seen := make(map[string]bool)
	hasEmptyFilter := false
	allTypes := make(map[string]bool)
	for _, p := range policies {
		if !seen[p.EntityID] {
			uids = append(uids, p.EntityID)
			seen[p.EntityID] = true
		}
		if len(p.QualifyingTypes) == 0 {
			hasEmptyFilter = true
		}
		for _, t := range p.QualifyingTypes {
			allTypes[t] = true
		}
	}

	query := activityDateRowsQuery(db, userID, uids)

	if !hasEmptyFilter && len(allTypes) > 0 {
		typeList := make([]string, 0, len(allTypes))
		for t := range allTypes {
			typeList = append(typeList, t)
		}
		query = query.Where("a.type IN ?", typeList)
	}

	var rows []activityDateRow
	if err := query.Order("a.date DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	// Issue #1736: union in given gifts (one extra bulk query) when any policy
	// could count them -- an empty-list policy, or one listing "gift".
	if hasEmptyFilter || allTypes[giftInteractionType] {
		gifts, err := givenGiftDateRows(db, userID, uids, nil)
		if err != nil {
			return nil, err
		}
		rows = mergeRowsDateDesc(rows, gifts)
	}

	// Group the (date DESC ordered) rows by contact once, so each policy only
	// walks its own contact's activities: O(rows + policies), not
	// O(policies x rows) (issue #1791 -- a policy on every contact of a
	// 15k-contact account made the old nested scan the dashboard's cost).
	byUID := groupActivityRowsByUID(rows)

	result := make(map[string]*time.Time, len(policies))
	for _, p := range policies {
		result[p.ID] = firstQualifyingDate(byUID[p.EntityID], &p)
	}
	return result, nil
}

// identity — the payload for the overdue list ("the screen people will
// actually live in") and the webhook job.
type OverdueCadence struct {
	Policy         models.CadencePolicy `json:"policy"`
	Health         CadenceHealth        `json:"health"`
	ContactID      uint                 `json:"contact_id"`
	ContactName    string               `json:"contact_name"`
	PhotoThumbnail string               `json:"photo_thumbnail,omitempty"`
}

// ListOverdueCadences returns every cadence policy of the user that is
// currently overdue, in order of most-overdue first, joined with the
// contact's numeric ID and display name (the frontend links to
// /contacts/<numeric id>). Overdue means derived OverdueBy > 0 — a policy
// with no qualifying interaction ever can never be overdue.
//
// Activities are pre-fetched in a single query (fetchLastInteractionDates)
// rather than N per-policy queries, since a personal-CRM user may have a
// policy on every contact and N round-trips would be wasteful.
func ListOverdueCadences(db *gorm.DB, userID uint, now time.Time) ([]OverdueCadence, error) {
	var policies []models.CadencePolicy
	if err := db.Where("user_id = ?", userID).Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}

	lastByPolicy, err := fetchLastInteractionDates(db, userID, policies)
	if err != nil {
		return nil, err
	}

	// Derive health first and resolve contacts only for the overdue ones
	// (issue #1791): the contact row carries the encrypted Card (decoded for
	// the deceased check), and a policy on every contact of a 15k-contact
	// account made loading all of them the dominant cost when only a few
	// hundred are overdue.
	type overduePolicy struct {
		policy models.CadencePolicy
		health CadenceHealth
	}
	var candidates []overduePolicy
	uidSet := make(map[string]bool)
	for i := range policies {
		health := deriveHealth(lastByPolicy[policies[i].ID], policies[i].TargetIntervalDays, now)
		if health.OverdueBy <= 0 {
			continue
		}
		candidates = append(candidates, overduePolicy{policies[i], health})
		uidSet[policies[i].EntityID] = true
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	uids := make([]string, 0, len(uidSet))
	for uid := range uidSet {
		uids = append(uids, uid)
	}
	// Issue #1733: an ambient contact is never overdue. Looked up only for the
	// contacts that would otherwise be listed, in bulk.
	ambient, err := ambientByEntityID(db, userID, uids, now)
	if err != nil {
		return nil, err
	}
	var contacts []models.Contact
	// Exclude the caller's own self contact (issue #1793): a cadence policy on
	// "me" is not a relationship to maintain. The row is dropped from the map
	// below, so the candidate loop must tolerate a missing contact.
	if err := ExcludeSelfContact(db.Where("user_id = ? AND vcard_uid IN ?", userID, uids), userID).
		Find(&contacts).Error; err != nil {
		return nil, err
	}
	contactByUID := make(map[string]models.Contact, len(contacts))
	for _, c := range contacts {
		contactByUID[c.VCardUID] = c
	}

	var overdue []OverdueCadence
	for _, cand := range candidates {
		if _, isAmbient := ambient[cand.policy.EntityID]; isAmbient {
			continue
		}
		c, exists := contactByUID[cand.policy.EntityID]
		if !exists {
			// The self contact (or a racing delete) is intentionally absent.
			continue
		}
		// Issue #1193: a deceased contact generates no overdue-cadence
		// nudges -- there is no one left to reach out to.
		if c.Card.IsDeceased() {
			continue
		}
		overdue = append(overdue, OverdueCadence{
			Policy:         cand.policy,
			Health:         cand.health,
			ContactID:      c.ID,
			ContactName:    strings.TrimSpace(c.Firstname + " " + c.Lastname),
			PhotoThumbnail: c.PhotoThumbnail,
		})
	}

	sortOverdueByMostOverdue(overdue)
	return overdue, nil
}

// sortOverdueByMostOverdue orders the overdue list so the most-neglected
// relationship is first — the top of the screen people live in.
func sortOverdueByMostOverdue(overdue []OverdueCadence) {
	slices.SortFunc(overdue, func(a, b OverdueCadence) int {
		return b.Health.OverdueBy - a.Health.OverdueBy
	})
}

// cadenceOverdueMinInterval is slightly less than the daily cron cadence so a
// natural clock-skew or overlap doesn't cause a skipped run — the same
// margin purge_service uses.
var cadenceOverdueMinInterval = JobCatchupWindow(24 * time.Hour)

// ProcessOverdueCadences is the scheduled job that emits a `cadence.overdue`
// webhook per currently-overdue cadence policy, so an external task manager
// (Vikunja) can materialize a task. Guarded by the job lock (T19 item 4) so
// a multi-instance deploy does not double-fire.
//
// Firing policy (decided for T19): emits once per daily run for every policy
// that is still overdue — the CRM owns the cadence state and the external
// manager is expected to idempotently handle the repeat (the spec says the
// webhook MAY emit a task). Completing such a task is deliberately NOT a
// reset signal; only recording a qualifying interaction resets cadence.
//
// The initial load scans every cadence_policies row across all users. In a
// single-user personal CRM this is harmless; in a multi-tenant instance it
// would be an unbounded scan once per day. At that scale the loop should be
// driven by iterating users rather than loading every policy at once.
//
// Returns the number of overdue webhooks emitted and, for the job-run record
// (issue #391), ErrJobSkipped when the lock is held / it ran too recently.
func ProcessOverdueCadences(db *gorm.DB, cfg config.Config) (int, error) {
	ctx := logger.JobContext(models.JobNameCadenceOverdue)
	acquired, err := acquireJobLock(db, models.JobNameCadenceOverdue, cadenceOverdueMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("cadence: failed to check job lock")
		return 0, err
	}
	if !acquired {
		logger.Info().Msg("cadence: skipping overdue job - rate limited")
		return 0, ErrJobSkipped
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameCadenceOverdue, true); err != nil {
			logger.Error().Err(err).Msg("cadence: failed to release job lock")
		}
	}()

	now := Now().In(cfg.GetReminderLocation())

	var policies []models.CadencePolicy
	if err := db.Find(&policies).Error; err != nil {
		logger.Error().Err(err).Msg("cadence: failed to load policies for overdue webhooks")
		return 0, fmt.Errorf("loading cadence policies: %w", err)
	}
	if len(policies) == 0 {
		return 0, nil
	}

	// Group policies by UserID so the batch activity pre-fetch runs once
	// per user (at personal-CRM scale this is exactly one user).
	byUser := make(map[uint][]models.CadencePolicy)
	for _, p := range policies {
		byUser[p.UserID] = append(byUser[p.UserID], p)
	}

	// Resolve every referenced contact's numeric ID per user for the
	// webhook payload. Runs alongside the activity pre-fetch, not before it.
	type key struct {
		userID uint
		uid    string
	}
	contactID := make(map[key]uint)

	emitted := 0
	for userID, userPolicies := range byUser {
		lastByPolicy, err := fetchLastInteractionDates(db, userID, userPolicies)
		if err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("cadence: failed to pre-fetch activities for webhooks")
			continue
		}

		// Resolve contacts for this user.
		uids := make([]string, len(userPolicies))
		for i, p := range userPolicies {
			uids[i] = p.EntityID
		}
		var contacts []models.Contact
		if err := ExcludeSelfContact(db.Where("user_id = ? AND vcard_uid IN ?", userID, uids), userID).
			Find(&contacts).Error; err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("cadence: failed to resolve contacts for webhook payload")
			continue
		}
		// Issue #1193: deceased contacts (Card.Anniversaries[kind=death]) are
		// tracked here too, so ProcessOverdueCadences can skip emitting a
		// cadence.overdue webhook for them below.
		deceasedByUID := make(map[string]bool, len(contacts))
		for _, c := range contacts {
			contactID[key{userID, c.VCardUID}] = c.ID
			if c.Card.IsDeceased() {
				deceasedByUID[c.VCardUID] = true
			}
		}

		// Issue #1733: an ambient contact is never overdue, so no
		// cadence.overdue webhook either.
		ambient, err := ambientByEntityID(db, userID, uids, now)
		if err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("cadence: failed to resolve ambient contacts for webhooks")
			continue
		}

		for i := range userPolicies {
			p := userPolicies[i]
			if _, resolved := contactID[key{p.UserID, p.EntityID}]; !resolved {
				// The self contact (issue #1793) is excluded above; a vanished
				// contact has no webhook payload either.
				continue
			}
			if deceasedByUID[p.EntityID] {
				continue
			}
			health := deriveHealth(lastByPolicy[p.ID], p.TargetIntervalDays, now)
			if health.OverdueBy <= 0 {
				continue
			}
			if _, isAmbient := ambient[p.EntityID]; isAmbient {
				continue
			}
			payload := buildOverduePayload(p, health, contactID[key{p.UserID, p.EntityID}])
			TriggerWebhooksAsync(ctx, db, cfg, p.UserID, "cadence.overdue", payload)
			emitted++
		}
	}

	if emitted > 0 {
		logger.Info().Int("emitted", emitted).Msg("cadence: emitted overdue webhooks")
	}
	return emitted, nil
}

// buildOverduePayload constructs the `cadence.overdue` webhook payload from
// a policy and its derived health.
func buildOverduePayload(p models.CadencePolicy, health CadenceHealth, contactID uint) map[string]interface{} {
	return map[string]interface{}{
		"cadence_policy_id":           p.ID,
		"entity_id":                   p.EntityID,
		"target_interval_days":        p.TargetIntervalDays,
		"qualifying_types":            p.QualifyingTypes,
		"overdue_by":                  health.OverdueBy,
		"next_due":                    health.NextDue,
		"last_qualifying_interaction": health.LastInteraction,
		"contact_id":                  contactID,
	}
}
