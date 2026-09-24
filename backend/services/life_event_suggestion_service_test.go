package services

import (
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/contactgen"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func intPtr(v int) *int { return &v }

func yearPtr(v int) *contactmodel.PartialDate { return &contactmodel.PartialDate{Year: intPtr(v)} }

// contactWithPeriods builds a contact whose Card carries the given entries and
// whose envelope carries the given periods, the way the nested write path does
// (ApplyRecordToContact, so the Card/CRM columns are authoritative).
func contactWithPeriods(
	t *testing.T,
	card contactmodel.Card,
	periods []contactmodel.EntryPeriod,
) (*gorm.DB, models.User, models.Contact) {
	t.Helper()
	db := dbtest.New(t)
	user, err := contactgen.NewUser(db, "le-suggest")
	require.NoError(t, err)

	contact := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&contact, &contactmodel.Record{
		Card:     card,
		Envelope: contactmodel.CRMEnvelope{Periods: periods},
	}, "")
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

func suggestionFixture(t *testing.T) (*gorm.DB, models.User, models.Contact) {
	t.Helper()
	return contactWithPeriods(t,
		contactmodel.Card{
			Name:          &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Addresses:     []contactmodel.Address{{ID: "addr-1", Full: "1 Main St"}},
			Organizations: []contactmodel.Organization{{ID: "org-1", Name: "Acme"}},
		},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-1", Range: contactmodel.TemporalRange{
				Start: yearPtr(2019),
				End:   yearPtr(2024),
			}},
			{Kind: contactmodel.PeriodKindOrganization, EntryID: "org-1", Range: contactmodel.TemporalRange{
				Start: yearPtr(2020),
			}},
		},
	)
}

func suggestionsByType(t *testing.T, s []models.LifeEventSuggestion) map[string][]models.LifeEventSuggestion {
	t.Helper()
	byType := map[string][]models.LifeEventSuggestion{}
	for _, suggestion := range s {
		byType[suggestion.Type] = append(byType[suggestion.Type], suggestion)
	}
	return byType
}

func TestSuggestLifeEvents_InfersMovedJobChangeAndDeparture(t *testing.T) {
	db, _, contact := suggestionFixture(t)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 3)

	byType := suggestionsByType(t, suggestions)

	moved := byType[models.LifeEventTypeMoved]
	require.Len(t, moved, 1, "an address start must suggest a moved event")
	assert.Equal(t, models.LifeEventCategoryHomeLiving, moved[0].Category)
	require.NotNil(t, moved[0].Date)
	assert.Equal(t, 2019, *moved[0].Date.Year)
	require.NotNil(t, moved[0].EndDate, "the period's end must ride along as the event's end date")
	assert.Equal(t, 2024, *moved[0].EndDate.Year)
	assert.Equal(t, contactmodel.PeriodKindAddress, moved[0].SourceKind)
	assert.Equal(t, "addr-1", moved[0].SourceEntryID)

	// The address has an end and no successor, so it is also a departure. It
	// anchors on the end date; "moved" is the arrival.
	departure := byType[models.LifeEventTypeMovedOut]
	require.Len(t, departure, 1, "an address end with no successor must suggest a departure")
	require.NotNil(t, departure[0].Date)
	assert.Equal(t, 2024, *departure[0].Date.Year)
	assert.Nil(t, departure[0].EndDate)

	job := byType[models.LifeEventTypeJobChange]
	require.Len(t, job, 1, "an organization start must suggest a job_change event")
	assert.Equal(t, models.LifeEventCategoryWorkEducation, job[0].Category)
	assert.Equal(t, "org-1", job[0].SourceEntryID)
	assert.Nil(t, job[0].EndDate)
}

func TestSuggestLifeEvents_AddressEndWithoutSuccessorLeavesDeparture(t *testing.T) {
	db, _, contact := contactWithPeriods(t,
		contactmodel.Card{Addresses: []contactmodel.Address{{ID: "addr-1", Full: "1 Main St"}}},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-1", Range: contactmodel.TemporalRange{
				End: yearPtr(2024),
			}},
		},
	)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "an end-only address cannot anchor an arrival, only a departure")
	assert.Equal(t, models.LifeEventTypeMovedOut, suggestions[0].Type)
	require.NotNil(t, suggestions[0].Date)
	assert.Equal(t, 2024, *suggestions[0].Date.Year)
}

func TestSuggestLifeEvents_AddressEndWithSuccessorHasNoDeparture(t *testing.T) {
	db, _, contact := contactWithPeriods(t,
		contactmodel.Card{Addresses: []contactmodel.Address{
			{ID: "addr-1", Full: "1 Main St"},
			{ID: "addr-2", Full: "2 Oak Ave"},
		}},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-1", Range: contactmodel.TemporalRange{
				Start: yearPtr(2019), End: yearPtr(2024),
			}},
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-2", Range: contactmodel.TemporalRange{
				Start: yearPtr(2024),
			}},
		},
	)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)

	byType := suggestionsByType(t, suggestions)
	assert.Len(t, byType[models.LifeEventTypeMoved], 2, "each address start is an arrival")
	assert.Empty(t, byType[models.LifeEventTypeMovedOut],
		"the first address has a successor, so its end is a move, not a departure")
}

func TestSuggestLifeEvents_TitlePeriodSuggestsJobChange(t *testing.T) {
	db, _, contact := contactWithPeriods(t,
		contactmodel.Card{Titles: []contactmodel.Title{{ID: "title-1", Name: "Engineer", Kind: "title"}}},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindTitle, EntryID: "title-1", Range: contactmodel.TemporalRange{
				Start: yearPtr(2020),
			}},
		},
	)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.LifeEventTypeJobChange, suggestions[0].Type)
	assert.Equal(t, models.LifeEventCategoryWorkEducation, suggestions[0].Category)
	assert.Equal(t, contactmodel.PeriodKindTitle, suggestions[0].SourceKind)
	assert.Equal(t, "title-1", suggestions[0].SourceEntryID)
}

func TestSuggestLifeEvents_OrganizationAndTitleShareOneJobChange(t *testing.T) {
	db, _, contact := contactWithPeriods(t,
		contactmodel.Card{
			Organizations: []contactmodel.Organization{{ID: "org-1", Name: "Acme"}},
			Titles:        []contactmodel.Title{{ID: "title-1", Name: "Engineer", Kind: "title"}},
		},
		[]contactmodel.EntryPeriod{
			// Title first, to prove the organization wins regardless of order.
			{Kind: contactmodel.PeriodKindTitle, EntryID: "title-1", Range: contactmodel.TemporalRange{Start: yearPtr(2020)}},
			{Kind: contactmodel.PeriodKindOrganization, EntryID: "org-1", Range: contactmodel.TemporalRange{Start: yearPtr(2020)}},
		},
	)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "one job_change, attributed to the organization")
	assert.Equal(t, models.LifeEventTypeJobChange, suggestions[0].Type)
	assert.Equal(t, contactmodel.PeriodKindOrganization, suggestions[0].SourceKind)
	assert.Equal(t, "org-1", suggestions[0].SourceEntryID)
}

func TestSuggestLifeEvents_OrganizationEndDoesNotAnchor(t *testing.T) {
	db, _, contact := contactWithPeriods(t,
		contactmodel.Card{Organizations: []contactmodel.Organization{{ID: "org-1", Name: "Acme"}}},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindOrganization, EntryID: "org-1", Range: contactmodel.TemporalRange{
				End: yearPtr(2024),
			}},
		},
	)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	assert.Empty(t, suggestions, "an organization end has no inference rule")
}

func TestSuggestLifeEvents_SuppressedByExistingEvent(t *testing.T) {
	db, user, contact := suggestionFixture(t)
	require.NoError(t, db.Create(&models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMoved,
		Date: yearPtr(2019),
	}).Error)
	require.NoError(t, db.Create(&models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMovedOut,
		Date: yearPtr(2024),
	}).Error)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "already-recorded move and departure must not be re-offered")
	assert.Equal(t, models.LifeEventTypeJobChange, suggestions[0].Type)
}

func TestSuggestLifeEvents_SuppressedByResolution(t *testing.T) {
	db, user, contact := suggestionFixture(t)
	require.NoError(t, ResolveLifeEventSuggestion(db, user.ID, models.LifeEventSuggestionResolutionInput{
		EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindAddress,
		SourceEntryID: "addr-1", EventType: models.LifeEventTypeMoved,
		Resolution: models.LifeEventSuggestionDismissed,
	}))

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	byType := suggestionsByType(t, suggestions)
	assert.Empty(t, byType[models.LifeEventTypeMoved], "a dismissed candidate must never be offered again")
	assert.Len(t, byType[models.LifeEventTypeMovedOut], 1,
		"dismissing the arrival does not dismiss the departure (distinct event types)")
	assert.Len(t, byType[models.LifeEventTypeJobChange], 1)
}

func TestResolveLifeEventSuggestion_TitleSourceKind(t *testing.T) {
	db, user, contact := contactWithPeriods(t,
		contactmodel.Card{Titles: []contactmodel.Title{{ID: "title-1", Name: "Engineer", Kind: "title"}}},
		[]contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindTitle, EntryID: "title-1", Range: contactmodel.TemporalRange{Start: yearPtr(2020)}},
		},
	)

	require.NoError(t, ResolveLifeEventSuggestion(db, user.ID, models.LifeEventSuggestionResolutionInput{
		EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindTitle,
		SourceEntryID: "title-1", EventType: models.LifeEventTypeJobChange,
		Resolution: models.LifeEventSuggestionDismissed,
	}))

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	assert.Empty(t, suggestions, "a resolved title candidate must not be offered again")
}

func TestResolveLifeEventSuggestion_Idempotent(t *testing.T) {
	db, user, contact := suggestionFixture(t)

	in := models.LifeEventSuggestionResolutionInput{
		EntityID: contact.VCardUID, SourceKind: contactmodel.PeriodKindAddress,
		SourceEntryID: "addr-1", EventType: models.LifeEventTypeMoved,
		Resolution: models.LifeEventSuggestionAccepted,
	}
	require.NoError(t, ResolveLifeEventSuggestion(db, user.ID, in))
	require.NoError(t, ResolveLifeEventSuggestion(db, user.ID, in))

	var count int64
	require.NoError(t, db.Model(&models.LifeEventSuggestionResolution{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "resolving twice must not duplicate the memory row")
}

func TestSuggestLifeEvents_NilSafety(t *testing.T) {
	suggestions, err := SuggestLifeEvents(nil, nil)
	require.NoError(t, err)
	assert.Nil(t, suggestions)
	require.NoError(t, ResolveLifeEventSuggestion(nil, 0, models.LifeEventSuggestionResolutionInput{}))
}
