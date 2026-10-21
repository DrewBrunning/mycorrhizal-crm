package services

import (
	"context"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1796: ApplyRecordToContact never writes target_closeness, so the
// account bundle / source import sets it explicitly.

func TestBundleTargetCloseness_OnlyVocabularyTiersSurvive(t *testing.T) {
	assert.Nil(t, bundleTargetCloseness(nil))
	for _, tier := range models.TargetClosenessTiers() {
		got := bundleTargetCloseness(strPtr(tier))
		require.NotNil(t, got)
		assert.Equal(t, tier, *got)
	}
	assert.Nil(t, bundleTargetCloseness(strPtr("bestie")), "a token outside the vocabulary is dropped")
	assert.Nil(t, bundleTargetCloseness(strPtr("")))
}

func TestMapAccountBundle_CarriesTargetCloseness(t *testing.T) {
	bundle := &models.AccountBundle{}
	bundle.Plan.Contacts = []models.AccountBundleContact{
		{UID: "11111111-1111-4111-8111-111111111111", CRM: contactmodel.CRMEnvelope{TargetCloseness: strPtr("close")}},
		{UID: "22222222-2222-4222-8222-222222222222", CRM: contactmodel.CRMEnvelope{TargetCloseness: strPtr("bestie")}},
		{UID: "33333333-3333-4333-8333-333333333333"},
	}
	plan := MapAccountBundle(bundle)
	require.Len(t, plan.Contacts, 3)
	require.NotNil(t, plan.Contacts[0].TargetCloseness)
	assert.Equal(t, "close", *plan.Contacts[0].TargetCloseness)
	assert.Nil(t, plan.Contacts[1].TargetCloseness)
	assert.Nil(t, plan.Contacts[2].TargetCloseness)
}

func TestSourceImport_TargetClosenessOnCreateAndFillOnlyWhenEmptyOnMerge(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	// Create: the mapped target lands in the column.
	plan := &ImportSourcePlan{System: "mycorrhizal"}
	plan.Contacts = []MappedContact{{Ref: ref("contact/new"), Record: minimalRecord("New", "Person"), TargetCloseness: strPtr("casual")}}
	_, _, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, nil, nil)
	require.NoError(t, err)
	var created models.Contact
	require.NoError(t, db.Where("user_id = ? AND firstname = ?", user.ID, "New").First(&created).Error)
	require.NotNil(t, created.TargetCloseness)
	assert.Equal(t, "casual", *created.TargetCloseness)

	// Merge into a contact with no target: filled.
	empty := models.Contact{UserID: user.ID, Firstname: "Empty", Lastname: "Target"}
	require.NoError(t, db.Create(&empty).Error)
	// Merge into a contact that already has one: kept.
	has := models.Contact{UserID: user.ID, Firstname: "Has", Lastname: "Target", TargetCloseness: strPtr("immediate")}
	require.NoError(t, db.Create(&has).Error)

	plan = &ImportSourcePlan{System: "mycorrhizal"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/a"), Record: minimalRecord("Empty", "Target"), TargetCloseness: strPtr("distant")},
		{Ref: ref("contact/b"), Record: minimalRecord("Has", "Target"), TargetCloseness: strPtr("distant")},
	}
	actions := map[string]SourceContactAction{
		"contact/a": {Action: SourceActionMerge, MergeTargetUID: empty.VCardUID},
		"contact/b": {Action: SourceActionMerge, MergeTargetUID: has.VCardUID},
	}
	report, _, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, actions, nil)
	require.NoError(t, err)
	require.Equal(t, 2, report.ContactsUpdated)

	require.NoError(t, db.First(&empty, empty.ID).Error)
	require.NotNil(t, empty.TargetCloseness)
	assert.Equal(t, "distant", *empty.TargetCloseness)
	require.NoError(t, db.First(&has, has.ID).Error)
	require.NotNil(t, has.TargetCloseness)
	assert.Equal(t, "immediate", *has.TargetCloseness, "an existing target is never overwritten by a merge")
}

func TestContactMerge_TargetClosenessResolution(t *testing.T) {
	cases := []struct {
		name          string
		keeper, loser *string
		want          string
		conflict      bool
	}{
		{"keeper nil takes loser's", nil, strPtr("close"), "close", false},
		{"loser nil keeps keeper's", strPtr("casual"), nil, "casual", false},
		{"both equal", strPtr("close"), strPtr("close"), "close", false},
		{"both nil", nil, nil, "", false},
		{"differing values conflict", strPtr("close"), strPtr("distant"), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keeper := &models.Contact{TargetCloseness: tc.keeper}
			loser := &models.Contact{TargetCloseness: tc.loser}
			res := ComputeContactMergeResolution(keeper, loser)

			var conflicted bool
			for _, c := range res.Conflicts {
				if c.Field == "target_closeness" {
					conflicted = true
					assert.Equal(t, "close", c.KeeperValue)
					assert.Equal(t, "distant", c.LoserValue)
				}
			}
			assert.Equal(t, tc.conflict, conflicted)
			if tc.conflict {
				// The caller's resolution decides.
				require.NoError(t, ApplyContactMergeResolution(keeper, res, map[string]string{"target_closeness": "distant"}))
				require.NotNil(t, keeper.TargetCloseness)
				assert.Equal(t, "distant", *keeper.TargetCloseness)
				return
			}
			require.NoError(t, ApplyContactMergeResolution(keeper, res, nil))
			if tc.want == "" {
				assert.Nil(t, keeper.TargetCloseness, "an empty resolution stores NULL, never an empty string")
			} else {
				require.NotNil(t, keeper.TargetCloseness)
				assert.Equal(t, tc.want, *keeper.TargetCloseness)
			}
		})
	}
}
