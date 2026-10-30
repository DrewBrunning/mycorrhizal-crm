package services

import (
	"encoding/json"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue #1997: per-contact field visibility is owner preference data. The
// account bundle is the full-fidelity owner export, so the overrides and the
// account hide-empty default travel in it and land on the destination keyed by
// the imported contact's (stable) vcard_uid.
func TestAccountBundle_FieldOverridesRoundTrip(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "ovr-owner")
	target := bundleTestUser(t, targetDB, "ovr-target")
	useBundlePhotoDir(t)

	ada := models.Contact{UserID: owner.ID, Firstname: "Ada"}
	bob := models.Contact{UserID: owner.ID, Firstname: "Bob"}
	require.NoError(t, ownerDB.Create(&ada).Error)
	require.NoError(t, ownerDB.Create(&bob).Error)
	require.NoError(t, ReplaceContactFieldOverrides(ownerDB, owner.ID, ada.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "suffix": "show"}, HideEmpty: "hide",
	}))
	require.NoError(t, ownerDB.Model(&models.User{}).Where("id = ?", owner.ID).Update("hide_empty_fields", true).Error)

	bundle, _, err := BuildAccountBundle(ownerDB, owner.ID, t.TempDir())
	require.NoError(t, err)
	assert.True(t, bundle.HideEmptyFields)

	// Wire shape: omitted for a contact with no overrides.
	raw, err := json.Marshal(bundle.Plan.Contacts)
	require.NoError(t, err)
	var wire []map[string]any
	require.NoError(t, json.Unmarshal(raw, &wire))
	for _, c := range wire {
		if c["uid"] == ada.VCardUID {
			assert.Equal(t, map[string]any{"emails": "hide", "suffix": "show"}, c["field_overrides"])
			assert.Equal(t, "hide", c["hide_empty_override"])
		} else {
			assert.NotContains(t, c, "field_overrides")
			assert.NotContains(t, c, "hide_empty_override")
		}
	}

	report, _, err := ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues)

	var landed models.Contact
	require.NoError(t, targetDB.Where("user_id = ? AND firstname = ?", target.ID, "Ada").First(&landed).Error)
	got, err := GetContactFieldVisibility(targetDB, target.ID, landed.VCardUID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"emails": "hide", "suffix": "show"}, got.Overrides)
	assert.Equal(t, "hide", got.HideEmpty)
	assert.True(t, got.Defaults.HideEmpty, "the account default came across")

	var landedBob models.Contact
	require.NoError(t, targetDB.Where("user_id = ? AND firstname = ?", target.ID, "Bob").First(&landedBob).Error)
	gotBob, err := GetContactFieldVisibility(targetDB, target.ID, landedBob.VCardUID)
	require.NoError(t, err)
	assert.Empty(t, gotBob.Overrides)

	// Re-importing is a no-op (import_source_links ledger): no duplicates.
	_, _, err = ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(bundle), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count(t, targetDB, false, &models.ContactFieldOverride{}, "user_id = ?", target.ID))

	// A re-export from the destination carries the same overrides.
	again, _, err := BuildAccountBundle(targetDB, target.ID, t.TempDir())
	require.NoError(t, err)
	assert.True(t, again.HideEmptyFields)
	for _, c := range again.Plan.Contacts {
		if c.UID == landed.VCardUID {
			assert.Equal(t, map[string]string{"emails": "hide", "suffix": "show"}, c.FieldOverrides)
			assert.Equal(t, "hide", c.HideEmptyOverride)
		}
	}
}

func TestAccountBundle_ImportDropsInvalidFieldOverrides(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	bundle := &models.AccountBundle{
		Format: models.AccountBundleFormat, Version: models.AccountBundleVersion,
		Plan: models.AccountBundlePlan{Contacts: []models.AccountBundleContact{{
			UID:  "11111111-1111-4111-8111-111111111111",
			Card: minimalRecord("Ada", "Lovelace").Card,
			FieldOverrides: map[string]string{
				"emails":       "hide",
				"firstname":    "hide",  // not an overridable field
				"phones":       "maybe", // not a stored state
				"suffix":       "inherit",
				"birthday":     "show",
				"__hide_empty": "hide", // reserved key is not a field
			},
			HideEmptyOverride: "on",
		}}},
	}
	report, err := ExecuteSourceImport(db, user.ID, MapAccountBundle(bundle))
	require.NoError(t, err)
	require.Empty(t, report.Issues)

	got, err := GetContactFieldVisibility(db, user.ID, "11111111-1111-4111-8111-111111111111")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"emails": "hide", "birthday": "show"}, got.Overrides, "only validated entries are persisted")
	assert.Equal(t, models.FieldStateInherit, got.HideEmpty)
}

func TestAccountBundle_ImportKeepsDestinationHideEmptyChoiceAndOlderBundlesAreFine(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	// An older bundle (no field-visibility members) imports cleanly and
	// leaves the destination account default alone.
	old := &models.AccountBundle{Format: models.AccountBundleFormat, Version: models.AccountBundleVersion}
	assert.False(t, MapAccountBundle(old).HideEmptyFields)
	_, err := ExecuteSourceImport(db, user.ID, MapAccountBundle(old))
	require.NoError(t, err)
	var u models.User
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.False(t, u.HideEmptyFields)

	// A bundle that says hide-empty is adopted while the destination is still
	// on the default (off) ...
	on := &models.AccountBundle{Format: models.AccountBundleFormat, Version: models.AccountBundleVersion, HideEmptyFields: true}
	_, err = ExecuteSourceImport(db, user.ID, MapAccountBundle(on))
	require.NoError(t, err)
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.True(t, u.HideEmptyFields)

	// ... and a bundle that says off never switches a destination choice off.
	off := &models.AccountBundle{Format: models.AccountBundleFormat, Version: models.AccountBundleVersion}
	_, err = ExecuteSourceImport(db, user.ID, MapAccountBundle(off))
	require.NoError(t, err)
	require.NoError(t, db.First(&u, user.ID).Error)
	assert.True(t, u.HideEmptyFields, "an import never overwrites the destination's own choice")
}
