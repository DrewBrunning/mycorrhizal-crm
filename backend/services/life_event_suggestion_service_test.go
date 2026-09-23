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

// suggestionFixture builds a contact whose card carries an address and an
// employer, each with a dated period, the way the nested write path does
// (ApplyRecordToContact, so the Card/CRM columns are authoritative).
func suggestionFixture(t *testing.T) (*gorm.DB, models.User, models.Contact) {
	t.Helper()
	db := dbtest.New(t)
	user, err := contactgen.NewUser(db, "le-suggest")
	require.NoError(t, err)

	contact := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&contact, &contactmodel.Record{
		Card: contactmodel.Card{
			Name:          &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Ada"}}},
			Addresses:     []contactmodel.Address{{ID: "addr-1", Full: "1 Main St"}},
			Organizations: []contactmodel.Organization{{ID: "org-1", Name: "Acme"}},
		},
		Envelope: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-1", Range: contactmodel.TemporalRange{
				Start: &contactmodel.PartialDate{Year: intPtr(2019)},
				End:   &contactmodel.PartialDate{Year: intPtr(2024)},
			}},
			{Kind: contactmodel.PeriodKindOrganization, EntryID: "org-1", Range: contactmodel.TemporalRange{
				Start: &contactmodel.PartialDate{Year: intPtr(2020)},
			}},
		}},
	}, "")
	require.NoError(t, db.Create(&contact).Error)
	return db, user, contact
}

func TestSuggestLifeEvents_InfersMovedAndJobChange(t *testing.T) {
	db, _, contact := suggestionFixture(t)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 2)

	byType := map[string]models.LifeEventSuggestion{}
	for _, s := range suggestions {
		byType[s.Type] = s
	}

	moved, ok := byType[models.LifeEventTypeMoved]
	require.True(t, ok, "an address start must suggest a moved event")
	assert.Equal(t, models.LifeEventCategoryHomeLiving, moved.Category)
	require.NotNil(t, moved.Date)
	assert.Equal(t, 2019, *moved.Date.Year)
	require.NotNil(t, moved.EndDate, "the period's end must ride along as the event's end date")
	assert.Equal(t, 2024, *moved.EndDate.Year)
	assert.Equal(t, contactmodel.PeriodKindAddress, moved.SourceKind)
	assert.Equal(t, "addr-1", moved.SourceEntryID)

	job, ok := byType[models.LifeEventTypeJobChange]
	require.True(t, ok, "an organization start must suggest a job_change event")
	assert.Equal(t, models.LifeEventCategoryWorkEducation, job.Category)
	assert.Equal(t, "org-1", job.SourceEntryID)
	assert.Nil(t, job.EndDate)
}

func TestSuggestLifeEvents_RequiresAStartDate(t *testing.T) {
	db := dbtest.New(t)
	user, err := contactgen.NewUser(db, "le-suggest-nostart")
	require.NoError(t, err)

	contact := models.Contact{UserID: user.ID}
	models.ApplyRecordToContact(&contact, &contactmodel.Record{
		Card: contactmodel.Card{
			Name:      &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: "Bo"}}},
			Addresses: []contactmodel.Address{{ID: "addr-1", Full: "1 Main St"}},
		},
		Envelope: contactmodel.CRMEnvelope{Periods: []contactmodel.EntryPeriod{
			{Kind: contactmodel.PeriodKindAddress, EntryID: "addr-1", Range: contactmodel.TemporalRange{
				End: &contactmodel.PartialDate{Year: intPtr(2024)},
			}},
		}},
	}, "")
	require.NoError(t, db.Create(&contact).Error)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	assert.Empty(t, suggestions, "an end-only period cannot anchor an event")
}

func TestSuggestLifeEvents_SuppressedByExistingEvent(t *testing.T) {
	db, user, contact := suggestionFixture(t)
	require.NoError(t, db.Create(&models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: models.LifeEventTypeMoved,
		Date: &contactmodel.PartialDate{Year: intPtr(2019)},
	}).Error)

	suggestions, err := SuggestLifeEvents(db, &contact)
	require.NoError(t, err)
	require.Len(t, suggestions, 1, "the already-recorded move must not be re-offered")
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
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.LifeEventTypeJobChange, suggestions[0].Type, "a dismissed candidate must never be offered again")
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
