package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"mycorrhizal/monica"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// monicaSnapshotRelPath is the snapshot fixture's repo-root-relative path.
const monicaSnapshotRelPath = "testdata/monica-fixture/snapshot.json"

func loadMonicaFixture(t *testing.T) *monica.Snapshot {
	t.Helper()
	path := findUp(t, monicaSnapshotRelPath)
	f, err := os.Open(path) // #nosec G304 -- checked-in fixture path, just resolved by findUp
	require.NoError(t, err)
	defer f.Close()
	snap, err := monica.LoadSnapshot(f)
	require.NoError(t, err)
	return snap
}

// findUp resolves a repo-root-relative path by walking up from the test
// process's working directory (works from backend/, backend/services/, or the
// repo root).
func findUp(t *testing.T, rel string) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		candidate := filepath.Join(dir, rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("fixture %s not found from %s", rel, dir) // # pragma: no cover
		}
		dir = parent
	}
}

// importMonicaFixture runs the full production pipeline — snapshot → map →
// execute into a real migrated schema.
func importMonicaFixture(t *testing.T, now time.Time) (*gorm.DB, models.User, *ImportReport, *ImportSourcePlan) {
	t.Helper()
	db := dbtest.New(t)
	user := createSourceImportUser(t, db)

	snap := loadMonicaFixture(t)
	plan := MapMonicaSnapshot(snap, now)
	report, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	return db, user, report, plan
}

func TestMonicaImport_FixtureLandsInRealMigratedSchema(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	db, user, report, _ := importMonicaFixture(t, now)

	// ada, ben, carol, grace import; the partial contact is skipped.
	assert.Equal(t, 4, report.ContactsCreated)

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&contacts).Error)
	require.Len(t, contacts, 4)

	byName := map[string]*models.Contact{}
	for i := range contacts {
		byName[contacts[i].Firstname] = &contacts[i]
	}

	ada := byName["Ada"]
	require.NotNil(t, ada)
	assert.Equal(t, "Lovelace", ada.Lastname)
	assert.Equal(t, "Enchantress", ada.Nickname)
	assert.Equal(t, "female", ada.Gender)
	assert.Equal(t, "1815-12-10", ada.Birthday)
	assert.Equal(t, "First Programmer", ada.JobTitle)
	assert.Equal(t, "Analytical Engine Co", ada.Organization)
	assert.True(t, ada.IsFavorite) // is_starred
	assert.Contains(t, ada.HowWeMet, "lecture")
	assert.Contains(t, ada.ContactInformation, "correspondent")
	assert.Len(t, ada.Emails, 1)
	assert.Len(t, ada.Phones, 1)
	assert.Len(t, ada.URLs, 1)
	assert.Len(t, ada.IMPPs, 1)
	assert.Len(t, ada.Addresses, 1)
	assert.Equal(t, "London", ada.Addresses[0].City)

	// The avatar is carried on the plan (deferred to the assistant), never
	// fetched by the backend mapping.
	var avatarNote bool
	for _, iss := range report.Issues {
		if iss.Field == "photo" && iss.Category == ImportIssueCategoryTransformed && iss.Record == "monica contact/1" {
			avatarNote = true
		}
	}
	assert.True(t, avatarNote, "ada's avatar must be reported as deferred, not silently dropped")

	ben := byName["Ben"]
	require.NotNil(t, ben)
	assert.Equal(t, "--04-20", ben.Birthday) // year-unknown birthday survives as a partial date

	carol := byName["Carol"]
	require.NotNil(t, carol)
	assert.Equal(t, "1848-01-15", carol.Birthday)
	// Deceased becomes a death anniversary.
	var deathAnniversary bool
	for _, a := range carol.Card.Anniversaries {
		if a.Kind == "death" {
			deathAnniversary = true
		}
	}
	assert.True(t, deathAnniversary, "Carol's death must land as a death anniversary")

	grace := byName["Grace"]
	require.NotNil(t, grace)

	// The partial contact is reported as skipped.
	var partialSkipped bool
	for _, iss := range report.Issues {
		if iss.Record == "monica contact/5" && iss.Category == ImportIssueCategorySkipped {
			partialSkipped = true
		}
	}
	assert.True(t, partialSkipped, "the partial contact must be reported as skipped")

	// --- Food preference -------------------------------------------------------
	var prefs []models.Preference
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&prefs).Error)
	require.Len(t, prefs, 1)
	assert.Equal(t, models.PreferenceCategoryFood, prefs[0].Category)
	assert.Contains(t, prefs[0].Value, "oysters")
	assert.Equal(t, ada.VCardUID, prefs[0].EntityID)

	// --- Relationships with direction preserved ---------------------------------
	var edges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&edges).Error)

	byType := map[string][]models.RelationshipEdge{}
	for _, e := range edges {
		byType[e.Type] = append(byType[e.Type], e)
	}
	// Spouse pair collapsed to exactly one.
	assert.Len(t, byType["spouse_of"], 1, "the reciprocal spouse pair must collapse to one edge")
	// {contact_is: Ada, of_contact: Carol, type: Daughter} means "Carol is
	// Ada's daughter" → edge source=carol, target=ada, child_of.
	require.Len(t, byType["child_of"], 1)
	child := byType["child_of"][0]
	assert.Equal(t, carol.VCardUID, child.SourceID)
	assert.Equal(t, ada.VCardUID, child.TargetID)
	// {contact_is: Grace, of_contact: Ada, type: Colleague} → "Ada is Grace's
	// colleague" → coworker_of between the two.
	require.Len(t, byType["coworker_of"], 1)
	coworker := byType["coworker_of"][0]
	pair := map[string]bool{coworker.SourceID: true, coworker.TargetID: true}
	assert.True(t, pair[grace.VCardUID] && pair[ada.VCardUID])

	// --- Notes -------------------------------------------------------------------
	var notes []models.Note
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&notes).Error)
	// 2 real notes + the task note + the debt note.
	assert.Len(t, notes, 4)
	var taskNote, debtNote bool
	for _, n := range notes {
		if n.Content == "Task: Buy analytical engine parts\nFor the next demonstration\n(completed)" {
			taskNote = true
		}
		if n.Content == "Debt: £12.50 (they owe me)\nDinner" {
			debtNote = true
		}
	}
	assert.True(t, taskNote, "tasks become dated notes")
	assert.True(t, debtNote, "debts become dated notes")

	// --- Activities: the real activity + the logged call ---------------------------
	var activities []models.Activity
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&activities).Error)
	require.Len(t, activities, 2)
	var callActivity bool
	for _, a := range activities {
		if a.Type == models.InteractionTypeCall {
			callActivity = true
			assert.Contains(t, a.Title, "difference engine")
		}
	}
	assert.True(t, callActivity, "a logged Monica call becomes a call-type activity")

	// --- Gifts ----------------------------------------------------------------------
	var gifts []models.Gift
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&gifts).Error)
	require.Len(t, gifts, 2)
	giftByStatus := map[string]bool{}
	for _, g := range gifts {
		giftByStatus[g.Status] = true
		assert.NotEmpty(t, g.Description)
	}
	assert.True(t, giftByStatus["idea"])
	assert.True(t, giftByStatus["given"], "Monica's 'offered' gift maps to the local 'given' status")

	// --- Reminders --------------------------------------------------------------------
	var reminders []models.Reminder
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&reminders).Error)
	// The birthday reminder and the one-time reminder. The past one-time
	// reminder (2023-03-01, before "now" 2026) is a dead row and is dropped.
	require.Len(t, reminders, 1)
	rem := reminders[0]
	assert.Equal(t, "yearly", rem.Recurrence)
	var owner models.Contact
	require.NoError(t, db.Where("user_id = ? AND id = ?", user.ID, *rem.ContactID).First(&owner).Error)
	assert.Equal(t, ada.VCardUID, owner.VCardUID)
	// A yearly reminder on the contact's birthday is a birthday reminder:
	// reoccur_from_completion is false so marking it done never walks the date
	// forward.
	require.NotNil(t, rem.ReoccurFromCompletion)
	assert.False(t, *rem.ReoccurFromCompletion)
}

func TestMonicaImport_CirclesFromTags(t *testing.T) {
	db, user, _, _ := importMonicaFixture(t, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	names := map[string]bool{}
	var circles []models.Circle
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&circles).Error)
	for _, c := range circles {
		names[c.Name] = true
	}
	assert.True(t, names["Family"])
	assert.True(t, names["Book Club"])
}

func TestMonicaImport_ReRunIsIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	db, user, _, plan := importMonicaFixture(t, now)

	second, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	assert.Equal(t, 0, second.ContactsCreated)
	assert.Equal(t, 4, second.ContactsSkipped)
	assert.Equal(t, 0, second.NotesCreated)
	assert.Equal(t, 0, second.RelationshipsCreated)

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&contacts).Error)
	require.Len(t, contacts, 4)
}

// TestMonicaImport_RelationshipDirectionPinned pins the direction mapping
// against Monica's verified semantics (from ApiRelationshipController +
// Contact::setRelationship): a row {contact_is: X, of_contact: Y, type: T}
// means "Y is X's T", so the edge is Y → X with the matched type. Inverting
// the mapping fails this test.
func TestMonicaImport_RelationshipDirectionPinned(t *testing.T) {
	snap := &monica.Snapshot{Relationships: map[int][]monica.Relationship{}}
	snap.Contacts = []monica.Contact{
		{ID: 1, FirstName: "Ada", LastName: "Lovelace"},
		{ID: 2, FirstName: "Carol", LastName: "Lovelace"},
	}
	// "Carol is Ada's daughter" (listed on Ada's page).
	snap.Relationships[1] = []monica.Relationship{{
		RelationshipType: struct {
			Name string `json:"name"`
		}{Name: "Daughter"},
		ContactIs: &monica.ContactRef{ID: 1, CompleteName: "Ada Lovelace"},
		OfContact: &monica.ContactRef{ID: 2, CompleteName: "Carol Lovelace"},
	}}

	plan := MapMonicaSnapshot(snap, time.Now())
	require.Len(t, plan.Relationships, 1)

	rel := plan.Relationships[0]
	assert.Equal(t, "contact/2", rel.Source.ExternalID, "the edge source must be Carol (of_contact)")
	assert.Equal(t, "contact/1", rel.Target.ExternalID, "the edge target must be Ada (contact_is)")
	assert.Equal(t, "child_of", rel.Type)
}

func TestMonicaImport_NamePromotionAndDeadReminder(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)

	// A contact with only a last name gets its last name promoted to a first
	// name; a nickname-only contact uses the nickname; a past one-time
	// reminder is dropped as a dead row.
	snap := &monica.Snapshot{Relationships: map[int][]monica.Relationship{}}
	snap.Contacts = []monica.Contact{
		{ID: 1, FirstName: "", LastName: "Only"},
		{ID: 2, FirstName: "", LastName: "", Nickname: "JustNick"},
		{ID: 3, FirstName: "Dead", IsDead: true}, // no deceased date at all
	}
	snap.Reminders = []monica.Reminder{{
		ID: 1, Title: "Past", FrequencyType: "one_time", FrequencyNumber: 1,
		InitialDate: strPtr("2020-01-01"), NextExpectedDate: strPtr("2020-01-01"),
		Contact: &monica.ContactRef{ID: 1},
	}}

	plan := MapMonicaSnapshot(snap, now)
	require.Len(t, plan.Contacts, 3)
	assert.Equal(t, "Only", plan.Contacts[0].Record.Card.Name.Components[0].Value)
	assert.Equal(t, "JustNick", plan.Contacts[1].Record.Card.Name.Components[0].Value)
	// A bare death marker with no date is reported, not guessed.
	var deadLoss bool
	for _, iss := range plan.Report.Issues {
		if iss.Record == "monica contact/3" && iss.Field == "is_dead" {
			deadLoss = true
		}
	}
	assert.True(t, deadLoss)
	assert.Empty(t, plan.Reminders, "a past one-time reminder is a dead row and is dropped")
}

func strPtr(s string) *string { return &s }

// TestMapMonicaSnapshot_EdgeCases covers the mapper's pathological and
// defensive branches with a synthetic snapshot: an activity with a bad date,
// an orphaned note, an activity with no resolvable attendees, a gift with an
// empty name, an unrecognized gift status, and a contact whose fields stay
// empty.
func TestMapMonicaSnapshot_EdgeCases(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	snap := &monica.Snapshot{Relationships: map[int][]monica.Relationship{}}
	snap.Contacts = []monica.Contact{{ID: 1, FirstName: "Ada", LastName: "Lovelace"}}

	// Activity with an unusable date → reported invalid.
	snap.Activities = []monica.Activity{{ID: 1, Summary: "Undated", HappenedAt: "not-a-date"}}
	// Activity with a date but no attendees → the attendee lookup drops it.
	snap.Activities = append(snap.Activities, monica.Activity{ID: 2, Summary: "Orphan", HappenedAt: "2023-06-15"})
	// Activity with only a type name as its title, and one with neither.
	snap.Activities = append(snap.Activities, monica.Activity{ID: 3, HappenedAt: "2023-06-15", ActivityType: &struct {
		Name string `json:"name"`
	}{Name: "Cultural"}})
	snap.Activities = append(snap.Activities, monica.Activity{ID: 4, Summary: "Both", HappenedAt: "2023-06-15", ActivityType: &struct {
		Name string `json:"name"`
	}{Name: "Cultural"}})
	// An activity with neither summary nor type (title falls back to
	// "Activity").
	snap.Activities = append(snap.Activities, monica.Activity{ID: 5, HappenedAt: "2023-06-15"})
	// Note with an empty body → dropped; note for an unknown contact → dropped;
	// note with an unusable created_at → falls back to now.
	snap.Notes = []monica.Note{
		{ID: 1, Body: "   ", CreatedAt: "2023-01-01T00:00:00Z", Contact: &monica.ContactRef{ID: 1}},
		{ID: 2, Body: "orphan", CreatedAt: "2023-01-01T00:00:00Z", Contact: &monica.ContactRef{ID: 99}},
		{ID: 3, Body: "now-ish", CreatedAt: "bogus", Contact: &monica.ContactRef{ID: 1}},
	}
	// Reminder with no contact → dropped; reminder for unknown contact.
	snap.Reminders = []monica.Reminder{
		{ID: 1, Title: "Orphan", FrequencyType: "year", Contact: nil},
		{ID: 2, Title: "Ghost", FrequencyType: "year", InitialDate: strPtr("2026-01-01"), Contact: &monica.ContactRef{ID: 99}},
	}
	// Call/task/debt/gift with no contact → dropped; call for unknown contact;
	// a long call (title truncated); a task with an unusable created_at.
	snap.Calls = []monica.Call{
		{ID: 1, Content: "hi", CalledAt: "2023-01-01T00:00:00Z", Contact: nil},
		{ID: 2, Content: "ghost", CalledAt: "2023-01-01T00:00:00Z", Contact: &monica.ContactRef{ID: 99}},
		{ID: 3, Content: repeatStr("long chat about the analytical engine and its many parts ", 30), CalledAt: "2023-01-01T00:00:00Z", Contact: &monica.ContactRef{ID: 1}},
		{ID: 4, Content: "bad-date", CalledAt: "bogus", Contact: &monica.ContactRef{ID: 1}},
	}
	snap.Tasks = []monica.Task{
		{ID: 1, Title: "t", Contact: nil},
		{ID: 2, Title: "ghost", Contact: &monica.ContactRef{ID: 99}},
		{ID: 3, Title: "", Contact: &monica.ContactRef{ID: 1}},
		{ID: 4, Title: "bad date", Contact: &monica.ContactRef{ID: 1}, CreatedAt: "bogus"},
	}
	snap.Debts = []monica.Debt{
		{ID: 1, Contact: nil},
		{ID: 2, Contact: &monica.ContactRef{ID: 99}},
		{ID: 3, Contact: &monica.ContactRef{ID: 1}, AmountWithCurrency: "", Amount: 5},
		{ID: 4, Contact: &monica.ContactRef{ID: 1}, InDebt: "yes", AmountWithCurrency: "£5", CreatedAt: "bogus"},
	}
	snap.Gifts = []monica.Gift{
		{ID: 1, Name: "", Contact: &monica.ContactRef{ID: 1}},
		{ID: 2, Name: "ghost", Contact: &monica.ContactRef{ID: 99}},
		{ID: 3, Name: "Orphan", Contact: nil},
		{ID: 4, Name: "Received", Contact: &monica.ContactRef{ID: 1}, HasBeenReceived: boolPtr(true)},
		{ID: 5, Name: "Offered", Contact: &monica.ContactRef{ID: 1}, HasBeenOffered: boolPtr(true)},
		{ID: 6, Name: "Priced", Amount: strPtr("£34.50"), Contact: &monica.ContactRef{ID: 1}},
	}
	// A contact with an empty tag.
	snap.Contacts = append(snap.Contacts, monica.Contact{ID: 2, FirstName: "Taggy", Tags: []monica.Tag{{Name: ""}, {Name: "Real"}}})
	// A non-binary contact, an unknown-gender-type contact, a contact with an
	// all-empty address, one with a blank contact field, and one with a
	// malformed birthdate.
	snap.Contacts = append(snap.Contacts, monica.Contact{ID: 3, FirstName: "Sam", GenderType: "O"})
	snap.Contacts = append(snap.Contacts, monica.Contact{ID: 4, FirstName: "Riley", Gender: "Mx", GenderType: "X"})
	snap.Contacts = append(snap.Contacts, monica.Contact{
		ID:            5,
		FirstName:     "Blanks",
		Addresses:     []monica.Address{{Name: "home"}}, // no components at all
		ContactFields: []monica.ContactField{{Content: "   "}},
	})
	badBirth := "2023-aa-aa"
	snap.Contacts = append(snap.Contacts, monicaContactWithBirth(6, "BadBirth", &badBirth))
	// A relationship for an unknown subject and an unrecognized type.
	snap.Relationships[99] = []monica.Relationship{{RelationshipType: struct {
		Name string `json:"name"`
	}{Name: "Soulmate"}, ContactIs: &monica.ContactRef{ID: 99}, OfContact: &monica.ContactRef{ID: 1}}}
	snap.Relationships[1] = []monica.Relationship{
		{RelationshipType: struct {
			Name string `json:"name"`
		}{Name: "Soulmate"}, ContactIs: &monica.ContactRef{ID: 1}, OfContact: &monica.ContactRef{ID: 2}},
		{RelationshipType: struct {
			Name string `json:"name"`
		}{Name: "Spouse"}, ContactIs: &monica.ContactRef{ID: 1}, OfContact: &monica.ContactRef{ID: 1}},
		{RelationshipType: struct {
			Name string `json:"name"`
		}{Name: "Friend"}, ContactIs: &monica.ContactRef{ID: 1}, OfContact: &monica.ContactRef{ID: 98}},
	}

	plan := MapMonicaSnapshot(snap, now)

	// The bad-date activity is reported, not created.
	var badDate bool
	for _, iss := range plan.Report.Issues {
		if iss.Record == "monica activity/1" && iss.Field == "activity.happened_at" {
			badDate = true
		}
	}
	assert.True(t, badDate)

	// Activities 3 and 4 map (type-name title / both) and the orphan keeps an
	// attendee-less entry for the engine to report.
	var typeTitled, bothTitled bool
	for _, a := range plan.Activities {
		switch a.Ref.ExternalID {
		case "activity/3":
			typeTitled = a.Title == "Cultural"
		case "activity/4":
			bothTitled = a.Title == "Both"
		}
	}
	assert.True(t, typeTitled)
	assert.True(t, bothTitled)

	// Notes: empty-body and unknown-contact notes are dropped; the bad-date
	// note falls back to now (note/3). Tasks and debts become notes too: the
	// empty-title task and the contact-less task/debt are dropped, the bad-date
	// task and the amount-only debt survive.
	require.Len(t, plan.Notes, 4)
	assert.Equal(t, "note/3", plan.Notes[0].Ref.ExternalID)
	assert.Equal(t, "now-ish", plan.Notes[0].Content)
	var emptyAmount bool
	for _, n := range plan.Notes {
		if n.Ref.ExternalID == "debt/3" {
			emptyAmount = strings.HasPrefix(n.Content, "Debt: 5.00")
		}
	}
	assert.True(t, emptyAmount, "a debt with an empty amount_with_currency falls back to the numeric amount")
	require.Len(t, plan.Reminders, 0, "orphan reminders are dropped")
	// Activities: the orphan (2), type-titled (3), both (4), the untitled (5),
	// the long call (call/3) and the bad-date call (call/4, dated now).
	require.Len(t, plan.Activities, 6)
	var longCall bool
	for _, a := range plan.Activities {
		if a.Ref.ExternalID == "call/3" {
			longCall = len(a.Title) <= 200
		}
	}
	assert.True(t, longCall, "a call title longer than 200 runes is truncated")
	// Gifts: received, offered, priced (idea default).
	require.Len(t, plan.Gifts, 3)
	statuses := map[string]bool{}
	for _, g := range plan.Gifts {
		statuses[g.Status] = true
		if g.Ref.ExternalID == "gift/6" {
			assert.Contains(t, g.Notes, "Amount: £34.50")
		}
	}
	assert.True(t, statuses[models.GiftStatusReceived])
	assert.True(t, statuses[models.GiftStatusGiven])
	assert.True(t, statuses[models.GiftStatusIdea])

	// The unrecognized "Soulmate" relationship (for the real contact) falls
	// back to related_to.
	require.Len(t, plan.Relationships, 1)
	assert.Equal(t, "related_to", plan.Relationships[0].Type)

	// Taggy's "Real" tag is a circle; the empty one is ignored.
	var taggyCircle bool
	for _, c := range plan.Circles {
		if c.Name == "Real" {
			taggyCircle = true
		}
	}
	assert.True(t, taggyCircle)

	// MapMonicaSnapshot(nil) is safe.
	assert.NotNil(t, MapMonicaSnapshot(nil, now))
}

func repeatStr(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

func boolPtr(b bool) *bool { return &b }

// monicaContactWithBirth builds a contact carrying a birthdate, avoiding the
// anonymous-struct Information literal in call sites.
func monicaContactWithBirth(id int, first string, birth *string) monica.Contact {
	c := monica.Contact{ID: id, FirstName: first}
	if birth != nil {
		c.Information.Dates.Birthdate = monica.SpecialDate{Date: birth}
	}
	return c
}

// Unit coverage for the small pure helpers.
func TestMonicaHelperFunctions(t *testing.T) {
	// mapMonicaGiftStatus's full table.
	assert.Equal(t, models.GiftStatusIdea, mapMonicaGiftStatus("idea"))
	assert.Equal(t, models.GiftStatusGiven, mapMonicaGiftStatus("offered"))
	assert.Equal(t, models.GiftStatusReceived, mapMonicaGiftStatus("received"))
	assert.Equal(t, models.GiftStatusPurchased, mapMonicaGiftStatus("purchased"))
	assert.Equal(t, models.GiftStatusIdea, mapMonicaGiftStatus("wrapped"))

	// isMonicaBirthdayReminder: only a yearly reminder on the birthday is one.
	assert.True(t, isMonicaBirthdayReminder("yearly", time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC), "12-10"))
	assert.False(t, isMonicaBirthdayReminder("yearly", time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC), ""))
	assert.False(t, isMonicaBirthdayReminder("monthly", time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC), "12-10"))

	// monicaTimeUsable.
	assert.True(t, monicaTimeUsable("2023-01-01T00:00:00Z"))
	assert.True(t, monicaTimeUsable("2023-01-01"))
	assert.False(t, monicaTimeUsable(""))

	// nextRecurrenceOnOrAfter: a future date is kept, a past recurring date
	// moves forward to its first occurrence on/after today.
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 10, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, future, nextRecurrenceOnOrAfter(future, "yearly", now))
	past := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	moved := nextRecurrenceOnOrAfter(past, "monthly", now)
	assert.True(t, moved.After(now) || moved.Equal(now))
	// A past "once" reminder keeps its date (the mapper drops it earlier).
	kept := nextRecurrenceOnOrAfter(past, "once", now)
	assert.Equal(t, past, kept)
	// Weekly advances by whole weeks and lands on/after today.
	weekly := nextRecurrenceOnOrAfter(past, "weekly", now)
	assert.True(t, weekly.After(now) || weekly.Equal(now))
	assert.Equal(t, 0, int(weekly.Sub(past))%7)
	// Quarterly and six-monthly advance by whole periods; yearly too (a past
	// yearly date must move to its next anniversary).
	for _, rec := range []string{"quarterly", "six-months", "yearly"} {
		moved := nextRecurrenceOnOrAfter(past, rec, now)
		assert.True(t, moved.After(now) || moved.Equal(now))
	}

	// mapMonicaSpecialDateToPartial: year-unknown, malformed, age-based.
	bad := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC).Format("2006-01-02T15:04:05Z")
	_ = bad
	short := "2023"
	assert.Nil(t, mapMonicaSpecialDateToPartial(monica.SpecialDate{Date: &short}))
	assert.Nil(t, mapMonicaSpecialDateToPartial(monica.SpecialDate{Date: nil}))
	assert.Nil(t, mapMonicaSpecialDateToPartial(monica.SpecialDate{IsAgeBased: true, Date: strPtr("2023-01-01T00:00:00Z")}))
	full := "2023-01-02T00:00:00Z"
	p := mapMonicaSpecialDateToPartial(monica.SpecialDate{Date: &full})
	require.NotNil(t, p)
	assert.Equal(t, 2023, *p.Year)
	noYear := "2023-01-02T00:00:00Z"
	py := mapMonicaSpecialDateToPartial(monica.SpecialDate{Date: &noYear, IsYearUnknown: true})
	require.NotNil(t, py)
	assert.Nil(t, py.Year)
	assert.Equal(t, 1, *py.Month)
}

// TestMapMonicaReminder covers the reminder mapper's folding directly: every
// frequency branch, the initial-date fallback, the unparseable-date and
// dead-one-time drops, and the empty-message drop.
func TestMapMonicaReminder(t *testing.T) {
	now := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	contact := &monica.ContactRef{ID: 1}

	cases := []struct {
		name      string
		rem       monica.Reminder
		wantRecur string
	}{
		{"once", monica.Reminder{ID: 1, Title: "t", FrequencyType: "one_time", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "once"},
		{"weekly", monica.Reminder{ID: 2, Title: "t", FrequencyType: "week", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "weekly"},
		{"monthly", monica.Reminder{ID: 3, Title: "t", FrequencyType: "month", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "monthly"},
		{"quarterly", monica.Reminder{ID: 4, Title: "t", FrequencyType: "month", FrequencyNumber: 3, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "quarterly"},
		{"six-months", monica.Reminder{ID: 5, Title: "t", FrequencyType: "month", FrequencyNumber: 6, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "six-months"},
		{"yearly", monica.Reminder{ID: 6, Title: "t", FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, "yearly"},
		{"initial-date fallback", monica.Reminder{ID: 7, Title: "t", FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: nil, InitialDate: strPtr("2030-01-01"), Contact: contact}, "yearly"},
		{"message from description", monica.Reminder{ID: 8, FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact, Description: "desc only"}, "yearly"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mapped, ok := mapMonicaReminder(tc.rem, now, "")
			require.True(t, ok)
			assert.Equal(t, tc.wantRecur, mapped.Recurrence)
			assert.NotEmpty(t, mapped.Message)
		})
	}

	// A past one-time reminder is a dead row.
	_, ok := mapMonicaReminder(monica.Reminder{ID: 9, Title: "past", FrequencyType: "one_time", FrequencyNumber: 1, NextExpectedDate: strPtr("2020-01-01"), Contact: contact}, now, "")
	assert.False(t, ok)

	// A nil contact, an unknown frequency, an unparseable date, and an empty
	// message all fail closed.
	_, ok = mapMonicaReminder(monica.Reminder{ID: 10, Title: "t", FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: nil}, now, "")
	assert.False(t, ok)
	_, ok = mapMonicaReminder(monica.Reminder{ID: 11, Title: "t", FrequencyType: "fortnight", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, now, "")
	assert.False(t, ok)
	_, ok = mapMonicaReminder(monica.Reminder{ID: 12, Title: "t", FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: strPtr("bogus"), Contact: contact}, now, "")
	assert.False(t, ok)
	_, ok = mapMonicaReminder(monica.Reminder{ID: 13, FrequencyType: "year", FrequencyNumber: 1, NextExpectedDate: strPtr("2030-01-01"), Contact: contact}, now, "")
	assert.False(t, ok)
}
