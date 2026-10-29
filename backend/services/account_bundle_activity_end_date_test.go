package services

import (
	"encoding/json"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #2006: end_date travels in the account bundle (omitted for a
// single-day activity) and lands on the imported activity; an end that
// precedes the start, or that cannot be parsed, is reported rather than
// silently stored.
func TestAccountBundle_ActivityEndDateRoundTripAndOmission(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "end-owner")
	target := bundleTestUser(t, targetDB, "end-target")
	useBundlePhotoDir(t)

	ada := models.Contact{UserID: owner.ID, Firstname: "Ada"}
	require.NoError(t, ownerDB.Create(&ada).Error)
	end := time.Date(2026, 5, 15, 0, 0, 0, 0, time.UTC)
	trip := models.Activity{UserID: owner.ID, Title: "Trip", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC), EndDate: &end}
	lunch := models.Activity{UserID: owner.ID, Title: "Lunch", Date: time.Date(2026, 5, 12, 0, 0, 0, 0, time.UTC)}
	require.NoError(t, ownerDB.Create(&trip).Error)
	require.NoError(t, ownerDB.Create(&lunch).Error)
	require.NoError(t, ownerDB.Model(&trip).Association("Contacts").Replace(&[]models.Contact{ada}))
	require.NoError(t, ownerDB.Model(&lunch).Association("Contacts").Replace(&[]models.Contact{ada}))

	bundle, _, err := BuildAccountBundle(ownerDB, owner.ID, t.TempDir())
	require.NoError(t, err)
	raw, err := json.Marshal(bundle.Plan.Activities)
	require.NoError(t, err)
	var wire []map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	require.Len(t, wire, 2)
	for _, a := range wire {
		if a["title"] == "Trip" {
			assert.Equal(t, "2026-05-15T00:00:00Z", a["end_date"])
		} else {
			assert.NotContains(t, a, "end_date", "a single-day activity omits the key")
		}
	}

	report, _, err := ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues)
	var gotTrip, gotLunch models.Activity
	require.NoError(t, targetDB.Where("user_id = ? AND title = ?", target.ID, "Trip").First(&gotTrip).Error)
	require.NoError(t, targetDB.Where("user_id = ? AND title = ?", target.ID, "Lunch").First(&gotLunch).Error)
	require.NotNil(t, gotTrip.EndDate)
	assert.Equal(t, end, gotTrip.EndDate.UTC())
	assert.Nil(t, gotLunch.EndDate)
}

func TestAccountBundle_ImportReportsBadActivityEndDate(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "test"}
	plan.Contacts = []MappedContact{{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")}}
	plan.Activities = []MappedActivity{
		{Ref: ref("activity/1"), UUID: "a1", Contacts: []SourceRef{ref("contact/1")},
			Title: "Bad range", Date: "2026-05-12T00:00:00Z", EndDate: "2026-05-01T00:00:00Z"},
		{Ref: ref("activity/2"), UUID: "a2", Contacts: []SourceRef{ref("contact/1")},
			Title: "Unparseable", Date: "2026-05-12T00:00:00Z", EndDate: "soon"},
	}
	report, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	var fields []string
	for _, i := range report.Issues {
		fields = append(fields, i.Field)
	}
	assert.Equal(t, []string{"activity.end_date", "activity.end_date"}, fields)
	var acts []models.Activity
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&acts).Error)
	require.Len(t, acts, 2, "the activities themselves still import")
	for _, a := range acts {
		assert.Nil(t, a.EndDate)
	}
}
