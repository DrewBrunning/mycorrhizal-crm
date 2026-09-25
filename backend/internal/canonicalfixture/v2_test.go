package canonicalfixture

import (
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fixedNow is the reference instant the v1.2.0 relative-timing tests pin, so
// days_ago/updated_days_ago resolve deterministically.
var fixedNow = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// populatedAt loads the manifest and populates it against a pinned clock.
func populatedAt(t *testing.T) (*Manifest, *Dataset, *gorm.DB) {
	t.Helper()
	m := readManifest(t)
	db := dbtest.New(t)
	ds, err := PopulateAt(db, m, fixedNow)
	require.NoError(t, err)
	return m, ds, db
}

// TestSelfContactPopulates pins the T90 pointer the health score's Closeness
// facet depends on (ADR 0023) — without it every score stays at the neutral
// default.
func TestSelfContactPopulates(t *testing.T) {
	_, ds, db := populatedAt(t)
	require.NotEmpty(t, ds.Contacts["me"].VCardUID)

	var user models.User
	require.NoError(t, db.First(&user, ds.User.ID).Error)
	require.NotNil(t, user.SelfContactVCardUID, "self_contact must set users.self_contact_vcard_uid")
	assert.Equal(t, ds.Contacts["me"].VCardUID, *user.SelfContactVCardUID)
}

// TestDemoRelativeTimingResolves pins the injected-clock convention: activities
// with days_ago and contacts with updated_days_ago land at now-N days rather
// than at the zero value, so the demo dataset's health spread stays meaningful
// as wall-clock time advances.
func TestDemoRelativeTimingResolves(t *testing.T) {
	_, ds, db := populatedAt(t)

	var dinner models.Activity
	require.NoError(t, db.Where("title = ?", "Dinner with Nadia").First(&dinner).Error)
	assert.WithinDuration(t, fixedNow.AddDate(0, 0, -3), dinner.Date, time.Second, "activity days_ago must resolve against the pinned clock")

	var nadia models.Contact
	require.NoError(t, db.First(&nadia, ds.Contacts["nadia"].ID).Error)
	assert.WithinDuration(t, fixedNow.AddDate(0, 0, -2), nadia.UpdatedAt, time.Second, "contact updated_days_ago must back-date updated_at")

	var bea models.Contact
	require.NoError(t, db.First(&bea, ds.Contacts["bea"].ID).Error)
	assert.WithinDuration(t, fixedNow.AddDate(0, 0, -200), bea.UpdatedAt, time.Second)
}

// TestFavoritesPopulate pins the CRM-local favorite flag (issue #173) the
// dashboard's Favorites block reads.
func TestFavoritesPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)

	var favorites int64
	require.NoError(t, db.Model(&models.Contact{}).
		Where("user_id = ? AND is_favorite = ?", ds.User.ID, true).Count(&favorites).Error)
	assert.EqualValues(t, 2, favorites, "nadia and theo are the fixture's favorites")

	var nadia models.Contact
	require.NoError(t, db.First(&nadia, ds.Contacts["nadia"].ID).Error)
	assert.True(t, nadia.IsFavorite)

	var bea models.Contact
	require.NoError(t, db.First(&bea, ds.Contacts["bea"].ID).Error)
	assert.False(t, bea.IsFavorite)
}

// TestCadencePoliciesPopulate pins the cadence-policy section, the target
// interval the health score's Recency/Frequency facets read.
func TestCadencePoliciesPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)
	require.Len(t, ds.CadencePolicies, 4)

	var nadia models.CadencePolicy
	require.NoError(t, db.Where("entity_id = ?", ds.Contacts["nadia"].VCardUID).First(&nadia).Error)
	assert.Equal(t, 21, nadia.TargetIntervalDays)
	assert.Equal(t, ds.User.ID, nadia.UserID)

	var count int64
	require.NoError(t, db.Model(&models.CadencePolicy{}).Where("user_id = ?", ds.User.ID).Count(&count).Error)
	assert.Equal(t, int64(4), count)
}

// TestReachOutSuggestionsPopulate pins the pending/dismissed status the
// health score's ReachOut facet and the dashboard block read.
func TestReachOutSuggestionsPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)
	require.Len(t, ds.ReachOutSuggestions, 2)

	var bjork models.ReachOutSuggestion
	require.NoError(t, db.Where("contact_vcard_uid = ?", ds.Contacts["bjork"].VCardUID).First(&bjork).Error)
	assert.Equal(t, models.ReachOutStatusPending, bjork.Status)
	assert.Equal(t, models.ReachOutKindAddress, bjork.Kind)

	var marcus models.ReachOutSuggestion
	require.NoError(t, db.Where("contact_vcard_uid = ?", ds.Contacts["marcus"].VCardUID).First(&marcus).Error)
	assert.Equal(t, models.ReachOutStatusDismissed, marcus.Status)
}

// TestOccasionsPopulate pins the ADR 0024/0026 sections: obligations with
// anchors/lead time/active flag, and events with a full RSVP ledger.
func TestOccasionsPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)
	require.Len(t, ds.OccasionObligations, 3)
	require.Len(t, ds.OccasionEvents, 2)

	var gift models.OccasionObligation
	require.NoError(t, db.Where("label = ?", "Birthday gift for Nadia").First(&gift).Error)
	require.NotNil(t, gift.AnchorMonth)
	require.NotNil(t, gift.AnchorDay)
	assert.Equal(t, 9, *gift.AnchorMonth)
	assert.Equal(t, 14, *gift.AnchorDay)
	assert.Equal(t, 14, gift.LeadTimeDays)
	assert.True(t, gift.Active)

	var retired models.OccasionObligation
	require.NoError(t, db.Where("label = ?", "Annual garden party invite").First(&retired).Error)
	assert.False(t, retired.Active, "an explicitly inactive obligation must stay inactive")

	var party models.OccasionEvent
	require.NoError(t, db.Where("title = ?", "Summer garden party").First(&party).Error)
	assert.WithinDuration(t, fixedNow.AddDate(0, 0, 21), party.StartsAt, time.Second)

	var attendees []models.OccasionEventAttendee
	require.NoError(t, db.Where("event_id = ?", party.ID).Find(&attendees).Error)
	require.Len(t, attendees, 4)
	rsvps := map[string]bool{}
	for _, a := range attendees {
		rsvps[a.RSVP] = true
	}
	assert.True(t, rsvps[models.OccasionEventRSVPAccepted])
	assert.True(t, rsvps[models.OccasionEventRSVPPending])
	assert.True(t, rsvps[models.OccasionEventRSVPMaybe])

	var declined int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).
		Joins("JOIN occasion_events ON occasion_events.id = occasion_event_attendees.event_id").
		Where("occasion_events.title = ? AND occasion_event_attendees.rsvp = ?", "Book club dinner", models.OccasionEventRSVPDeclined).
		Count(&declined).Error)
	assert.Equal(t, int64(1), declined)
}

// TestPreferenceLevelAndFieldPositionPopulate pins the two additive v1.2.0
// columns the fixture now exercises (issues #246 and #1210).
func TestPreferenceLevelAndFieldPositionPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)

	var level string
	require.NoError(t, db.Model(&models.Preference{}).
		Where("entity_id = ? AND category = ? AND value = ?", ds.Contacts["nadia"].VCardUID, "hobby", "pottery").
		Pluck("level", &level).Error)
	assert.Equal(t, "high", level)

	var positions []int
	require.NoError(t, db.Model(&models.FieldDefinition{}).
		Where("user_id = ?", ds.User.ID).Order("position").Pluck("position", &positions).Error)
	assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6}, positions, "custom field positions must populate as a contiguous deliberate order")
}

// TestLifeEventEndDatePopulates pins the ADR 0025 span (migration 000058).
func TestLifeEventEndDatePopulates(t *testing.T) {
	_, ds, db := populatedAt(t)

	var event models.LifeEvent
	require.NoError(t, db.Where("entity_id = ? AND type = ?", ds.Contacts["nadia"].VCardUID, "job_change").First(&event).Error)
	require.NotNil(t, event.EndDate)
	require.NotNil(t, event.EndDate.Year)
	assert.Equal(t, 2021, *event.EndDate.Year)
}

// TestDeceasedContactsPopulate pins issue #1193/#1238: death is a
// Card.Anniversaries[kind=death], and IsDeceased() is derived from it.
func TestDeceasedContactsPopulate(t *testing.T) {
	_, ds, db := populatedAt(t)

	for _, name := range []string{"margaret", "harold"} {
		var c models.Contact
		require.NoError(t, db.First(&c, ds.Contacts[name].ID).Error)
		assert.True(t, c.Card.IsDeceased(), "%s must read as deceased", name)
	}

	var ada models.Contact
	require.NoError(t, db.First(&ada, ds.Contacts["ada"].ID).Error)
	assert.False(t, ada.Card.IsDeceased(), "a live contact must not read as deceased")

	// The recent death carries its date; the year-only death stays partial.
	margaret := ds.Contacts["margaret"].Card.DeathAnniversary()
	require.NotNil(t, margaret)
	require.NotNil(t, margaret.Date.Partial)
	assert.Equal(t, 2024, *margaret.Date.Partial.Year)
}

// TestSoftDeleteCascadeCoversV2Entities pins the cascade extension: a
// soft-deleted contact's cadence policy, occasion obligation and reach-out
// suggestions must be removed exactly the way DeleteContact removes them.
func TestSoftDeleteCascadeCoversV2Entities(t *testing.T) {
	db := dbtest.New(t)
	base := readManifest(t)

	// Add a soft-deleted persona carrying one row of each new contact-scoped
	// entity, so the cascade has something to sweep.
	m := *base
	m.Contacts = append([]ContactEntry{{
		Name:        "gone",
		SoftDeleted: true,
		Card:        base.Contacts[0].Card,
		CRM:         base.Contacts[0].CRM,
	}}, base.Contacts...)
	// Re-point the card uid so it is distinct.
	m.Contacts[0].Card.UID = "10000000-0000-4000-8000-00000000dead"
	m.CadencePolicies = append(m.CadencePolicies, CadencePolicyEntry{Contact: "gone", TargetIntervalDays: 30})
	m.ReachOutSuggestions = append(m.ReachOutSuggestions, ReachOutSuggestionEntry{Contact: "gone", Kind: "title", Status: "pending"})
	m.OccasionObligations = append(m.OccasionObligations, OccasionObligationEntry{Contact: "gone", Kind: "card", Label: "Gone card", Active: boolPtr(true)})
	m.OccasionEvents = append(m.OccasionEvents, OccasionEventEntry{
		Title:        "Gone's farewell",
		StartsInDays: intPtr(3),
		Attendees:    []OccasionEventAttendeeEntry{{Contact: "gone", RSVP: "accepted"}, {Contact: "nadia", RSVP: "accepted"}},
	})

	ds, err := PopulateAt(db, &m, fixedNow)
	require.NoError(t, err)
	gone := ds.Contacts["gone"]
	require.True(t, gone.DeletedAt.Valid)
	uid := gone.VCardUID

	// Cadence policy and occasion obligation are user-authored content, so the
	// cascade soft-deletes them (the undo button) — the live count is zero, the
	// tombstoned row survives. Reach-out suggestions are system-generated and
	// edge-shaped, so they are hard-deleted even under Unscoped.
	var liveCadences, liveObligations, allReachOuts int64
	db.Model(&models.CadencePolicy{}).Where("entity_id = ?", uid).Count(&liveCadences)
	db.Model(&models.OccasionObligation{}).Where("entity_id = ?", uid).Count(&liveObligations)
	db.Unscoped().Model(&models.ReachOutSuggestion{}).Where("contact_vcard_uid = ?", uid).Count(&allReachOuts)
	assert.Zero(t, liveCadences, "a tombstoned contact's cadence policy must be soft-swept")
	assert.Zero(t, liveObligations, "a tombstoned contact's occasion obligations must be soft-swept")
	assert.Zero(t, allReachOuts, "a tombstoned contact's reach-out suggestions must be hard-deleted")

	var sweptCadences, sweptObligations int64
	db.Unscoped().Model(&models.CadencePolicy{}).Where("entity_id = ? AND deleted_at IS NOT NULL", uid).Count(&sweptCadences)
	db.Unscoped().Model(&models.OccasionObligation{}).Where("entity_id = ? AND deleted_at IS NOT NULL", uid).Count(&sweptObligations)
	assert.Equal(t, int64(1), sweptCadences, "the cadence row must be tombstoned, not never-created")
	assert.Equal(t, int64(1), sweptObligations, "the obligation row must be tombstoned, not never-created")

	// The attendee row is join-shaped (hard delete); the event itself and the
	// other invitee survive.
	var goneAttendees, farewellAttendees int64
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).Where("entity_id = ?", uid).Count(&goneAttendees).Error)
	require.NoError(t, db.Model(&models.OccasionEventAttendee{}).
		Joins("JOIN occasion_events ON occasion_events.id = occasion_event_attendees.event_id").
		Where("occasion_events.title = ?", "Gone's farewell").Count(&farewellAttendees).Error)
	assert.Zero(t, goneAttendees, "a tombstoned contact's RSVP rows must be hard-deleted")
	assert.Equal(t, int64(1), farewellAttendees, "the other invitee's RSVP must survive")
}

// TestSoftDeletedSelfContactClearsPointer pins that tombstoning the
// self-contact clears users.self_contact_vcard_uid, the way DeleteContact
// does, instead of leaving it dangling on a deleted row.
func TestSoftDeletedSelfContactClearsPointer(t *testing.T) {
	db := dbtest.New(t)
	m := *readManifest(t)
	m.Contacts = append([]ContactEntry(nil), m.Contacts...)
	for i := range m.Contacts {
		if m.Contacts[i].Name == m.SelfContact {
			m.Contacts[i].SoftDeleted = true
		}
	}

	ds, err := PopulateAt(db, &m, fixedNow)
	require.NoError(t, err)

	var user models.User
	require.NoError(t, db.First(&user, ds.User.ID).Error)
	assert.Nil(t, user.SelfContactVCardUID, "a tombstoned self-contact must not stay the Me pointer")
}

// TestV2SectionDefaultsAndTombstones covers the loader paths the committed
// manifest does not exercise: omitted reach-out status and RSVP (default
// pending), absolute occasion-event instants, an explicit-sensitivity-free
// event, and a soft_deleted row in each new soft-delete section.
func TestV2SectionDefaultsAndTombstones(t *testing.T) {
	db := dbtest.New(t)
	m := *readManifest(t)
	starts := time.Date(2026, 12, 31, 20, 0, 0, 0, time.UTC)
	ends := starts.Add(4 * time.Hour)

	// A soft-deleted cadence row on a contact with no live policy: the partial
	// unique index only covers live rows, but keep the test independent of it.
	m.CadencePolicies = append(append([]CadencePolicyEntry(nil), m.CadencePolicies...),
		CadencePolicyEntry{Contact: "marcus", TargetIntervalDays: 7, SoftDeleted: true})
	m.ReachOutSuggestions = append(append([]ReachOutSuggestionEntry(nil), m.ReachOutSuggestions...),
		ReachOutSuggestionEntry{Contact: "soren", Kind: "title", OldValue: "Engineer", NewValue: "Staff engineer"})
	m.OccasionObligations = append(append([]OccasionObligationEntry(nil), m.OccasionObligations...),
		OccasionObligationEntry{Contact: "theo", Kind: "card", Label: "Retired anniversary card", SoftDeleted: true})
	m.OccasionEvents = append(append([]OccasionEventEntry(nil), m.OccasionEvents...),
		OccasionEventEntry{
			Title:     "New Year's Eve",
			StartsAt:  &starts,
			EndsAt:    &ends,
			Attendees: []OccasionEventAttendeeEntry{{Contact: "theo"}},
		},
		OccasionEventEntry{Title: "Cancelled picnic", StartsInDays: intPtr(5), SoftDeleted: true},
	)

	ds, err := PopulateAt(db, &m, fixedNow)
	require.NoError(t, err)

	var suggestion models.ReachOutSuggestion
	require.NoError(t, db.Where("contact_vcard_uid = ?", ds.Contacts["soren"].VCardUID).First(&suggestion).Error)
	assert.Equal(t, models.ReachOutStatusPending, suggestion.Status, "an omitted status defaults to pending")

	var nye models.OccasionEvent
	require.NoError(t, db.Where("title = ?", "New Year's Eve").First(&nye).Error)
	assert.True(t, starts.Equal(nye.StartsAt), "an absolute starts_at is used verbatim, not resolved against the clock")
	require.NotNil(t, nye.EndsAt)
	assert.True(t, ends.Equal(*nye.EndsAt))
	var attendee models.OccasionEventAttendee
	require.NoError(t, db.Where("event_id = ?", nye.ID).First(&attendee).Error)
	assert.Equal(t, models.OccasionEventRSVPPending, attendee.RSVP, "an omitted rsvp defaults to pending")

	var liveCadence, deadCadence int64
	require.NoError(t, db.Model(&models.CadencePolicy{}).Where("entity_id = ?", ds.Contacts["marcus"].VCardUID).Count(&liveCadence).Error)
	require.NoError(t, db.Unscoped().Model(&models.CadencePolicy{}).
		Where("entity_id = ? AND deleted_at IS NOT NULL", ds.Contacts["marcus"].VCardUID).Count(&deadCadence).Error)
	assert.Zero(t, liveCadence)
	assert.Equal(t, int64(1), deadCadence, "a soft_deleted cadence policy is tombstoned, not skipped")

	var liveObligation, deadObligation int64
	require.NoError(t, db.Model(&models.OccasionObligation{}).Where("label = ?", "Retired anniversary card").Count(&liveObligation).Error)
	require.NoError(t, db.Unscoped().Model(&models.OccasionObligation{}).
		Where("label = ? AND deleted_at IS NOT NULL", "Retired anniversary card").Count(&deadObligation).Error)
	assert.Zero(t, liveObligation)
	assert.Equal(t, int64(1), deadObligation)

	var liveEvent, deadEvent int64
	require.NoError(t, db.Model(&models.OccasionEvent{}).Where("title = ?", "Cancelled picnic").Count(&liveEvent).Error)
	require.NoError(t, db.Unscoped().Model(&models.OccasionEvent{}).
		Where("title = ? AND deleted_at IS NOT NULL", "Cancelled picnic").Count(&deadEvent).Error)
	assert.Zero(t, liveEvent)
	assert.Equal(t, int64(1), deadEvent)
}

func boolPtr(v bool) *bool { return &v }
