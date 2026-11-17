package services

import (
	"context"
	"testing"
	"time"

	"mycorrhizal/internal/clock"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2663: the "Import updated this contact" note written when an import
// merges into an existing contact was saved with the zero time (0001-01-01),
// so every merge import produced a note that sorted last and rendered as
// 01.01.0001. Every merge path (CSV, VCF/JSContact/records, and the Monica /
// Meerkat / bundle source engine) goes through CreateMergeNote, so the
// regression is pinned at all three call sites plus a zero-date sweep over
// every dated record an import can create.

var importNoteDateNow = time.Date(2026, time.November, 17, 9, 30, 15, 0, time.UTC)

// assertNoZeroImportDates fails when any note, activity, reminder or life event
// (soft-deleted included) carries the zero time.
func assertNoZeroImportDates(t *testing.T, db *gorm.DB) {
	t.Helper()
	zero := time.Time{}
	var notes []models.Note
	require.NoError(t, db.Unscoped().Find(&notes).Error)
	for _, n := range notes {
		assert.False(t, n.Date.IsZero() || n.Date.Equal(zero), "note %d (%.40q) has a zero date", n.ID, n.Content)
	}
	var activities []models.Activity
	require.NoError(t, db.Unscoped().Find(&activities).Error)
	for _, a := range activities {
		assert.False(t, a.Date.IsZero(), "activity %s has a zero date", a.UUID)
	}
	var reminders []models.Reminder
	require.NoError(t, db.Unscoped().Find(&reminders).Error)
	for _, r := range reminders {
		assert.False(t, r.RemindAt.IsZero(), "reminder %s has a zero remind_at", r.UUID)
	}
}

func assertMergeNoteDatedAtImport(t *testing.T, db *gorm.DB, contactID uint, prefix string) {
	t.Helper()
	var merge models.Note
	require.NoError(t, db.Where("contact_id = ? AND content LIKE ?", contactID, prefix+"%").First(&merge).Error)
	assert.WithinDuration(t, importNoteDateNow, merge.Date, time.Second, "the merge note must be dated at import time, from the injected clock")
	assert.Equal(t, time.UTC, merge.Date.Location())
}

func TestCreateMergeNote_IsDatedAtImportTime(t *testing.T) {
	db := setupImportSessionTestDB(t)
	defer SetClock(clock.NewFake(importNoteDateNow))()

	existing := models.Contact{UserID: 1, Firstname: "Old", Lastname: "Name"}
	require.NoError(t, db.Create(&existing).Error)
	incoming := &models.Contact{Firstname: "New", Lastname: "Name"}

	require.NoError(t, CreateMergeNote(db, 1, existing.ID, &existing, incoming, "CSV"))

	assertMergeNoteDatedAtImport(t, db, existing.ID, "CSV Import updated this contact")
	assertNoZeroImportDates(t, db)
}

func TestConfirmCSVUpdate_MergeNoteIsDated(t *testing.T) {
	db := setupImportSessionTestDB(t)
	defer SetClock(clock.NewFake(importNoteDateNow))()
	m := NewImportSessionManager()

	existing := models.Contact{UserID: 1, Firstname: "Old", Lastname: "Name", Email: "match@example.com"}
	require.NoError(t, db.Create(&existing).Error)
	id := m.CreateCSVSession(1, []string{"First Name", "Last Name", "Email"}, [][]string{{"New", "Name", "match@example.com"}})
	_, appErr := m.PreviewCSV(db, 1, models.ImportPreviewRequest{SessionID: id, Mappings: csvMappings()})
	require.Nil(t, appErr)

	result, appErr := m.Confirm(db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "update"}}}, testImportLogger())
	require.Nil(t, appErr)
	require.Equal(t, 1, result.Updated)

	assertMergeNoteDatedAtImport(t, db, existing.ID, "CSV Import updated this contact")
	assertNoZeroImportDates(t, db)
}

func TestConfirmVCFUpdate_MergeNoteIsDated(t *testing.T) {
	db := setupImportSessionTestDB(t)
	defer SetClock(clock.NewFake(importNoteDateNow))()
	m := NewImportSessionManager()

	existing := models.Contact{UserID: 1, Firstname: "Has", Lastname: "Name", Email: "vcf@example.com"}
	require.NoError(t, db.Create(&existing).Error)
	incoming := &models.Contact{Firstname: "Renamed", Lastname: "Name", Email: "vcf@example.com"}
	previews := []models.ImportRowPreview{{
		RowIndex:        0,
		DuplicateMatch:  &models.DuplicateMatch{ExistingContactID: existing.ID, MatchReason: "email"},
		SuggestedAction: "update",
	}}
	id := m.CreateVCFSession(1, []VCFContactData{{Contact: incoming}}, previews)

	result, appErr := m.ConfirmVCF(context.Background(), db, 1, models.ImportConfirmRequest{SessionID: id, Actions: []models.RowImportAction{{RowIndex: 0, Action: "update"}}}, testImportConfig(t), testImportLogger())
	require.Nil(t, appErr)
	require.Equal(t, 1, result.Updated)

	assertMergeNoteDatedAtImport(t, db, existing.ID, "VCF Import updated this contact")
	assertNoZeroImportDates(t, db)
}

func TestSourceImportMerge_NotesAreDated(t *testing.T) {
	db := setupSourceImportTestDB(t)
	defer SetClock(clock.NewFake(importNoteDateNow))()
	user := createSourceImportUser(t, db)

	existing := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "WrongName"}
	require.NoError(t, db.Create(&existing).Error)

	plan := &ImportSourcePlan{System: "monica"}
	plan.Contacts = []MappedContact{{Ref: ref("contact/1"), Record: minimalRecord("Ada", "Lovelace")}}
	plan.Notes = []MappedNote{{Ref: ref("note/1"), Contact: ref("contact/1"), Content: "met at a talk", Date: "2023-05-01T00:00:00Z"}}
	actions := map[string]SourceContactAction{"contact/1": {Action: SourceActionMerge, MergeTargetUID: existing.VCardUID}}

	report, _, err := ExecuteSourceImportWithActions(context.Background(), db, user.ID, plan, actions, nil)
	require.NoError(t, err)
	require.Equal(t, 1, report.ContactsUpdated)

	assertMergeNoteDatedAtImport(t, db, existing.ID, "Monica Import updated this contact")
	// The source's own note keeps its own date: the fix must not re-date it.
	var own models.Note
	require.NoError(t, db.Where("contact_id = ? AND content = ?", existing.ID, "met at a talk").First(&own).Error)
	assert.Equal(t, time.Date(2023, time.May, 1, 0, 0, 0, 0, time.UTC), own.Date.UTC())
	assertNoZeroImportDates(t, db)
}
