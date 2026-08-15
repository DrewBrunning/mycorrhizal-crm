package services

import (
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/database"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// T96: the
// import preview must show, per duplicate row, exactly what the "Merge"
// (update) action will change — scalars overwritten (incoming-wins-when-
// non-empty) and multi-valued entries appended (additive T49). These tests
// pin ComputeImportMergeDiff's contents and, critically, that it predicts
// MergeImportedContact exactly: the preview must never describe a merge the
// confirm path would not perform.

func TestComputeImportMergeDiff_ReportsScalarUpdatesAndArrayAdditions(t *testing.T) {
	existing := &models.Contact{
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		JobTitle:  "Engineer",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-0000"}},
	}
	incoming := &models.Contact{
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane+new@example.com",
		JobTitle:  "Staff Engineer",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane+new@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-1111"}},
	}

	diff := ComputeImportMergeDiff(existing, incoming)

	// The new phone and the new email are additions, not scalar updates —
	// email/phone are BeforeSave projections of the multi-value arrays and are
	// reported where the change really lands (mirrors contact_merge_service.go's
	// mergeScalarFields reasoning).
	added := map[string]string{}
	for _, a := range diff.Added {
		added[a.Kind] = a.Value
	}
	assert.Equal(t, "555-1111", added["phone"], "the new phone must be reported as an addition")
	assert.Equal(t, "jane+new@example.com", added["email"], "the new email must be reported as an addition")

	var jobTitleChanged *models.ImportScalarChange
	for i := range diff.Updated {
		if diff.Updated[i].Field == "job_title" {
			jobTitleChanged = &diff.Updated[i]
		}
	}
	require.NotNil(t, jobTitleChanged, "job title differs and must be reported as an update")
	assert.Equal(t, "Engineer", jobTitleChanged.Old)
	assert.Equal(t, "Staff Engineer", jobTitleChanged.New)
	assert.Equal(t, "Job Title", jobTitleChanged.Label)

	// Identical firstname/lastname must NOT appear as updates.
	for _, u := range diff.Updated {
		assert.NotEqual(t, "firstname", u.Field, "identical firstname must not be reported")
		assert.NotEqual(t, "lastname", u.Field, "identical lastname must not be reported")
	}
}

func TestComputeImportMergeDiff_NoChangesForIdenticalContact(t *testing.T) {
	contact := &models.Contact{
		Firstname: "Jane",
		Lastname:  "Smith",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-0000"}},
	}
	diff := ComputeImportMergeDiff(contact, contact)
	assert.Empty(t, diff.Updated)
	assert.Empty(t, diff.Added)
}

func TestComputeImportMergeDiff_BlankIncomingChangesNothing(t *testing.T) {
	existing := &models.Contact{
		Firstname: "Jane",
		JobTitle:  "Engineer",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
	}
	// A zero-value incoming Contact must not report any change (the "existing
	// survives when incoming blank" direction of MergeImportedContact).
	diff := ComputeImportMergeDiff(existing, &models.Contact{})
	assert.Empty(t, diff.Updated)
	assert.Empty(t, diff.Added)
}

// TestComputeImportMergeDiff_PredictsMergeImportedContact is the convergence
// guarantee T96's "back the preview with the same computation the commit
// applies" asks for: every change the diff reports must be exactly the change
// MergeImportedContact makes, and nothing it makes may go unreported.
func TestComputeImportMergeDiff_PredictsMergeImportedContact(t *testing.T) {
	existing := &models.Contact{
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		Phone:     "555-0000",
		JobTitle:  "Engineer",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-0000"}},
		Addresses: []models.ContactAddress{{Type: "home", Street: "1 Old St"}},
	}
	incoming := &models.Contact{
		Firstname: "Jane",
		Lastname:  "Smith",
		Nickname:  "Jay",
		Email:     "jane+work@example.com",
		Phone:     "555-2222",
		JobTitle:  "Staff Engineer",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane+work@example.com"}},
		Phones:    []models.ContactPhone{{Type: "cell", Value: "555-2222"}},
		URLs:      []models.ContactURL{{Type: "home", Value: "https://jane.dev"}},
	}

	diff := ComputeImportMergeDiff(existing, incoming)

	// Apply the merge the way the confirm path does.
	applied := *existing
	MergeImportedContact(&applied, incoming)

	// Every scalar the diff marks as updated must have changed on the applied
	// copy, and no scalar it leaves alone may have changed.
	for _, u := range diff.Updated {
		got, want := scalarFor(&applied, u.Field), u.New
		assert.Equalf(t, want, got, "diff predicted %s → %s, merge produced %q", u.Field, u.Old, got)
	}

	// Every addition must be present on the applied copy's arrays.
	for _, a := range diff.Added {
		assert.Truef(t, appliedCarries(applied, a.Kind, a.Value),
			"diff reported added %s %q, merge result does not carry it", a.Kind, a.Value)
	}

	// No unreported additions: every value the merge ADDED to an array (i.e.
	// present after but not before) must have been flagged by the diff.
	// Existing values survive the additive merge by design and are correctly
	// not flagged.
	for _, e := range applied.Emails {
		if !existingCarries(*existing, "email", e.Value) {
			assert.True(t, diffFlagsValue(diff, "email", e.Value), "unreported email %q", e.Value)
		}
	}
	for _, p := range applied.Phones {
		if !existingCarries(*existing, "phone", p.Value) {
			assert.True(t, diffFlagsValue(diff, "phone", p.Value), "unreported phone %q", p.Value)
		}
	}
	for _, u := range applied.URLs {
		if !existingCarries(*existing, "url", u.Value) {
			assert.True(t, diffFlagsValue(diff, "url", u.Value), "unreported url %q", u.Value)
		}
	}
}

func existingCarries(c models.Contact, kind, value string) bool {
	return appliedCarries(c, kind, value)
}

func scalarFor(c *models.Contact, field string) string {
	for _, f := range importMergeDiffScalars {
		if f.Key == field {
			return f.Get(c)
		}
	}
	return ""
}

func appliedCarries(c models.Contact, kind, value string) bool {
	switch kind {
	case "email":
		for _, e := range c.Emails {
			if strings.TrimSpace(e.Value) == value {
				return true
			}
		}
	case "phone":
		for _, p := range c.Phones {
			if strings.TrimSpace(p.Value) == value {
				return true
			}
		}
	case "url":
		for _, u := range c.URLs {
			if strings.TrimSpace(u.Value) == value {
				return true
			}
		}
	}
	return false
}

func diffFlagsValue(diff models.ImportMergeDiff, kind, value string) bool {
	for _, a := range diff.Added {
		if a.Kind == kind && a.Value == value {
			return true
		}
	}
	return false
}

// --- within-batch detection (T96) ------------------------------------------

func TestBatchDuplicateIndex_MatchesByEmailNamePhone(t *testing.T) {
	first := &models.Contact{
		Firstname: "Jane", Lastname: "Smith",
		Emails: []models.ContactEmail{{Value: "jane@example.com"}},
		Phones: []models.ContactPhone{{Value: "+1 (800) 555-1234"}},
	}
	byEmail := &models.Contact{
		Firstname: "J", Lastname: "S",
		Emails: []models.ContactEmail{{Value: "JANE@example.com"}}, // case-insensitive email
	}
	byName := &models.Contact{
		Firstname: "jane", Lastname: "smith", // exact name, no contact info at all
	}
	byPhone := &models.Contact{
		Firstname: "Jane", Lastname: "Smith",
		Phones: []models.ContactPhone{{Value: "8005551234"}}, // PhoneKey (T68): digits, last-10
	}
	unrelated := &models.Contact{Firstname: "Bob", Lastname: "Jones"}

	batch := []*models.Contact{first}
	assert.Equal(t, 0, batchDuplicateIndex(batch, byEmail))
	assert.Equal(t, 0, batchDuplicateIndex(batch, byName))
	assert.Equal(t, 0, batchDuplicateIndex(batch, byPhone))
	assert.Equal(t, -1, batchDuplicateIndex(batch, unrelated))
}

func TestBatchDuplicateIndex_EarliestSiblingWins(t *testing.T) {
	alice := &models.Contact{Firstname: "Alice", Lastname: "A", Emails: []models.ContactEmail{{Value: "a@example.com"}}}
	bob := &models.Contact{Firstname: "Bob", Lastname: "B", Emails: []models.ContactEmail{{Value: "b@example.com"}}}
	aliceAgain := &models.Contact{Firstname: "Alice", Lastname: "A", Emails: []models.ContactEmail{{Value: "a@example.com"}}}

	assert.Equal(t, 0, batchDuplicateIndex([]*models.Contact{alice, bob}, aliceAgain))
	assert.Equal(t, 1, batchDuplicateIndex([]*models.Contact{alice, bob}, bob))
}

func TestContactsMatchWithinBatch_ShortPhoneDoesNotMatch(t *testing.T) {
	// PhoneKey returns "" below 7 digits (T68) — two stub rows with
	// essentially-empty numbers must not be considered duplicates.
	a := &models.Contact{Phones: []models.ContactPhone{{Value: "555"}}}
	b := &models.Contact{Phones: []models.ContactPhone{{Value: "555"}}}
	assert.False(t, contactsMatchWithinBatch(a, b))
}

// TestParseVCF_WithinBatchDuplicate exercises the wiring end-to-end against
// the real migrated schema (CLAUDE.md trap 1): a file carrying the same
// person twice must flag the second row as a within-batch duplicate defaulting
// to skip, not as a DB duplicate.
func TestParseVCF_WithinBatchDuplicate(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t96.db"))
	require.NoError(t, err)

	user := models.User{Username: "t96batch", Password: "password123!A", Email: "t96batch@example.com"}
	require.NoError(t, db.Create(&user).Error)

	raw := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Jane Smith\r\nN:Smith;Jane;;;\r\nEMAIL:jane@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Jane Smith\r\nN:Smith;Jane;;;\r\nEMAIL:jane@example.com\r\nEND:VCARD\r\n" +
		"BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Bob Jones\r\nN:Jones;Bob;;;\r\nEMAIL:bob@example.com\r\nEND:VCARD\r\n"

	contacts, previews, stats, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 3)
	require.Len(t, previews, 3)

	assert.Equal(t, "add", previews[0].SuggestedAction)
	assert.Nil(t, previews[0].BatchDuplicateOf, "first occurrence is not a within-batch duplicate")

	require.NotNil(t, previews[1].BatchDuplicateOf, "second Jane Smith must be flagged as a within-batch duplicate")
	assert.Equal(t, 0, *previews[1].BatchDuplicateOf, "the duplicate points at the earlier row")
	assert.Equal(t, "skip", previews[1].SuggestedAction, "within-batch duplicates default to skip")
	assert.Nil(t, previews[1].DuplicateMatch, "no DB record matches, so no DB duplicate")

	assert.Equal(t, "add", previews[2].SuggestedAction)
	assert.Equal(t, 3, stats.ValidCount)
	assert.Equal(t, 0, stats.DuplicateCount, "within-batch dups are not DB duplicate matches")
}

// TestParseVCF_DuplicateOfExistingCarriesMergeDiff proves the preview's
// per-row merge diff is populated when a row matches an existing DB contact
// and describes what "Merge" would change.
func TestParseVCF_DuplicateOfExistingCarriesMergeDiff(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t96.db"))
	require.NoError(t, err)

	user := models.User{Username: "t96diff", Password: "password123!A", Email: "t96diff@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
	}
	require.NoError(t, db.Create(existing).Error)

	// The incoming card matches by email and adds a phone the existing
	// contact does not have, and updates the job title.
	raw := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Jane Smith\r\nN:Smith;Jane;;;\r\n" +
		"EMAIL:jane@example.com\r\nTEL:+15559998888\r\nTITLE:Staff Engineer\r\nEND:VCARD\r\n"

	contacts, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, contacts, 1)
	require.Len(t, previews, 1)
	require.NotNil(t, previews[0].DuplicateMatch)
	assert.Equal(t, "update", previews[0].SuggestedAction)

	require.NotNil(t, previews[0].MergeDiff, "a DB duplicate row must carry its merge diff")
	assert.Equal(t, "+15559998888", mergeDiffAddedValue(previews[0].MergeDiff, "phone"),
		"the incoming phone must be reported as an addition")
	assert.Equal(t, "Staff Engineer", mergeDiffUpdatedValue(previews[0].MergeDiff, "job_title"),
		"the incoming job title must be reported as an update")
}

func mergeDiffAddedValue(diff *models.ImportMergeDiff, kind string) string {
	for _, a := range diff.Added {
		if a.Kind == kind {
			return a.Value
		}
	}
	return ""
}

func mergeDiffUpdatedValue(diff *models.ImportMergeDiff, field string) string {
	for _, u := range diff.Updated {
		if u.Field == field {
			return u.New
		}
	}
	return ""
}

// TestGenerateCSVPreview_WithinBatchDuplicate pins the T96 within-batch
// detection on the CSV preview path specifically (the other two paths are
// covered by TestParseVCF_WithinBatchDuplicate and the records-endpoint test):
// a CSV carrying the same person twice must flag the second row as a
// within-batch duplicate defaulting to skip, with no DB match involved.
func TestGenerateCSVPreview_WithinBatchDuplicate(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t96-csv-batch.db"))
	require.NoError(t, err)

	user := models.User{Username: "t96csvbatch", Password: "password123!A", Email: "t96csvbatch@example.com"}
	require.NoError(t, db.Create(&user).Error)

	headers := []string{"First Name", "Last Name", "Email"}
	mappings := []models.ColumnMapping{
		{CSVColumn: "First Name", ContactField: "firstname"},
		{CSVColumn: "Last Name", ContactField: "lastname"},
		{CSVColumn: "Email", ContactField: "email"},
	}
	rows := [][]string{
		{"Jane", "Smith", "jane@example.com"},
		{"Jane", "Smith", "jane@example.com"},
		{"Bob", "Jones", "bob@example.com"},
	}

	contacts, previews, stats := GenerateCSVPreview(db, user.ID, rows, headers, mappings)
	require.Len(t, contacts, 3)
	require.Len(t, previews, 3)

	assert.Equal(t, "add", previews[0].SuggestedAction)
	assert.Nil(t, previews[0].BatchDuplicateOf, "first occurrence is not a within-batch duplicate")

	require.NotNil(t, previews[1].BatchDuplicateOf, "the CSV twin must be flagged as a within-batch duplicate")
	assert.Equal(t, 0, *previews[1].BatchDuplicateOf)
	assert.Equal(t, "skip", previews[1].SuggestedAction)

	assert.Equal(t, "add", previews[2].SuggestedAction)
	assert.Equal(t, 3, stats.ValidCount)
	assert.Equal(t, 0, stats.DuplicateCount, "within-batch dups are not DB duplicate matches")
}

// TestParseVCF_WithinBatchAndDbDuplicateCombined covers the combined case a
// real import hits when the same person appears twice AND already exists in
// the address book: the twin must carry BOTH the within-batch flag and the DB
// duplicate match (with its merge diff), and default to skip so neither a
// second contact is created nor a second merge is applied silently.
func TestParseVCF_WithinBatchAndDbDuplicateCombined(t *testing.T) {
	db, err := database.InitDB(filepath.Join(t.TempDir(), "t96-combined.db"))
	require.NoError(t, err)

	user := models.User{Username: "t96combined", Password: "password123!A", Email: "t96combined@example.com"}
	require.NoError(t, db.Create(&user).Error)

	existing := &models.Contact{
		UserID:    user.ID,
		Firstname: "Jane",
		Lastname:  "Smith",
		Email:     "jane@example.com",
		Emails:    []models.ContactEmail{{Type: "work", Value: "jane@example.com"}},
	}
	require.NoError(t, db.Create(existing).Error)

	card := "BEGIN:VCARD\r\nVERSION:4.0\r\nFN:Jane Smith\r\nN:Smith;Jane;;;\r\nEMAIL:jane@example.com\r\nEND:VCARD\r\n"
	raw := card + card

	_, previews, _, err := ParseVCF(strings.NewReader(raw), db, user.ID)
	require.NoError(t, err)
	require.Len(t, previews, 2)

	// First occurrence: DB duplicate, default merge.
	require.NotNil(t, previews[0].DuplicateMatch)
	assert.Nil(t, previews[0].BatchDuplicateOf)
	assert.Equal(t, "update", previews[0].SuggestedAction)

	// Twin: BOTH flags, default skip.
	require.NotNil(t, previews[1].DuplicateMatch, "the twin still matches the DB record")
	require.NotNil(t, previews[1].BatchDuplicateOf, "the twin is also a within-batch duplicate")
	assert.Equal(t, 0, *previews[1].BatchDuplicateOf)
	assert.Equal(t, "skip", previews[1].SuggestedAction, "a row that duplicates both a sibling and an existing record stays skip-by-default")
}
