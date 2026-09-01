package services

import (
	"strings"
	"testing"

	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The per-contact action map (issues #549/#550) lets an import assistant's
// review step exclude a contact or merge it into an existing local one,
// instead of the all-or-nothing "create every contact" ExecuteSourceImport
// keeps.

func TestExecuteSourceImportWithActions_SkipExcludesContactAndItsGraph(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "monica"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
		{Ref: ref("contact/2"), Record: minimalRecord("Ben", "Babbage")},
	}
	plan.Notes = []MappedNote{{Ref: ref("note/1"), Contact: ref("contact/1"), Content: "hi", Date: "2023-01-01T00:00:00Z"}}
	plan.Relationships = []MappedRelationship{{
		Ref: ref("rel/1"), Source: ref("contact/1"), Target: ref("contact/2"), Type: "friend_of",
	}}

	actions := map[string]SourceContactAction{
		"contact/1": {Action: SourceActionSkip},
	}
	report, ids, err := ExecuteSourceImportWithActions(db, user.ID, plan, actions)
	require.NoError(t, err)

	assert.Equal(t, 1, report.ContactsCreated)
	assert.Equal(t, 1, report.ContactsSkipped)
	assert.Equal(t, 0, report.ContactsUpdated)
	assert.Equal(t, 0, report.NotesCreated, "a note on a skipped contact is not created")
	assert.Equal(t, 0, report.RelationshipsCreated, "an edge to a skipped contact is not created")

	_, hasSkipped := ids["contact/1"]
	assert.False(t, hasSkipped)
	_, hasCreated := ids["contact/2"]
	assert.True(t, hasCreated)

	var count int64
	db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count)
	assert.Equal(t, int64(1), count)

	// The skip and its knock-on drops are all named on the report.
	var skipped, dropped bool
	for _, iss := range report.Issues {
		if iss.Category == ImportIssueCategorySkipped && strings.Contains(iss.Record, "contact/1") {
			skipped = true
		}
		if iss.Category == ImportIssueCategoryUnsupported {
			dropped = true
		}
	}
	assert.True(t, skipped, "the excluded contact is named")
	assert.True(t, dropped, "graph entities referencing it are named, not silently dropped")
}

func TestExecuteSourceImportWithActions_MergeIntoExistingContact(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	existing := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "WrongName"}
	require.NoError(t, db.Create(&existing).Error)
	require.NotEmpty(t, existing.VCardUID)

	plan := &ImportSourcePlan{System: "monica"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
	}
	plan.Notes = []MappedNote{{Ref: ref("note/1"), Contact: ref("contact/1"), Content: "met at a talk", Date: "2023-05-01T00:00:00Z"}}

	actions := map[string]SourceContactAction{
		"contact/1": {Action: SourceActionMerge, MergeTargetUID: existing.VCardUID},
	}
	report, ids, err := ExecuteSourceImportWithActions(db, user.ID, plan, actions)
	require.NoError(t, err)

	assert.Equal(t, 0, report.ContactsCreated)
	assert.Equal(t, 1, report.ContactsUpdated)

	// No new contact row; the existing one took the mapped surname.
	var count int64
	db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count)
	assert.Equal(t, int64(1), count)
	var merged models.Contact
	require.NoError(t, db.First(&merged, existing.ID).Error)
	assert.Equal(t, "Lovelace", merged.Lastname)

	// A merge note documents the change; the imported note resolves onto the
	// merged contact.
	var mergeNotes int64
	db.Model(&models.Note{}).Where("contact_id = ? AND content LIKE ?", existing.ID, "Monica%").Count(&mergeNotes)
	assert.Equal(t, int64(1), mergeNotes)
	assert.Equal(t, 1, report.NotesCreated)

	// The idempotency ledger records the merge so a re-run skips it.
	var links int64
	db.Model(&models.ImportSourceLink{}).
		Where("user_id = ? AND system = ? AND external_id = ?", user.ID, "monica", "contact/1").
		Count(&links)
	assert.Equal(t, int64(1), links)

	assert.Equal(t, existing.ID, ids["contact/1"])
}

func TestExecuteSourceImportWithActions_MergeTargetNotFound(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "monica"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
	}
	actions := map[string]SourceContactAction{
		"contact/1": {Action: SourceActionMerge, MergeTargetUID: "does-not-exist"},
	}
	report, _, err := ExecuteSourceImportWithActions(db, user.ID, plan, actions)
	require.NoError(t, err)

	assert.Equal(t, 0, report.ContactsCreated)
	assert.Equal(t, 0, report.ContactsUpdated)
	var count int64
	db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&count)
	assert.Equal(t, int64(0), count)

	var named bool
	for _, iss := range report.Issues {
		if iss.Category == ImportIssueCategoryInvalid && strings.Contains(iss.Record, "contact/1") {
			named = true
		}
	}
	assert.True(t, named, "a missing merge target is a named failure, not a crash")
}

func TestExecuteSourceImportWithActions_NilActionsCreatesEveryContact(t *testing.T) {
	db := setupSourceImportTestDB(t)
	user := createSourceImportUser(t, db)

	plan := &ImportSourcePlan{System: "monica"}
	plan.Contacts = []MappedContact{
		{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")},
		{Ref: ref("contact/2"), Record: minimalRecord("Ben", "Babbage")},
	}
	report, ids, err := ExecuteSourceImportWithActions(db, user.ID, plan, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, report.ContactsCreated)
	assert.Len(t, ids, 2)
}
