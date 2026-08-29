package services

import (
	"path/filepath"
	"testing"

	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/meerkatfixture"
	"mycorrhizal/meerkat"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// buildMeerkatFixtureDB writes the checked-in meerkat manifest as a real
// Meerkat-schema SQLite file and reads it back through the production reader.
func buildMeerkatFixtureDB(t *testing.T) *meerkat.Snapshot {
	t.Helper()
	m, err := meerkatfixture.Read()
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "meerkat.db")
	require.NoError(t, meerkatfixture.Populate(path, m))

	snap, err := meerkat.Open(path)
	require.NoError(t, err)
	return snap
}

// importMeerkatFixture runs the full production pipeline — read → map →
// execute into a real migrated schema — and returns the db, local user,
// report, and plan.
func importMeerkatFixture(t *testing.T, requestedUser *int64) (*gorm.DB, models.User, *ImportReport, *ImportSourcePlan) {
	t.Helper()
	db := dbtest.New(t)
	user := createSourceImportUser(t, db)

	snap := buildMeerkatFixtureDB(t)
	plan := MapMeerkatSnapshot(snap, requestedUser)
	report, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	return db, user, report, plan
}

func TestMeerkatImport_FixtureLandsInRealMigratedSchema(t *testing.T) {
	db, user, report, _ := importMeerkatFixture(t, nil)

	// The fixture's live user-1 contacts (ada, ben, carol) import; the
	// soft-deleted contact and the second user's contact do not.
	assert.Equal(t, 3, report.ContactsCreated)

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&contacts).Error)
	require.Len(t, contacts, 3)

	byName := map[string]*models.Contact{}
	for i := range contacts {
		byName[contacts[i].Firstname] = &contacts[i]
	}

	// --- Ada: every mapped flat field lands -----------------------------------
	ada := byName["Ada"]
	require.NotNil(t, ada)
	assert.Equal(t, "Lovelace", ada.Lastname)
	assert.Equal(t, "Żółć", ada.Nickname) // unicode survives
	assert.Equal(t, "Countess", ada.Prefix)
	assert.Equal(t, "Augusta", ada.MiddleName)
	assert.Equal(t, "female", ada.Gender)
	assert.Equal(t, "1815-12-10", ada.Birthday)
	assert.Equal(t, "1835-07-08", ada.Anniversary)
	assert.Equal(t, "Analytical Engine Co", ada.Organization)
	assert.Equal(t, "Mathematics", ada.Department)
	assert.Equal(t, "First Programmer", ada.JobTitle)
	assert.Equal(t, "Analyst", ada.Role)
	assert.Equal(t, "11111111-1111-1111-1111-111111111111", ada.VCardUID)
	assert.Equal(t, "Ada", ada.Firstname)
	assert.False(t, ada.Archived)

	// Multi-valued entries.
	assert.Len(t, ada.Emails, 2)
	assert.Equal(t, "ada@example.com", ada.Emails[0].Value)
	assert.Equal(t, "ada.lovelace@analytical.example", ada.Emails[1].Value)
	assert.Len(t, ada.Phones, 2)
	assert.Len(t, ada.Addresses, 1)
	assert.Equal(t, "12 Byron St", ada.Addresses[0].Street)
	assert.Equal(t, "London", ada.Addresses[0].City)
	assert.Equal(t, "England", ada.Addresses[0].Region)
	assert.Equal(t, "SW1A 1AA", ada.Addresses[0].Postal)
	assert.Equal(t, "United Kingdom", ada.Addresses[0].Country)
	assert.Len(t, ada.URLs, 1)
	assert.Len(t, ada.IMPPs, 1)

	// The very long how_we_met survived verbatim (no truncation; ADR-0002
	// preserve, don't reject).
	assert.Greater(t, len(ada.HowWeMet), 5000)
	assert.Contains(t, ada.HowWeMet, "analytical engine demonstration")

	// The neutral Card is the authoritative full-fidelity copy (trap #2/#3).
	require.NotNil(t, ada.Card.Name)
	assert.Equal(t, "Lovelace", ada.Card.Name.Components[len(ada.Card.Name.Components)-1].Value)

	// --- Ben: year-unknown birthday --------------------------------------------
	ben := byName["Ben"]
	require.NotNil(t, ben)
	assert.Equal(t, "--04-20", ben.Birthday)

	// --- Carol -----------------------------------------------------------------
	carol := byName["Carol"]
	require.NotNil(t, carol)
	assert.True(t, carol.Archived, "the fixture's archived contact keeps its archive flag")

	// --- The soft-deleted contact and the other user's contact are absent ------
	assert.NotContains(t, byName, "Removed")
	assert.NotContains(t, byName, "Zoe")

	// --- Relationships with direction preserved ---------------------------------
	var edges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&edges).Error)

	byType := map[string]models.RelationshipEdge{}
	for _, e := range edges {
		byType[e.Type] = e
	}
	// The reciprocal spouse pair collapsed to one edge.
	assert.Len(t, edges, 4, "spouse pair collapses to one, plus daughter, mentor, and the related_to fallback")

	// {contact: ada, related: carol, type: Daughter} → "Carol is Ada's
	// daughter" → edge source=carol, target=ada, child_of.
	child, ok := byType["child_of"]
	require.True(t, ok, "expected a child_of edge")
	assert.Equal(t, carol.VCardUID, child.SourceID)
	assert.Equal(t, ada.VCardUID, child.TargetID)

	// {contact: carol, related: ada, type: Mentor} → "Ada is Carol's mentor"
	// → edge source=ada, target=carol, mentor_of.
	mentor, ok := byType["mentor_of"]
	require.True(t, ok, "expected a mentor_of edge")
	assert.Equal(t, ada.VCardUID, mentor.SourceID)
	assert.Equal(t, carol.VCardUID, mentor.TargetID)

	// The two spouse rows collapsed to exactly one spouse_of edge between
	// ada and ben (the graph derives the inverse, so both would double-render).
	spouse, ok := byType["spouse_of"]
	require.True(t, ok, "expected a spouse_of edge")
	pair := map[string]bool{spouse.SourceID: true, spouse.TargetID: true}
	assert.True(t, pair[ada.VCardUID] && pair[ben.VCardUID])

	// The unrecognized "Bff" type fell back to related_to (ben → ada).
	fallback, ok := byType["related_to"]
	require.True(t, ok, "expected the related_to fallback edge")
	assert.Equal(t, ben.VCardUID, fallback.SourceID)
	assert.Equal(t, ada.VCardUID, fallback.TargetID)

	// --- Circles (from the contacts' circles column) ----------------------------
	var circles []models.Circle
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&circles).Error)
	assert.Len(t, circles, 2)
	names := map[string]bool{}
	for _, c := range circles {
		names[c.Name] = true
	}
	assert.True(t, names["Family"])
	assert.True(t, names["Book Club"])
	var familyMembers []models.CircleMember
	require.NoError(t, db.Where("circle_id = ?", circleIDFor(t, db, user.ID, "Family")).Find(&familyMembers).Error)
	assert.Len(t, familyMembers, 2)

	// --- Custom fields (field_definitions + field_values) ------------------------
	var defs []models.FieldDefinition
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&defs).Error)
	assert.Len(t, defs, 2)
	var values []models.FieldValue
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&values).Error)
	assert.Len(t, values, 2)

	// --- Food preference ----------------------------------------------------------
	var prefs []models.Preference
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&prefs).Error)
	require.Len(t, prefs, 1)
	assert.Equal(t, models.PreferenceCategoryFood, prefs[0].Category)
	assert.Contains(t, prefs[0].Value, "oysters")

	// --- Notes ---------------------------------------------------------------------
	var notes []models.Note
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&notes).Error)
	assert.Len(t, notes, 2)
	assert.Contains(t, notes[0].Content, "Σωκράτης")

	// --- Activities ------------------------------------------------------------------
	var activities []models.Activity
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&activities).Error)
	require.Len(t, activities, 1)
	assert.Equal(t, "Lecture at the Royal Institution", activities[0].Title)
	// Attendees live in the join table (the many2many association is not
	// auto-preloaded on a plain Find).
	var links []struct {
		ActivityID uint
		ContactID  uint
	}
	require.NoError(t, db.Table("activity_contacts").
		Joins("JOIN contacts ON contacts.id = activity_contacts.contact_id").
		Where("activity_contacts.activity_id = ? AND contacts.user_id = ?", activities[0].ID, user.ID).
		Select("activity_contacts.activity_id, activity_contacts.contact_id").
		Scan(&links).Error)
	assert.Len(t, links, 2)

	// --- Reminders --------------------------------------------------------------------
	var reminders []models.Reminder
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&reminders).Error)
	require.Len(t, reminders, 2)
}

func circleIDFor(t *testing.T, db *gorm.DB, userID uint, name string) string {
	t.Helper()
	var c models.Circle
	require.NoError(t, db.Where("user_id = ? AND name = ?", userID, name).First(&c).Error)
	return c.ID
}

func TestMeerkatImport_LossesAreNamedNotSilent(t *testing.T) {
	_, _, report, _ := importMeerkatFixture(t, nil)

	// The dangling relationship (target person not a contact) is reported.
	var dangling bool
	for _, iss := range report.Issues {
		if iss.Record == "meerkat relationship/4" && iss.Field == "relationship.target" && iss.Category == ImportIssueCategoryUnsupported {
			dangling = true
		}
	}
	assert.True(t, dangling, "the dangling relationship must be reported with its field and category")

	// The deleted contact is reported as a policy skip, not silently dropped.
	var deleted bool
	for _, iss := range report.Issues {
		if iss.Record == "meerkat contact/4" && iss.Category == ImportIssueCategorySkipped {
			deleted = true
		}
	}
	assert.True(t, deleted, "the soft-deleted contact must be reported as skipped")

	// The deleted note is reported too.
	var deletedNote bool
	for _, iss := range report.Issues {
		if iss.Record == "meerkat note/3" && iss.Category == ImportIssueCategorySkipped {
			deletedNote = true
		}
	}
	assert.True(t, deletedNote, "the soft-deleted note must be reported as skipped")

	// Photos are not importable from the DB file (they live on the source
	// server's filesystem) — named, never silent. Ada carries a photo in the
	// fixture, so exactly one photo loss is reported.
	var photoLosses []ImportIssue
	for _, iss := range report.Issues {
		if iss.Field == "photo" && iss.Category == ImportIssueCategoryUnsupported {
			photoLosses = append(photoLosses, iss)
		}
	}
	require.Len(t, photoLosses, 1, "ada's photo must be reported as an unmappable field")
	assert.Contains(t, photoLosses[0].Message, "filesystem")
}

func TestMeerkatImport_SecondUserNotMixedIn(t *testing.T) {
	_, _, report, plan := importMeerkatFixture(t, nil)

	// The second source user's contact (id 5, user 2) must not be imported:
	// the default import targets the first source user only.
	assert.Len(t, plan.Contacts, 3)
	assert.Equal(t, 3, report.ContactsCreated)
}

func TestMeerkatImport_ExplicitUserFilter(t *testing.T) {
	db := dbtest.New(t)
	user := createSourceImportUser(t, db)
	snap := buildMeerkatFixtureDB(t)

	// Requesting source user 2 imports exactly that user's rows.
	userTwo := int64(2)
	plan := MapMeerkatSnapshot(snap, &userTwo)
	require.Len(t, plan.Contacts, 1)
	assert.Equal(t, "Zoe", plan.Contacts[0].Record.Card.Name.Components[0].Value)

	report, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	assert.Equal(t, 1, report.ContactsCreated)
}

func TestMeerkatImport_ReRunIsIdempotent(t *testing.T) {
	db, user, _, plan := importMeerkatFixture(t, nil)

	second, err := ExecuteSourceImport(db, user.ID, plan)
	require.NoError(t, err)
	assert.Equal(t, 0, second.ContactsCreated)
	assert.Equal(t, 3, second.ContactsSkipped)
	assert.Equal(t, 0, second.NotesCreated)
	assert.Equal(t, 0, second.RelationshipsCreated)

	var contacts []models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&contacts).Error)
	require.Len(t, contacts, 3)
}

// TestMeerkatImport_RelationshipDirectionPinned is the hand-verify pin from
// issue #353 ("invert one relationship edge in the mapping and confirm a
// fixture test catches it"): the mapping must produce the edge exactly as the
// legacy row describes, and any inversion fails this test by construction.
func TestMeerkatImport_RelationshipDirectionPinned(t *testing.T) {
	db, user, _, _ := importMeerkatFixture(t, nil)

	var edges []models.RelationshipEdge
	require.NoError(t, db.Where("user_id = ?", user.ID).Find(&edges).Error)

	for _, e := range edges {
		switch e.Type {
		case "child_of":
			// "Daughter" on Ada's page means Carol is Ada's child: the edge
			// MUST be carol → ada, never ada → carol.
			assert.NotEqual(t, "11111111-1111-1111-1111-111111111111", e.SourceID, "child_of edge source must not be Ada")
			assert.Equal(t, "11111111-1111-1111-1111-111111111111", e.TargetID, "child_of edge target must be Ada")
		case "mentor_of":
			// "Mentor" on Carol's page means Ada is Carol's mentor: source=Ada.
			assert.Equal(t, "11111111-1111-1111-1111-111111111111", e.SourceID, "mentor_of edge source must be Ada")
			assert.Equal(t, "33333333-3333-3333-3333-333333333333", e.TargetID, "mentor_of edge target must be Carol")
		}
	}
}

func TestMeerkatSourceUser_DefaultsToSingleSourceUser(t *testing.T) {
	snap := &meerkat.Snapshot{}
	id := int64(7)
	snap.SourceUserID = &id
	snap.SourceUserCount = 1

	got := MeerkatSourceUser(snap, nil)
	require.NotNil(t, got)
	assert.Equal(t, id, *got)

	// Multiple users: default to the first source user, never import-all (a
	// second user's data must not be silently mixed into one local account).
	snap.SourceUserCount = 2
	got = MeerkatSourceUser(snap, nil)
	require.NotNil(t, got)
	assert.Equal(t, id, *got)

	// No user scoping at all (pre-000004 database): nil means "every row
	// belongs to the importing user".
	assert.Nil(t, MeerkatSourceUser(&meerkat.Snapshot{}, nil))

	// An explicit request always wins.
	want := int64(9)
	got = MeerkatSourceUser(snap, &want)
	require.NotNil(t, got)
	assert.Equal(t, want, *got)

	// Nil snapshot is safe.
	assert.Nil(t, MeerkatSourceUser(nil, nil))
}

// MapMeerkatSnapshot_EdgeCases covers the mapper's pathological and
// defensive branches with synthetic snapshots (nil columns, malformed JSON,
// foreign-user rows, empty content) that the checked-in fixture does not
// carry.
func TestMapMeerkatSnapshot_EdgeCases(t *testing.T) {
	user := int64(1)
	other := int64(2)
	str := func(s string) *string { return &s }

	snap := &meerkat.Snapshot{}
	snap.SourceUserID = &user
	snap.SourceUserCount = 2

	// A contact with NULL columns and malformed JSON blobs (each parser must
	// degrade to nothing, never panic).
	snap.Contacts = append(snap.Contacts, meerkat.Contact{
		ID: 1, UserID: &user,
		Firstname:     str("Ada"),
		Lastname:      nil, // NULL lastname must not panic the mapping
		Birthday:      str("not-a-date"),
		Anniversary:   str("--xx-yy"),
		CirclesJSON:   str(`{"not":"an array"}`),
		CustomFields:  str(`{"empty":"","real":"value"}`),
		EmailsJSON:    str(`[{"broken":`),
		PhonesJSON:    str(`not json`),
		URLsJSON:      str(`[]`),
		IMPPsJSON:     str(`null`),
		AddressesJSON: str(`[{"type":"home","street":"","city":"","region":"","postal":"","country":""}]`),
		VCardExtra:    str(`{"properties":{"X-NOTE":[{"Value":"hi","Params":{"PREF":["1"]},"Group":"item1"}]}}`),
	})
	// A contact with only a scalar address (no structured addresses[]).
	snap.Contacts = append(snap.Contacts, meerkat.Contact{
		ID: 2, UserID: &user, Firstname: str("Ben"), Lastname: str("B"),
		Address: str("1 Main St"),
	})
	// A contact with fully malformed JSON blobs and a bad full-date birthday.
	snap.Contacts = append(snap.Contacts, meerkat.Contact{
		ID: 3, UserID: &user, Firstname: str("Malformed"),
		CustomFields:  str(`not json`),
		URLsJSON:      str(`{`),
		IMPPsJSON:     str(`[{"type":`),
		AddressesJSON: str(`not json`),
		Birthday:      str("2023-aa-aa"),
		VCardExtra:    str(`not json`),
	})
	// A second user's contact.
	snap.Contacts = append(snap.Contacts, meerkat.Contact{ID: 4, UserID: &other, Firstname: str("Zoe")})

	// A dangling relationship, a deleted one, one owned by the other user, and
	// one whose contact_id points at no row in the selected user's set.
	snap.Relationships = []meerkat.Relationship{
		{ID: 1, UserID: &user, ContactID: int64Ptr(1), RelatedContact: nil, Name: str("Grace Hopper"), Type: str("Friend")},
		{ID: 2, UserID: &user, DeletedAt: str("2024-01-01 00:00:00"), ContactID: int64Ptr(1), RelatedContact: int64Ptr(2), Type: str("Friend")},
		{ID: 3, UserID: &other, ContactID: int64Ptr(4), RelatedContact: int64Ptr(1), Type: str("Friend")},
		{ID: 4, UserID: &user, ContactID: int64Ptr(99), RelatedContact: int64Ptr(1), Type: str("Friend")},
	}

	// An empty note, a deleted note, a second-user note, and a note whose
	// contact is not in the selected user's set.
	snap.Notes = []meerkat.Note{
		{ID: 1, UserID: &user, ContactID: int64Ptr(1), Content: str("   "), Date: str("2023-01-01 00:00:00")},
		{ID: 2, UserID: &user, DeletedAt: str("2024-01-01 00:00:00"), ContactID: int64Ptr(1), Content: str("gone")},
		{ID: 3, UserID: &other, ContactID: int64Ptr(4), Content: str("other user")},
		{ID: 4, UserID: &user, ContactID: int64Ptr(99), Content: str("ghost contact")},
		{ID: 5, UserID: &user, ContactID: int64Ptr(1), Content: str("undated"), Date: nil},
	}

	// A deleted activity, an activity with no attendees.
	snap.Activities = []meerkat.Activity{
		{ID: 1, UserID: &user, DeletedAt: str("2024-01-01 00:00:00"), Title: str("gone")},
		{ID: 2, UserID: &user, Title: str("solo"), Date: str("2023-01-02 00:00:00")},
	}

	// A deleted reminder, a bad-recurrence reminder, a second-user reminder,
	// and one whose contact is not in the selected user's set.
	snap.Reminders = []meerkat.Reminder{
		{ID: 1, UserID: &user, DeletedAt: str("2024-01-01 00:00:00"), ContactID: int64Ptr(1), Message: str("gone")},
		{ID: 2, UserID: &user, ContactID: int64Ptr(1), Message: str("weird"), RemindAt: str("2023-01-03 00:00:00"), Recurrence: str("fortnightly")},
		{ID: 3, UserID: &other, ContactID: int64Ptr(4), Message: str("other user")},
		{ID: 4, UserID: &user, ContactID: int64Ptr(99), Message: str("ghost contact")},
	}

	plan := MapMeerkatSnapshot(snap, nil)

	// The three user-1 live contacts are mapped (Zoe and all second-user rows
	// are filtered; the NULL-lastname contact still maps).
	require.Len(t, plan.Contacts, 3)
	assert.Equal(t, "1 Main St", plan.Contacts[1].Record.Card.Addresses[0].Full)
	// The malformed JSON blobs degraded to nothing; the scalar-address
	// fallback did not fire for Ada (she has a structured-address array with
	// only blank entries, which is dropped, and no scalar address).
	assert.Empty(t, plan.Contacts[0].Record.Card.Addresses)
	// The vcard_extra passthrough survived.
	require.NotEmpty(t, plan.Contacts[0].Record.Passthrough.VCard)
	assert.Equal(t, "X-NOTE", plan.Contacts[0].Record.Passthrough.VCard[0].Name)
	// Only the non-empty custom field is mapped.
	require.Len(t, plan.CustomFields, 1)
	assert.Equal(t, "real", plan.CustomFields[0].Key)

	// The dangling relationship is reported; the deleted, foreign-user, and
	// ghost-contact relationships are not mapped as edges.
	assert.Len(t, plan.Relationships, 0)
	var dangling bool
	for _, iss := range plan.Report.Issues {
		if iss.Record == "meerkat relationship/1" && iss.Category == ImportIssueCategoryUnsupported {
			dangling = true
		}
	}
	assert.True(t, dangling)

	// Deleted rows are reported as skipped (relationship/2, note/2,
	// activity/1, reminder/1).
	var skipped int
	for _, iss := range plan.Report.Issues {
		if iss.Category == ImportIssueCategorySkipped {
			skipped++
		}
	}
	assert.Equal(t, 4, skipped, "one deleted row per entity kind reported")

	// The bad recurrence is reported and the reminder dropped.
	assert.Empty(t, plan.Reminders)
	var badRec bool
	for _, iss := range plan.Report.Issues {
		if iss.Record == "meerkat reminder/2" && iss.Field == "reminder.recurrence" {
			badRec = true
		}
	}
	assert.True(t, badRec)

	// MapMeerkatSnapshot(nil) is safe.
	empty := MapMeerkatSnapshot(nil, nil)
	assert.NotNil(t, empty)
	assert.Empty(t, empty.Contacts)

	// A snapshot with no user scoping at all imports every row (the
	// pre-000004 filter=nil branch).
	noScope := &meerkat.Snapshot{}
	noScope.Contacts = []meerkat.Contact{{ID: 1, Firstname: str("No")}, {ID: 2, Firstname: str("Scope")}}
	noScopePlan := MapMeerkatSnapshot(noScope, nil)
	require.Len(t, noScopePlan.Contacts, 2)
}

func int64Ptr(v int64) *int64 { return &v }

// collapseReciprocalCandidates unit coverage for the branches the fixture
// does not reach: a single-edge pair is untouched, and the lower-source-id
// survivor rule is deterministic for both iteration orders.
func TestCollapseReciprocalCandidates(t *testing.T) {
	single := []meerkatEdgeCandidate{{edgeID: 1, source: 2, target: 1, typ: "mentor_of"}}
	assert.Len(t, collapseReciprocalCandidates(single), 1, "a single-edge pair is never collapsed")

	// A true reciprocal pair with the higher-source-id edge listed first.
	pair := []meerkatEdgeCandidate{
		{edgeID: 1, source: 5, target: 3, typ: "parent_of"},
		{edgeID: 2, source: 3, target: 5, typ: "child_of"},
	}
	out := collapseReciprocalCandidates(pair)
	require.Len(t, out, 1, "a reciprocal pair collapses to one edge")
	assert.Equal(t, int64(3), out[0].source, "the lower-source-id half survives")

	// Same pair with the other order — the result is identical.
	reversed := []meerkatEdgeCandidate{pair[1], pair[0]}
	out = collapseReciprocalCandidates(reversed)
	require.Len(t, out, 1)
	assert.Equal(t, int64(3), out[0].source)

	// Two edges of unrelated types between the same pair are both kept.
	unrelated := []meerkatEdgeCandidate{
		{edgeID: 1, source: 1, target: 2, typ: "spouse_of"},
		{edgeID: 2, source: 2, target: 1, typ: "related_to"},
	}
	assert.Len(t, collapseReciprocalCandidates(unrelated), 2)

	// Two candidates that form two separate single-edge pairs (the group-loop
	// len<2 branch): neither is collapsed.
	separate := []meerkatEdgeCandidate{
		{edgeID: 1, source: 1, target: 2, typ: "friend_of"},
		{edgeID: 2, source: 3, target: 4, typ: "friend_of"},
	}
	assert.Len(t, collapseReciprocalCandidates(separate), 2)

	// Fewer than two candidates short-circuits.
	assert.Empty(t, collapseReciprocalCandidates(nil))
}
