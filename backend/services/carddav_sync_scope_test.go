package services

import (
	"sort"
	"testing"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type scopeFixture struct {
	db    *gorm.DB
	owner models.User
	other models.User
	a     models.Circle
	b     models.Circle
	c1    models.Contact
	c2    models.Contact
	c3    models.Contact
}

func newScopeFixture(t *testing.T) *scopeFixture {
	t.Helper()
	db := dbtest.New(t)
	owner := models.User{Username: "scope-svc-owner", Password: "password123!A", Email: "scope-svc-owner@example.com"}
	other := models.User{Username: "scope-svc-other", Password: "password123!A", Email: "scope-svc-other@example.com"}
	require.NoError(t, db.Create(&owner).Error)
	require.NoError(t, db.Create(&other).Error)

	a := models.Circle{UserID: owner.ID, Name: "A"}
	b := models.Circle{UserID: owner.ID, Name: "B"}
	otherCircle := models.Circle{UserID: other.ID, Name: "Theirs"}
	require.NoError(t, db.Create(&a).Error)
	require.NoError(t, db.Create(&b).Error)
	require.NoError(t, db.Create(&otherCircle).Error)

	c1 := models.Contact{UserID: owner.ID, Firstname: "One"}
	c2 := models.Contact{UserID: owner.ID, Firstname: "Two"}
	c3 := models.Contact{UserID: owner.ID, Firstname: "Three"}
	require.NoError(t, db.Create(&c1).Error)
	require.NoError(t, db.Create(&c2).Error)
	require.NoError(t, db.Create(&c3).Error)

	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: owner.ID, MemberVCardUID: c1.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: a.ID, UserID: owner.ID, MemberVCardUID: c2.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: b.ID, UserID: owner.ID, MemberVCardUID: c2.VCardUID}).Error)

	return &scopeFixture{db: db, owner: owner, other: other, a: a, b: b, c1: c1, c2: c2, c3: c3}
}

func TestLoadCardDAVSyncScope_DefaultAndStored(t *testing.T) {
	f := newScopeFixture(t)

	scope, ids, err := LoadCardDAVSyncScope(f.db, f.owner.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CardDAVSyncScopeModeAll, scope.Mode)
	assert.NotNil(t, ids)
	assert.Empty(t, ids)

	_, err = ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID,
	})
	require.NoError(t, err)

	scope, ids, err = LoadCardDAVSyncScope(f.db, f.owner.ID)
	require.NoError(t, err)
	assert.Equal(t, models.CardDAVSyncScopeModeInclude, scope.Mode)
	require.NotNil(t, scope.DefaultCircleID)
	assert.Equal(t, f.a.ID, *scope.DefaultCircleID)
	assert.Equal(t, []string{f.a.ID}, ids)
}

func TestLoadCardDAVSyncScope_Errors(t *testing.T) {
	f := newScopeFixture(t)

	dbtest.HideTable(t, f.db, "carddav_sync_scopes")
	_, _, err := LoadCardDAVSyncScope(f.db, f.owner.ID)
	require.Error(t, err)
}

func TestLoadCardDAVSyncScope_CircleListError(t *testing.T) {
	f := newScopeFixture(t)
	_, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeAll})
	require.NoError(t, err)
	dbtest.HideTable(t, f.db, "carddav_sync_scope_circles")

	_, _, err = LoadCardDAVSyncScope(f.db, f.owner.ID)
	require.Error(t, err)
}

func TestCardDAVSyncScopeSelectionFrom_CopiesIDs(t *testing.T) {
	ids := []string{"b", "a"}
	sel := CardDAVSyncScopeSelectionFrom(models.CardDAVSyncScope{Mode: models.CardDAVSyncScopeModeExclude}, ids)
	assert.Equal(t, models.CardDAVSyncScopeModeExclude, sel.Mode)
	assert.Equal(t, []string{"b", "a"}, sel.CircleIDs)
	ids[0] = "changed"
	assert.Equal(t, []string{"b", "a"}, sel.CircleIDs, "selection must not alias the caller's slice")
}

func TestValidateCardDAVSyncScope_AllModes(t *testing.T) {
	f := newScopeFixture(t)

	sel, appErr := ValidateCardDAVSyncScope(f.db, f.owner.ID, models.CardDAVSyncScopeInput{Mode: models.CardDAVSyncScopeModeAll})
	require.Nil(t, appErr)
	assert.Equal(t, models.CardDAVSyncScopeModeAll, sel.Mode)
	assert.Empty(t, sel.CircleIDs)

	sel, appErr = ValidateCardDAVSyncScope(f.db, f.owner.ID, models.CardDAVSyncScopeInput{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.b.ID, f.a.ID, f.a.ID}, DefaultCircleID: &f.a.ID,
	})
	require.Nil(t, appErr)
	wantIDs := []string{f.a.ID, f.b.ID}
	sort.Strings(wantIDs)
	assert.Equal(t, wantIDs, sel.CircleIDs, "ids are deduplicated and sorted")

	sel, appErr = ValidateCardDAVSyncScope(f.db, f.owner.ID, models.CardDAVSyncScopeInput{
		Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID}, DefaultCircleID: &f.a.ID,
	})
	require.Nil(t, appErr)
	assert.Equal(t, f.a.ID, *sel.DefaultCircleID)

	sel, appErr = ValidateCardDAVSyncScope(f.db, f.owner.ID, models.CardDAVSyncScopeInput{
		Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID},
	})
	require.Nil(t, appErr)
	assert.Nil(t, sel.DefaultCircleID)
}

func TestValidateCardDAVSyncScope_Rejections(t *testing.T) {
	f := newScopeFixture(t)
	otherCircle := models.Circle{UserID: f.other.ID, Name: "Other"}
	require.NoError(t, f.db.Create(&otherCircle).Error)
	missing := "00000000-0000-4000-8000-000000000000"

	cases := map[string]models.CardDAVSyncScopeInput{
		"all with circles":         {Mode: "all", CircleIDs: []string{f.a.ID}},
		"all with default":         {Mode: "all", DefaultCircleID: &f.a.ID},
		"include empty":            {Mode: "include"},
		"include no default":       {Mode: "include", CircleIDs: []string{f.a.ID}},
		"include default not in":   {Mode: "include", CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.b.ID},
		"exclude empty":            {Mode: "exclude"},
		"exclude default excluded": {Mode: "exclude", CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID},
		"unknown mode":             {Mode: "some"},
		"unknown circle":           {Mode: "include", CircleIDs: []string{missing}, DefaultCircleID: &missing},
		"other user's circle":      {Mode: "include", CircleIDs: []string{otherCircle.ID}, DefaultCircleID: &otherCircle.ID},
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			_, appErr := ValidateCardDAVSyncScope(f.db, f.owner.ID, input)
			require.NotNil(t, appErr)
			assert.Equal(t, string(apperrors.ReasonIntegrationRequestInvalid), appErr.ReasonCode)
		})
	}
}

func TestValidateCardDAVSyncScope_DatabaseError(t *testing.T) {
	f := newScopeFixture(t)
	dbtest.HideTable(t, f.db, "circles")

	_, appErr := ValidateCardDAVSyncScope(f.db, f.owner.ID, models.CardDAVSyncScopeInput{
		Mode: "include", CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID,
	})
	require.NotNil(t, appErr)
	assert.Equal(t, string(apperrors.ReasonCommonDatabaseError), appErr.ReasonCode)
}

func TestReplaceCardDAVSyncScope_ReplacesAndPreservesCreatedAt(t *testing.T) {
	f := newScopeFixture(t)
	first, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID, f.b.ID}, DefaultCircleID: &f.a.ID,
	})
	require.NoError(t, err)
	require.False(t, first.CreatedAt.IsZero())

	second, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID},
	})
	require.NoError(t, err)
	assert.Equal(t, first.CreatedAt, second.CreatedAt)
	assert.Equal(t, models.CardDAVSyncScopeModeExclude, second.Mode)
	assert.Nil(t, second.DefaultCircleID)

	var rows []models.CardDAVSyncScopeCircle
	require.NoError(t, f.db.Where("user_id = ?", f.owner.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, f.b.ID, rows[0].CircleID)
}

func TestReplaceCardDAVSyncScope_Errors(t *testing.T) {
	f := newScopeFixture(t)
	dbtest.HideTable(t, f.db, "carddav_sync_scope_circles")

	_, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID,
	})
	require.Error(t, err)
}

func TestReplaceCardDAVSyncScope_CircleInsertError(t *testing.T) {
	f := newScopeFixture(t)
	_, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{
		Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{"00000000-0000-4000-8000-000000000000"},
	})
	require.Error(t, err)

	var count int64
	require.NoError(t, f.db.Model(&models.CardDAVSyncScope{}).Where("user_id = ?", f.owner.ID).Count(&count).Error)
	assert.Zero(t, count, "the transaction rolls back with the failed join row")
}

func TestCardDAVSyncScopeCounts_AllModes(t *testing.T) {
	f := newScopeFixture(t)

	in, total, err := CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeAll})
	require.NoError(t, err)
	assert.Equal(t, 3, in)
	assert.Equal(t, 3, total)

	in, total, err = CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}})
	require.NoError(t, err)
	assert.Equal(t, 2, in)
	assert.Equal(t, 3, total)

	in, total, err = CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude})
	require.NoError(t, err)
	assert.Equal(t, 0, in)
	assert.Equal(t, 3, total)

	in, total, err = CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID}})
	require.NoError(t, err)
	assert.Equal(t, 2, in)
	assert.Equal(t, 3, total)

	in, total, err = CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude})
	require.NoError(t, err)
	assert.Equal(t, 3, in)
	assert.Equal(t, 3, total)
}

func TestCardDAVSyncScopeCounts_Errors(t *testing.T) {
	f := newScopeFixture(t)
	dbtest.HideTable(t, f.db, "contacts")

	_, _, err := CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeAll})
	require.Error(t, err)
}

func TestCardDAVSyncScopeCounts_IncludeAndExcludeQueryErrors(t *testing.T) {
	f := newScopeFixture(t)
	dbtest.HideTable(t, f.db, "circle_members")

	_, _, err := CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}})
	require.Error(t, err)

	_, _, err = CardDAVSyncScopeCounts(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.a.ID}})
	require.Error(t, err)
}

func TestCardDAVSyncScopeWarnings(t *testing.T) {
	assert.Empty(t, CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeAll}))
	assert.Len(t, CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude}), 1)
	assert.Equal(t, models.CardDAVSyncScopeWarningEmpty, CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude})[0].Code)

	w := CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{"c"}})
	require.Len(t, w, 1)
	assert.Equal(t, models.CardDAVSyncScopeWarningDefaultMissing, w[0].Code)

	w = CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{"c"}})
	assert.Empty(t, w)
	assert.Empty(t, CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{"c"}, DefaultCircleID: ptr("d")}))
	w = CardDAVSyncScopeWarnings(CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude})
	require.Len(t, w, 1)
	assert.Equal(t, models.CardDAVSyncScopeWarningExclusionEmpty, w[0].Code)
}

func TestApplyCardDAVSyncScope_AllModes(t *testing.T) {
	f := newScopeFixture(t)

	count := func() int64 {
		var n int64
		require.NoError(t, ApplyCardDAVSyncScope(f.db.Model(&models.Contact{}).Where("user_id = ?", f.owner.ID)).Count(&n).Error)
		return n
	}

	assert.EqualValues(t, 3, count())

	_, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID})
	require.NoError(t, err)
	assert.EqualValues(t, 2, count())

	_, err = ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeExclude, CircleIDs: []string{f.b.ID}})
	require.NoError(t, err)
	assert.EqualValues(t, 2, count())
}

func TestContactInCardDAVSyncScope(t *testing.T) {
	f := newScopeFixture(t)
	_, err := ReplaceCardDAVSyncScope(f.db, f.owner.ID, CardDAVSyncScopeSelection{Mode: models.CardDAVSyncScopeModeInclude, CircleIDs: []string{f.a.ID}, DefaultCircleID: &f.a.ID})
	require.NoError(t, err)

	in, err := ContactInCardDAVSyncScope(f.db, f.owner.ID, f.c1)
	require.NoError(t, err)
	assert.True(t, in)

	in, err = ContactInCardDAVSyncScope(f.db, f.owner.ID, f.c3)
	require.NoError(t, err)
	assert.False(t, in)

	dbtest.HideTable(t, f.db, "contacts")
	_, err = ContactInCardDAVSyncScope(f.db, f.owner.ID, f.c1)
	require.Error(t, err)
}
