package services

// Per-contact field visibility (issue #1997): the service, the cascade, trash
// restore and the account bundle. Real migrated schema (trap #1), injected
// clock.

import (
	"errors"
	"testing"
	"time"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetContactFieldVisibility_ResolutionOrder(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID

	// Nothing configured: built-in default.
	got, err := GetContactFieldVisibility(f.db, f.userID, uid)
	require.NoError(t, err)
	assert.True(t, got.Resolved.Fields["emails"])
	assert.False(t, got.Resolved.Fields["suffix"])
	assert.False(t, got.Resolved.HideEmpty)
	assert.Empty(t, got.Overrides)
	assert.Equal(t, models.FieldStateInherit, got.HideEmpty)

	// Account default replaces the built-in set and turns hide-empty on.
	require.NoError(t, f.db.Model(&models.User{}).Where("id = ?", f.userID).
		Updates(map[string]any{"enabled_contact_fields": `["suffix"]`, "hide_empty_fields": true}).Error)
	got, err = GetContactFieldVisibility(f.db, f.userID, uid)
	require.NoError(t, err)
	assert.True(t, got.Resolved.Fields["suffix"])
	assert.False(t, got.Resolved.Fields["emails"])
	assert.True(t, got.Resolved.HideEmpty)

	// Per-contact override beats the account default, in both directions.
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"suffix": "hide", "emails": "show"},
		HideEmpty: "show",
	}))
	got, err = GetContactFieldVisibility(f.db, f.userID, uid)
	require.NoError(t, err)
	assert.False(t, got.Resolved.Fields["suffix"], "override hide beats account show")
	assert.True(t, got.Resolved.Fields["emails"], "override show beats account hide")
	assert.False(t, got.Resolved.HideEmpty, "override show-all beats account hide-empty")
	assert.True(t, got.Defaults.Fields["suffix"], "defaults reflect the account only")
	assert.True(t, got.Defaults.HideEmpty)
	assert.Equal(t, map[string]string{"suffix": "hide", "emails": "show"}, got.Overrides)
	assert.Equal(t, "show", got.HideEmpty)

	// The other contact is untouched.
	other, err := GetContactFieldVisibility(f.db, f.userID, f.other.VCardUID)
	require.NoError(t, err)
	assert.True(t, other.Resolved.Fields["suffix"])
	assert.Empty(t, other.Overrides)
}

func TestReplaceContactFieldOverrides_ReplacesTheWholeSet(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID

	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "phones": "show", "suffix": "hide"},
		HideEmpty: "hide",
	}))
	assert.EqualValues(t, 4, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", uid))

	// Second PUT: emails flips, phones -> inherit (row deleted), suffix absent
	// (row deleted), hide_empty absent (row deleted).
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "show", "phones": "inherit"},
	}))
	var rows []models.ContactFieldOverride
	require.NoError(t, f.db.Where("contact_vcard_uid = ?", uid).Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, "emails", rows[0].FieldKey)
	assert.Equal(t, "show", rows[0].State)

	// Idempotent.
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "show"},
	}))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", uid))

	// An empty set clears everything.
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{}))
	assert.Zero(t, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", uid))
}

func TestReplaceContactFieldOverrides_RejectsBadInputAndWritesNothing(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	}))

	cases := []struct {
		name string
		in   models.ContactFieldVisibilityInput
		kind FieldOverrideErrorKind
		key  string
	}{
		{"unknown key", models.ContactFieldVisibilityInput{Overrides: map[string]string{"phones": "show", "firstname": "hide"}}, FieldOverrideUnknownKey, "firstname"},
		{"reserved key in the map", models.ContactFieldVisibilityInput{Overrides: map[string]string{models.HideEmptyOverrideKey: "hide"}}, FieldOverrideUnknownKey, models.HideEmptyOverrideKey},
		{"bad field state", models.ContactFieldVisibilityInput{Overrides: map[string]string{"phones": "maybe"}}, FieldOverrideInvalidState, "phones"},
		{"bad hide_empty", models.ContactFieldVisibilityInput{HideEmpty: "on"}, FieldOverrideInvalidState, "hide_empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ReplaceContactFieldOverrides(f.db, f.userID, uid, tc.in)
			var oe *FieldOverrideError
			require.True(t, errors.As(err, &oe), "got %v", err)
			assert.Equal(t, tc.kind, oe.Kind)
			assert.Equal(t, tc.key, oe.Key)
			assert.NotEmpty(t, oe.Error())
			// The pre-existing set survives untouched.
			var rows []models.ContactFieldOverride
			require.NoError(t, f.db.Where("contact_vcard_uid = ?", uid).Find(&rows).Error)
			require.Len(t, rows, 1)
			assert.Equal(t, "emails", rows[0].FieldKey)
		})
	}
}

func TestContactFieldOverrides_AreOwnerScoped(t *testing.T) {
	f := newCascadeFixture(t)
	var intruder models.User
	require.NoError(t, f.db.Create(&models.User{Username: "intruder", Password: "x", Email: "intruder@example.com"}).Error)
	require.NoError(t, f.db.Where("username = ?", "intruder").First(&intruder).Error)
	uid := f.contact.VCardUID

	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	}))

	// The other user sees nothing for this uid, and their writes/clears
	// cannot touch the owner's rows.
	got, err := GetContactFieldVisibility(f.db, intruder.ID, uid)
	require.NoError(t, err)
	assert.Empty(t, got.Overrides)
	require.NoError(t, ClearContactFieldOverrides(f.db, intruder.ID, uid))
	require.NoError(t, ReplaceContactFieldOverrides(f.db, intruder.ID, uid, models.ContactFieldVisibilityInput{}))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactFieldOverride{}, "user_id = ?", f.userID))

	owner, err := GetContactFieldVisibility(f.db, f.userID, uid)
	require.NoError(t, err)
	assert.Equal(t, "hide", owner.Overrides["emails"])
}

func TestClearContactFieldOverrides_ResetsToDefault(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"}, HideEmpty: "hide",
	}))
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, f.other.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	}))
	require.NoError(t, ClearContactFieldOverrides(f.db, f.userID, uid))
	assert.Zero(t, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", uid))
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", f.other.VCardUID), "only this contact is reset")
}

func TestReplaceContactFieldOverrides_DoesNotTouchTheContact(t *testing.T) {
	f := newCascadeFixture(t)
	var before models.Contact
	require.NoError(t, f.db.First(&before, f.contact.ID).Error)

	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, f.contact.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"}, HideEmpty: "hide",
	}))
	require.NoError(t, ClearContactFieldOverrides(f.db, f.userID, f.contact.VCardUID))

	var after models.Contact
	require.NoError(t, f.db.First(&after, f.contact.ID).Error)
	assert.Equal(t, before.Revision, after.Revision, "display preferences never bump the revision/ETag")
	assert.Equal(t, before.ETag, after.ETag)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt))
}

func TestDeleteContact_RemovesFieldOverrides(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, f.contact.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"}, HideEmpty: "hide",
	}))
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, f.other.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"phones": "show"},
	}))

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))

	assert.Zero(t, count(t, f.db, true, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", f.contact.VCardUID), "hard-deleted with the contact")
	assert.EqualValues(t, 1, count(t, f.db, true, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", f.other.VCardUID), "another contact's overrides are kept")
}

func TestRestoreContact_BringsFieldOverridesBack(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide", "suffix": "show"}, HideEmpty: "hide",
	}))

	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	res, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow.Add(time.Hour))
	require.NoError(t, err)
	assert.Empty(t, res.Skipped)

	got, err := GetContactFieldVisibility(f.db, f.userID, uid)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"emails": "hide", "suffix": "show"}, got.Overrides)
	assert.Equal(t, "hide", got.HideEmpty)
	assert.False(t, got.Resolved.Fields["emails"])
	assert.True(t, got.Resolved.HideEmpty)
}

func TestRestoreContact_FieldOverrideRestoreNeverDuplicates(t *testing.T) {
	f := newCascadeFixture(t)
	uid := f.contact.VCardUID
	require.NoError(t, ReplaceContactFieldOverrides(f.db, f.userID, uid, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	}))
	require.NoError(t, DeleteContact(f.db, f.contact, f.userID, trashNow))
	// The user re-created a row for the same key while the contact was in the
	// trash: restore keeps the newer row and does not fail.
	require.NoError(t, f.db.Create(&models.ContactFieldOverride{UserID: f.userID, ContactVCardUID: uid, FieldKey: "emails", State: "show"}).Error)

	res, err := RestoreContact(f.db, f.userID, f.contact.ID, trashNow)
	require.NoError(t, err)
	assert.Len(t, res.Skipped, 1)
	assert.EqualValues(t, 1, count(t, f.db, false, &models.ContactFieldOverride{}, "contact_vcard_uid = ?", uid))
	var row models.ContactFieldOverride
	require.NoError(t, f.db.Where("contact_vcard_uid = ?", uid).First(&row).Error)
	assert.Equal(t, "show", row.State, "the newer choice wins")
}

func TestContactFieldOverridesRegistryStep(t *testing.T) {
	var step *CascadeStep
	for _, s := range ContactCascadeRegistry() {
		if s.Table == "contact_field_overrides" {
			s := s
			step = &s
		}
	}
	require.NotNil(t, step, "contact_field_overrides must be in the contact cascade registry (trap #6)")
	assert.Equal(t, CascadeHard, step.Mode)
	assert.Empty(t, step.SnapshotSkip, "overrides are snapshotted and restored with the contact")
	_, restorable := snapshotRestorers["contact_field_overrides"]
	assert.True(t, restorable)
}
