package services

// Per-contact field visibility service error paths (issue #1997): a closed
// database and a hidden/blocked contact_field_overrides table exercise every
// database-error return without changing any successful-path assertions.

import (
	"errors"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/stretchr/testify/require"
)

func TestGetContactFieldVisibility_UserQueryError(t *testing.T) {
	f := newCascadeFixture(t)
	sqlDB, err := f.db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = GetContactFieldVisibility(f.db, f.userID, f.contact.VCardUID)
	require.Error(t, err)
}

func TestGetContactFieldVisibility_OverridesQueryError(t *testing.T) {
	f := newCascadeFixture(t)
	dbtest.HideTable(t, f.db, "contact_field_overrides")

	_, err := GetContactFieldVisibility(f.db, f.userID, f.contact.VCardUID)
	require.Error(t, err)
}

func TestReplaceContactFieldOverrides_DeleteError(t *testing.T) {
	f := newCascadeFixture(t)
	dbtest.HideTable(t, f.db, "contact_field_overrides")

	err := ReplaceContactFieldOverrides(f.db, f.userID, f.contact.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	})
	require.Error(t, err)
	var oe *FieldOverrideError
	require.False(t, errors.As(err, &oe), "a database failure is not a validation error")
}

func TestReplaceContactFieldOverrides_UpsertError(t *testing.T) {
	f := newCascadeFixture(t)
	require.NoError(t, f.db.Exec(
		`CREATE TRIGGER fail_override_insert BEFORE INSERT ON contact_field_overrides
		 BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END;`,
	).Error)

	err := ReplaceContactFieldOverrides(f.db, f.userID, f.contact.VCardUID, models.ContactFieldVisibilityInput{
		Overrides: map[string]string{"emails": "hide"},
	})
	require.Error(t, err)
	var oe *FieldOverrideError
	require.False(t, errors.As(err, &oe), "a database failure is not a validation error")
}

// --- account bundle helper -------------------------------------------------

func TestAttachBundleFieldOverrides_UserQueryError(t *testing.T) {
	db := dbtest.New(t)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	err = attachBundleFieldOverrides(db, 1, &models.AccountBundle{})
	require.Error(t, err)
}

func TestAttachBundleFieldOverrides_OverridesQueryError(t *testing.T) {
	db := dbtest.New(t)
	user := createSourceImportUser(t, db)
	dbtest.HideTable(t, db, "contact_field_overrides")

	err := attachBundleFieldOverrides(db, user.ID, &models.AccountBundle{})
	require.Error(t, err)
}

func TestAttachBundleFieldOverrides_SkipsRowsForContactsNotInBundle(t *testing.T) {
	db := dbtest.New(t)
	user := createSourceImportUser(t, db)
	require.NoError(t, db.Create(&models.ContactFieldOverride{
		UserID: user.ID, ContactVCardUID: "not-in-bundle", FieldKey: "emails", State: "hide",
	}).Error)

	bundle := &models.AccountBundle{}
	bundle.Plan.Contacts = []models.AccountBundleContact{{UID: "in-bundle"}}
	require.NoError(t, attachBundleFieldOverrides(db, user.ID, bundle))
	require.Len(t, bundle.Plan.Contacts, 1)
	require.Empty(t, bundle.Plan.Contacts[0].FieldOverrides)
}

// --- source import error paths --------------------------------------------

func TestExecuteSourceImport_FieldOverridePersistError(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	require.NoError(t, db.Exec(
		`CREATE TRIGGER fail_override_insert BEFORE INSERT ON contact_field_overrides
		 BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END;`,
	).Error)

	plan := &ImportSourcePlan{System: "test"}
	plan.Contacts = []MappedContact{{
		Ref:            ref("contact/1"),
		Record:         minimalRecord("Ada", "Lovelace"),
		FieldOverrides: map[string]string{"emails": "hide"},
	}}
	_, err := ExecuteSourceImport(db, user.ID, plan)
	require.Error(t, err)
}

func TestExecuteSourceImport_HideEmptyDefaultUpdateError(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)
	require.NoError(t, db.Exec(
		`CREATE TRIGGER fail_hide_empty_update BEFORE UPDATE OF hide_empty_fields ON users
		 WHEN NEW.hide_empty_fields = 1
		 BEGIN SELECT RAISE(ABORT, 'injected update failure'); END;`,
	).Error)

	plan := &ImportSourcePlan{System: "test", HideEmptyFields: true}
	plan.Contacts = []MappedContact{{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")}}
	_, err := ExecuteSourceImport(db, user.ID, plan)
	require.Error(t, err)
}
