package services

import (
	"math"
	"mycorrhizal/models"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
)

// DataDecayHealth is the DERIVED read-model for a DataDecayPolicy — computed
// from LastVerifiedAt (or CreatedAt, when never verified) + IntervalDays,
// never persisted (there is deliberately no next_due column). See
// models.DataDecayPolicy's doc comment for why LastVerifiedAt itself, unlike
// CadencePolicy's last-qualifying-interaction, IS a stored column.
//
// Unlike CadenceHealth, there is no "undefined" state: every policy has a
// baseline (CreatedAt when never verified), so NextDue is always defined.
// OverdueBy is whole calendar days past due (0 when due today, in the
// future, or undefined) — "due today" is deliberately not overdue, matching
// CadenceHealth and reminders' day-boundary convention.
type DataDecayHealth struct {
	NextDue   time.Time `json:"next_due"`
	OverdueBy int       `json:"overdue_by"`
}

// calendarDaysBetweenDecay counts whole calendar days from a to b (negative
// when b precedes a), normalizing both to local midnight in a's location.
// Duplicated from cadence_service.go's calendarDaysBetween rather than
// shared: both are small, self-contained, and a shared helper would be a
// premature abstraction over two call sites with no other coupling.
func calendarDaysBetweenDecay(a, b time.Time) int {
	loc := a.Location()
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	aMid := time.Date(ay, am, ad, 0, 0, 0, 0, loc)
	bMid := time.Date(by, bm, bd, 0, 0, 0, 0, loc)
	return int(math.Round(bMid.Sub(aMid).Hours() / 24))
}

// ComputeDataDecayHealth derives the health read-model for one policy. Pure
// and DB-free (unlike ComputeCadenceHealth): LastVerifiedAt already lives on
// the policy row, so there is no timeline to query. `now` anchors the
// "today" boundary; callers should pass a timezone-appropriate now (the
// reminder location) so the day boundary matches the user's clock.
func ComputeDataDecayHealth(policy *models.DataDecayPolicy, now time.Time) DataDecayHealth {
	baseline := policy.CreatedAt
	if policy.LastVerifiedAt != nil {
		baseline = *policy.LastVerifiedAt
	}
	loc := now.Location()
	baselineLocal := baseline.In(loc)
	baselineMidnight := time.Date(baselineLocal.Year(), baselineLocal.Month(), baselineLocal.Day(), 0, 0, 0, 0, loc)
	nextDue := baselineMidnight.AddDate(0, 0, policy.IntervalDays)
	health := DataDecayHealth{NextDue: nextDue}
	if overdue := -calendarDaysBetweenDecay(now, nextDue); overdue > 0 {
		health.OverdueBy = overdue
	}
	return health
}

// OverdueDataDecayPolicy is the payload for the overdue list — the dashboard
// widget and contact-page review surface's data source.
type OverdueDataDecayPolicy struct {
	Policy         models.DataDecayPolicy `json:"policy"`
	Health         DataDecayHealth        `json:"health"`
	ContactID      uint                   `json:"contact_id"`
	ContactName    string                 `json:"contact_name"`
	PhotoThumbnail string                 `json:"photo_thumbnail,omitempty"`
}

// ListOverdueDataDecayPolicies returns every active data-decay policy of the
// user that is currently overdue, in order of most-overdue first, joined
// with the contact's numeric ID and display name (the frontend links to
// /contacts/<numeric id>). A paused (Active: false) policy never appears.
func ListOverdueDataDecayPolicies(db *gorm.DB, userID uint, now time.Time) ([]OverdueDataDecayPolicy, error) {
	var policies []models.DataDecayPolicy
	if err := db.Where("user_id = ? AND active = ?", userID, true).Find(&policies).Error; err != nil {
		return nil, err
	}
	if len(policies) == 0 {
		return nil, nil
	}

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

	var overdue []OverdueDataDecayPolicy
	for i := range policies {
		health := ComputeDataDecayHealth(&policies[i], now)
		if health.OverdueBy <= 0 {
			continue
		}
		c := contactByUID[policies[i].EntityID]
		overdue = append(overdue, OverdueDataDecayPolicy{
			Policy:         policies[i],
			Health:         health,
			ContactID:      c.ID,
			ContactName:    strings.TrimSpace(c.Firstname + " " + c.Lastname),
			PhotoThumbnail: c.PhotoThumbnail,
		})
	}

	slices.SortFunc(overdue, func(a, b OverdueDataDecayPolicy) int {
		return b.Health.OverdueBy - a.Health.OverdueBy
	})
	return overdue, nil
}
