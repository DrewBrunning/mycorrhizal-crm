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

// ComputeCadenceHealth derives the health read-model for one policy against
// the real timeline. Pure — performs no writes. `now` anchors the "today"
// boundary; callers should pass a timezone-appropriate now (the reminder
// location) so the day boundary matches the user's clock.
func ComputeCadenceHealth(db *gorm.DB, userID uint, policy *models.CadencePolicy, now time.Time) (CadenceHealth, error) {
	last, err := lastQualifyingInteraction(db, userID, policy)
	if err != nil {
		return CadenceHealth{}, err
	}
	if last == nil {
		return CadenceHealth{HasQualifyingInteraction: false}, nil
	}

	nextDue := last.AddDate(0, 0, policy.TargetIntervalDays)
	health := CadenceHealth{
		HasQualifyingInteraction: true,
		LastInteraction:          last,
		NextDue:                  &nextDue,
	}
	if overdue := -calendarDaysBetween(now, nextDue); overdue > 0 {
		health.OverdueBy = overdue
	}
	return health, nil
}

// OverdueCadence is one overdue policy joined with its contact's display
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
func ListOverdueCadences(db *gorm.DB, userID uint, now time.Time) ([]OverdueCadence, error) {
	var policies []models.CadencePolicy
	if err := db.Where("user_id = ?", userID).Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
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
		health, err := ComputeCadenceHealth(db, userID, &policies[i], now)
		if err != nil {
			return nil, err
		}
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
		if a.Health.OverdueBy != b.Health.OverdueBy {
			return b.Health.OverdueBy - a.Health.OverdueBy
		}
		return 0
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

	// All users at once: overdue policies carry their own UserID, and
	// TriggerWebhooks scopes the subscription lookup per user.
	var policies []models.CadencePolicy
	if err := db.Find(&policies).Error; err != nil {
		logger.Error().Err(err).Msg("cadence: failed to load policies for overdue webhooks")
		return
	}
	if len(policies) == 0 {
		return
	}

	// Index policies per (user, entity) so the per-user health computation
	// can be driven by the same contact-lookup pass below.
	type key struct {
		userID uint
		uid    string
	}
	uidByUser := make(map[uint][]string)
	for _, p := range policies {
		uidByUser[p.UserID] = append(uidByUser[p.UserID], p.EntityID)
	}

	// Resolve every referenced contact's numeric ID (needed for the payload)
	// and cache it per (user, vcard_uid).
	contactID := make(map[key]uint)
	for userID, uids := range uidByUser {
		var contacts []models.Contact
		if err := db.Where("user_id = ? AND vcard_uid IN ?", userID, uids).Find(&contacts).Error; err != nil {
			logger.Error().Err(err).Uint("user_id", userID).Msg("cadence: failed to resolve contacts for webhook payload")
			continue
		}
		for _, c := range contacts {
			contactID[key{userID, c.VCardUID}] = c.ID
		}
	}

	emitted := 0
	for i := range policies {
		p := policies[i]
		health, err := ComputeCadenceHealth(db, p.UserID, &p, now)
		if err != nil {
			logger.Error().Err(err).Str("policy_id", p.ID).Msg("cadence: failed to compute health for webhook")
			continue
		}
		if health.OverdueBy <= 0 {
			continue
		}
		payload := ginHForCadence(p, health, contactID[key{p.UserID, p.EntityID}])
		go TriggerWebhooks(db, cfg, p.UserID, "cadence.overdue", payload)
		emitted++
	}

	if emitted > 0 {
		logger.Info().Int("emitted", emitted).Msg("cadence: emitted overdue webhooks")
	}
}

// ginHForCadence builds the `cadence.overdue` webhook payload.
func ginHForCadence(p models.CadencePolicy, health CadenceHealth, contactID uint) map[string]interface{} {
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
