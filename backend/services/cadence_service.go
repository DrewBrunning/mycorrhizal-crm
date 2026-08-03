package services

import (
	"math"
	"mycorrhizal/config"
	"mycorrhizal/logger"
	"mycorrhizal/models"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// CadenceHealth is the DERIVED relationship-health read model (§91.10) —
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
	HasQualifyingInteraction bool       `json:"has_qualifying_interaction"`
	LastInteraction          *time.Time `json:"last_interaction,omitempty"`
	NextDue                  *time.Time `json:"next_due,omitempty"`
	OverdueBy                int        `json:"overdue_by"`
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
	for i := range activities {
		if policy.Qualifies(&activities[i]) {
			t := activities[i].Date
			return &t, nil
		}
	}
	return nil, nil
}

// deriveHealth completes the health read-model from an already-resolved last
// qualifying interaction date. Shared between ComputeCadenceHealth (single-
// policy, live query) and the batch path (fetchLastInteractionDates +
// per-policy deriveHealth).
func deriveHealth(last *time.Time, intervalDays int, now time.Time) CadenceHealth {
	if last == nil {
		return CadenceHealth{HasQualifyingInteraction: false}
	}
	loc := now.Location()
	lastLocal := last.In(loc)
	lastMidnight := time.Date(lastLocal.Year(), lastLocal.Month(), lastLocal.Day(), 0, 0, 0, 0, loc)
	nextDue := lastMidnight.AddDate(0, 0, intervalDays)
	health := CadenceHealth{
		HasQualifyingInteraction: true,
		LastInteraction:          last,
		NextDue:                  &nextDue,
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
	return deriveHealth(last, policy.TargetIntervalDays, now), nil
}

// activityDateRow is the light scan target for the batch activity pre-fetch.
// The VCardUID column tag is mandatory: without it GORM derives v_card_uid
// from the Go field name while the SQL alias is the raw column name vcard_uid.
type activityDateRow struct {
	Date     time.Time
	Type     string
	VCardUID string `gorm:"column:vcard_uid"`
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

	query := db.Table("activities").
		Select("activities.date, activities.type, c.vcard_uid").
		Joins("JOIN activity_contacts ac ON ac.activity_id = activities.id").
		Joins("JOIN contacts c ON c.id = ac.contact_id").
		Where("c.vcard_uid IN ? AND c.user_id = ? AND activities.user_id = ? AND activities.deleted_at IS NULL",
			uids, userID, userID)

	if !hasEmptyFilter && len(allTypes) > 0 {
		typeList := make([]string, 0, len(allTypes))
		for t := range allTypes {
			typeList = append(typeList, t)
		}
		query = query.Where("activities.type IN ?", typeList)
	}

	var rows []activityDateRow
	if err := query.Order("activities.date DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[string]*time.Time, len(policies))
	for _, p := range policies {
		result[p.ID] = nil
		for i := range rows {
			if rows[i].VCardUID != p.EntityID {
				continue
			}
			candidate := &models.Activity{Type: rows[i].Type, Date: rows[i].Date}
			if p.Qualifies(candidate) {
				t := rows[i].Date
				result[p.ID] = &t
				break
			}
		}
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

	// Resolve all referenced contacts in one query for the display fields.
	uidSet := make(map[string]bool)
	for _, p := range policies {
		uidSet[p.EntityID] = true
	}
	uids := make([]string, 0, len(uidSet))
	for uid := range uidSet {
		uids = append(uids, uid)
	}
	var contacts []models.Contact
	if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
		return nil, err
	}
	contactByUID := make(map[string]models.Contact, len(contacts))
	for _, c := range contacts {
		contactByUID[c.VCardUID] = c
	}

	var overdue []OverdueCadence
	for i := range policies {
		health := deriveHealth(lastByPolicy[policies[i].ID], policies[i].TargetIntervalDays, now)
		if health.OverdueBy <= 0 {
			continue
		}
		c := contactByUID[policies[i].EntityID]
		overdue = append(overdue, OverdueCadence{
			Policy:         policies[i],
			Health:         health,
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
const cadenceOverdueMinInterval = 23 * time.Hour

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
func ProcessOverdueCadences(db *gorm.DB, cfg config.Config) {
	acquired, err := acquireJobLock(db, models.JobNameCadenceOverdue, cadenceOverdueMinInterval)
	if err != nil {
		logger.Error().Err(err).Msg("cadence: failed to check job lock")
		return
	}
	if !acquired {
		logger.Info().Msg("cadence: skipping overdue job - rate limited")
		return
	}
	defer func() {
		if err := releaseJobLock(db, models.JobNameCadenceOverdue, true); err != nil {
			logger.Error().Err(err).Msg("cadence: failed to release job lock")
		}
	}()

	now := time.Now().In(cfg.GetReminderLocation())

	var policies []models.CadencePolicy
	if err := db.Find(&policies).Error; err != nil {
		logger.Error().Err(err).Msg("cadence: failed to load policies for overdue webhooks")
		return
	}
	if len(policies) == 0 {
		return
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
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("cadence: failed to resolve contacts for webhook payload")
			continue
		}
		for _, c := range contacts {
			contactID[key{userID, c.VCardUID}] = c.ID
		}

		for i := range userPolicies {
			p := userPolicies[i]
			health := deriveHealth(lastByPolicy[p.ID], p.TargetIntervalDays, now)
			if health.OverdueBy <= 0 {
				continue
			}
			payload := buildOverduePayload(p, health, contactID[key{p.UserID, p.EntityID}])
			go TriggerWebhooks(db, cfg, p.UserID, "cadence.overdue", payload)
			emitted++
		}
	}

	if emitted > 0 {
		logger.Info().Int("emitted", emitted).Msg("cadence: emitted overdue webhooks")
	}
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
