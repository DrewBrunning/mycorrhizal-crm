package services

import (
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// This file is the direct services-package coverage for occasion_service.go
// (ADR 0024, issue #387): every other "occasion" test in this repo lives in
// controllers/ (occasion_real_db_test.go etc), exercising this file only
// indirectly through the HTTP handler. Go's default per-package coverage
// mode doesn't attribute that to this package, and every other file in
// services/ has its own direct _test.go sibling (birthday_service.go,
// cadence_service.go, ...) -- this closes that gap the same way, using
// dbtest.New(t) (CLAUDE.md backend trap #1) rather than the older
// setupRouter(t)/AutoMigrate helper some sibling files in this package still
// use.

func occasionTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

func TestGetUpcomingOccasions_AllFourSources(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "occ-sources", Password: "password123!A", Email: "occ-sources@example.com"}
	require.NoError(t, db.Create(&user).Error)

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 5)

	bdayContact := models.Contact{UserID: user.ID, Firstname: "Bea", Birthday: soon.Format("2006-01-02")}
	require.NoError(t, db.Create(&bdayContact).Error)

	annivContact := models.Contact{UserID: user.ID, Firstname: "Ann", Anniversary: soon.Format("2006-01-02")}
	require.NoError(t, db.Create(&annivContact).Error)

	eventContact := models.Contact{UserID: user.ID, Firstname: "Lee"}
	require.NoError(t, db.Create(&eventContact).Error)
	event := models.LifeEvent{
		UserID: user.ID, EntityID: eventContact.VCardUID, Type: "graduation", Remind: true,
		Date: &contactmodel.PartialDate{Month: intPtr(int(soon.Month())), Day: intPtr(soon.Day())},
	}
	require.NoError(t, db.Create(&event).Error)

	obContact := models.Contact{UserID: user.ID, Firstname: "Obi"}
	require.NoError(t, db.Create(&obContact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: obContact.VCardUID, Kind: "card", Label: "Holiday card",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	occasions, err := GetUpcomingOccasions(db, user.ID, now, 30, false)
	require.NoError(t, err)

	bySource := map[string]models.UpcomingOccasion{}
	for _, o := range occasions {
		bySource[o.Source] = o
	}
	require.Contains(t, bySource, "birthday")
	assert.Equal(t, "Bea", bySource["birthday"].ContactName)
	require.Contains(t, bySource, "anniversary")
	assert.Equal(t, "Ann", bySource["anniversary"].ContactName)
	require.Contains(t, bySource, "life_event")
	assert.Equal(t, "Lee", bySource["life_event"].ContactName)
	assert.Equal(t, "graduation", bySource["life_event"].Label)
	require.Contains(t, bySource, "obligation")
	assert.Equal(t, "Obi", bySource["obligation"].ContactName)
	assert.Equal(t, "card", bySource["obligation"].Kind)

	// Sorted ascending by days-until -- all four land on the same day here,
	// so this also exercises the ContactID tiebreaker branch.
	for i := 1; i < len(occasions); i++ {
		assert.LessOrEqual(t, occasions[i-1].DaysUntil, occasions[i].DaysUntil)
	}
}

func TestGetUpcomingOccasions_ExcludesBeyondWindow(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "occ-window")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	inWindow := models.Contact{UserID: user.ID, Firstname: "In", Birthday: now.AddDate(0, 0, 10).Format("2006-01-02")}
	require.NoError(t, db.Create(&inWindow).Error)
	outOfWindow := models.Contact{UserID: user.ID, Firstname: "Out", Birthday: now.AddDate(0, 0, 40).Format("2006-01-02")}
	require.NoError(t, db.Create(&outOfWindow).Error)

	occasions, err := GetUpcomingOccasions(db, user.ID, now, 30, false)
	require.NoError(t, err)
	require.Len(t, occasions, 1)
	assert.Equal(t, "In", occasions[0].ContactName)
}

func TestGetUpcomingOccasions_SensitivityFiltering(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "occ-sensitivity")
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 3)

	contact := models.Contact{UserID: user.ID, Firstname: "Sec"}
	require.NoError(t, db.Create(&contact).Error)
	secret := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: "card", Label: "Secret card",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivitySecret,
	}
	require.NoError(t, db.Create(&secret).Error)

	excluded, err := GetUpcomingOccasions(db, user.ID, now, 30, false)
	require.NoError(t, err)
	assert.Empty(t, excluded)

	included, err := GetUpcomingOccasions(db, user.ID, now, 30, true)
	require.NoError(t, err)
	require.Len(t, included, 1)
	assert.Equal(t, "obligation", included[0].Source)
}

func TestGetUpcomingOccasions_LifeEventWithoutRemindOrDateIsExcluded(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "occ-noremind")
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 3)

	contact := models.Contact{UserID: user.ID, Firstname: "NoRemind"}
	require.NoError(t, db.Create(&contact).Error)

	// Remind=false: excluded by the query itself.
	noRemind := models.LifeEvent{
		UserID: user.ID, EntityID: contact.VCardUID, Type: "milestone", Remind: false,
		Date: &contactmodel.PartialDate{Month: intPtr(int(soon.Month())), Day: intPtr(soon.Day())},
	}
	require.NoError(t, db.Create(&noRemind).Error)

	// Remind=true but no Date at all: excluded by lifeEventHasMonthDay's nil check.
	noDate := models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "milestone2", Remind: true}
	require.NoError(t, db.Create(&noDate).Error)

	occasions, err := GetUpcomingOccasions(db, user.ID, now, 30, false)
	require.NoError(t, err)
	assert.Empty(t, occasions)
}

func TestGetUpcomingOccasions_UnknownContactEntityIsExcluded(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "occ-unknown")
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 3)

	// Obligation referencing an EntityID with no matching Contact row --
	// contactsByVCardUID's batch load won't find it, so the "found" guard
	// in GetUpcomingOccasions must skip it rather than panic.
	orphan := models.OccasionObligation{
		UserID: user.ID, EntityID: "no-such-vcard-uid", Kind: "card", Label: "Orphan",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&orphan).Error)

	occasions, err := GetUpcomingOccasions(db, user.ID, now, 30, false)
	require.NoError(t, err)
	assert.Empty(t, occasions)
}

func TestGetGiftShoppingList_NoObligationsReturnsEmptySlice(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "gift-empty")

	items, err := GetGiftShoppingList(db, user.ID, time.Now(), 30, false)
	require.NoError(t, err)
	assert.NotNil(t, items)
	assert.Empty(t, items)
}

func TestGetGiftShoppingList_NeededWhenNoMatchingGift(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "gift-needed")
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 5)

	contact := models.Contact{UserID: user.ID, Firstname: "Needy"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: models.OccasionObligationKindGift, Label: "Birthday gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	items, err := GetGiftShoppingList(db, user.ID, now, 30, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "needed", items[0].Status)
	assert.Empty(t, items[0].LinkedGiftID)
}

func TestGetGiftShoppingList_MatchesViaLinkedLifeEvent(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "gift-linked")
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 5)

	contact := models.Contact{UserID: user.ID, Firstname: "Linked"}
	require.NoError(t, db.Create(&contact).Error)
	event := models.LifeEvent{UserID: user.ID, EntityID: contact.VCardUID, Type: "graduation"}
	require.NoError(t, db.Create(&event).Error)

	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: models.OccasionObligationKindGift, Label: "Grad gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal, LinkedLifeEventID: event.ID,
	}
	require.NoError(t, db.Create(&obligation).Error)

	// Dated far outside the 330-day window -- only the LinkedLifeEventID
	// match should find it, proving that branch runs independently of the
	// date-window fallback.
	oldDate := now.AddDate(-3, 0, 0)
	gift := models.Gift{
		UserID: user.ID, EntityID: contact.VCardUID, Status: "purchased",
		LifeEventID: event.ID, Date: &oldDate,
	}
	require.NoError(t, db.Create(&gift).Error)

	items, err := GetGiftShoppingList(db, user.ID, now, 30, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "purchased", items[0].Status)
	assert.Equal(t, gift.ID, items[0].LinkedGiftID)
}

func TestGetGiftShoppingList_MatchesViaDateWindowFallback(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "gift-datewindow")
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 5)

	contact := models.Contact{UserID: user.ID, Firstname: "Windowed"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: models.OccasionObligationKindGift, Label: "This cycle's gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&obligation).Error)

	// No LinkedLifeEventID on either side; recent enough (inside
	// giftMatchWindowDays) to match via the date-proximity fallback.
	recent := now.AddDate(0, -2, 0)
	gift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Status: "idea", Date: &recent}
	require.NoError(t, db.Create(&gift).Error)

	// A second, older gift outside the window must NOT win over the recent one.
	tooOld := now.AddDate(-2, 0, 0)
	oldGift := models.Gift{UserID: user.ID, EntityID: contact.VCardUID, Status: "given", Date: &tooOld}
	require.NoError(t, db.Create(&oldGift).Error)

	items, err := GetGiftShoppingList(db, user.ID, now, 30, false)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, "idea", items[0].Status)
	assert.Equal(t, gift.ID, items[0].LinkedGiftID)
}

func TestGetGiftShoppingList_SensitivityFiltering(t *testing.T) {
	db := dbtest.New(t)
	user := occasionTestUser(t, db, "gift-sensitivity")
	now := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	soon := now.AddDate(0, 0, 5)

	contact := models.Contact{UserID: user.ID, Firstname: "Priv"}
	require.NoError(t, db.Create(&contact).Error)
	obligation := models.OccasionObligation{
		UserID: user.ID, EntityID: contact.VCardUID, Kind: models.OccasionObligationKindGift, Label: "Private gift",
		AnchorMonth: intPtr(int(soon.Month())), AnchorDay: intPtr(soon.Day()), Active: true,
		Sensitivity: models.RelationshipSensitivityPrivate,
	}
	require.NoError(t, db.Create(&obligation).Error)

	excluded, err := GetGiftShoppingList(db, user.ID, now, 30, false)
	require.NoError(t, err)
	assert.Empty(t, excluded)

	included, err := GetGiftShoppingList(db, user.ID, now, 30, true)
	require.NoError(t, err)
	assert.Len(t, included, 1)
}

func TestResolveAnnualOccurrence_InvalidMonthDay(t *testing.T) {
	_, _, ok := resolveAnnualOccurrence(13, 40, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	assert.False(t, ok)
}

func TestOccasionDateMonthDay_TooShortIsInvalid(t *testing.T) {
	_, _, ok := occasionDateMonthDay("2026")
	assert.False(t, ok)
}
