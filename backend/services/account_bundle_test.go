package services

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"testing"
	"time"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func bundleTestUser(t *testing.T, db *gorm.DB, username string) models.User {
	t.Helper()
	user := models.User{Username: username, Password: "password123!A", Email: username + "@example.com"}
	require.NoError(t, db.Create(&user).Error)
	return user
}

// bundleSeed is the set of stable identifiers a seeded account exposes, so a
// test can assert scoping and compare round-trips.
type bundleSeed struct {
	Contacts []models.Contact
}

// seedFullAccount creates one of every user-authored entity the account bundle
// covers, all tagged with marker so scoping assertions can spot them.
func seedFullAccount(t *testing.T, db *gorm.DB, user models.User, marker string) bundleSeed {
	t.Helper()

	mkContact := func(given string) models.Contact {
		c := models.Contact{UserID: user.ID, Firstname: given, Lastname: marker}
		require.NoError(t, db.Create(&c).Error)
		require.NotEmpty(t, c.VCardUID)
		return c
	}
	ada := mkContact("Ada" + marker)
	bob := mkContact("Bob" + marker)

	// A private and a suggested relationship edge: the bundle must carry both.
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: bob.VCardUID, TargetID: ada.VCardUID,
		Type: "friend_of", Directional: false, Source: "user-confirmed",
		Confidence: 1, Status: models.RelationshipStatusConfirmed,
		Sensitivity: models.RelationshipSensitivityPrivate,
	}).Error)
	require.NoError(t, db.Create(&models.RelationshipEdge{
		UserID: user.ID, SourceID: ada.VCardUID, TargetID: bob.VCardUID,
		Type: "coworker_of", Directional: false, Source: "suggested",
		Confidence: 0.4, Status: models.RelationshipStatusSuggested,
		Sensitivity: models.RelationshipSensitivityNormal,
	}).Error)

	require.NoError(t, db.Create(&models.Note{
		UserID: user.ID, ContactID: &ada.ID, Content: "note-" + marker, Date: time.Now().UTC().Truncate(time.Second),
	}).Error)

	lifeEvent := models.LifeEvent{
		UserID: user.ID, EntityID: ada.VCardUID, Type: "new_job", Category: "career",
		Date:        &contactmodel.PartialDate{Year: bundleIntPtr(2020), Month: bundleIntPtr(3), Day: bundleIntPtr(1)},
		Description: "life-" + marker, Source: "user",
	}
	require.NoError(t, db.Create(&lifeEvent).Error)

	reminder := models.Reminder{
		UserID: user.ID, ContactID: &ada.ID, Message: "reminder-" + marker,
		RemindAt: time.Now().UTC().Truncate(time.Second), Recurrence: "once",
		LifeEventID: &lifeEvent.ID,
	}
	require.NoError(t, db.Create(&reminder).Error)

	require.NoError(t, db.Create(&models.ReminderCompletion{
		UserID: user.ID, ReminderID: &reminder.ID, ContactID: ada.ID,
		Message: "completion-" + marker, CompletedAt: time.Now().UTC().Truncate(time.Second),
	}).Error)

	activity := models.Activity{
		UserID: user.ID, Title: "activity-" + marker, Description: "desc", Location: "here",
		Date: time.Now().UTC().Truncate(time.Second), Type: "meeting",
	}
	require.NoError(t, db.Create(&activity).Error)
	require.NoError(t, db.Model(&activity).Association("Contacts").Replace(&[]models.Contact{ada}))

	require.NoError(t, db.Create(&models.Gift{
		UserID: user.ID, EntityID: ada.VCardUID, Status: models.GiftStatusIdea,
		Description: "gift-" + marker, ValueCents: 1234, Currency: "EUR",
		LifeEventID: lifeEvent.ID, ActivityID: &activity.ID,
	}).Error)

	require.NoError(t, db.Create(&models.Preference{
		UserID: user.ID, EntityID: ada.VCardUID, Category: models.PreferenceCategoryFood,
		Value: "pref-" + marker, Source: models.PreferenceSourceUser,
		Sensitivity: models.RelationshipSensitivitySecret,
	}).Error)

	require.NoError(t, db.Create(&models.ConversationAgenda{
		UserID: user.ID, EntityID: ada.VCardUID, Content: "agenda-" + marker, ActivityID: &activity.ID,
	}).Error)

	require.NoError(t, db.Create(&models.CadencePolicy{
		UserID: user.ID, EntityID: ada.VCardUID, TargetIntervalDays: 30,
		QualifyingTypes: []string{"meeting"},
	}).Error)

	active := true
	require.NoError(t, db.Create(&models.DataDecayPolicy{
		UserID: user.ID, EntityID: ada.VCardUID, IntervalDays: 90, Active: active,
	}).Error)

	household := models.Household{UserID: user.ID, Name: "household-" + marker, Type: "family_unit"}
	require.NoError(t, db.Create(&household).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{
		HouseholdID: household.ID, UserID: user.ID, MemberVCardUID: ada.VCardUID, Role: "head",
	}).Error)

	circle := models.Circle{UserID: user.ID, Name: "circle-" + marker}
	require.NoError(t, db.Create(&circle).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: circle.ID, UserID: user.ID, MemberVCardUID: ada.VCardUID}).Error)

	tag := models.Tag{UserID: user.ID, Name: "tag-" + marker}
	require.NoError(t, db.Create(&tag).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tag.ID, UserID: user.ID, ContactVCardUID: ada.VCardUID}).Error)

	def := models.FieldDefinition{
		UserID: user.ID, Label: "Zodiac", Key: "zodiac_" + marker, Target: models.FieldDefinitionTargetContact,
		Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&def).Error)
	require.NoError(t, db.Create(&models.FieldValue{
		FieldDefinitionID: def.ID, UserID: user.ID, EntityID: ada.VCardUID,
		Value: json.RawMessage(`"Pisces"`),
	}).Error)

	require.NoError(t, db.Create(&models.OccasionObligation{
		UserID: user.ID, EntityID: ada.VCardUID, Kind: models.OccasionObligationKindCard,
		Label: "occasion-" + marker, LeadTimeDays: 7, Active: active,
		Sensitivity: models.RelationshipSensitivityNormal, LinkedLifeEventID: lifeEvent.ID,
	}).Error)

	event := models.OccasionEvent{
		UserID: user.ID, Title: "event-" + marker, StartsAt: time.Now().UTC().Truncate(time.Second),
		Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&event).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{
		UserID: user.ID, EventID: event.ID, EntityID: ada.VCardUID, RSVP: models.OccasionEventRSVPAccepted,
	}).Error)

	return bundleSeed{Contacts: []models.Contact{ada, bob}}
}

func bundleIntPtr(v int) *int { return &v }

func bundlePlanJSON(t *testing.T, b *models.AccountBundle) string {
	t.Helper()
	out, err := json.Marshal(b.Plan)
	require.NoError(t, err)
	return string(out)
}

// TestBuildAccountBundle_ScopesAndIncludesEverything seeds two users and
// asserts the bundle contains exactly the owner's rows — including
// private/secret and suggested ones — and that its collections marshall as
// arrays, never absent (frontend trap #8).
func TestBuildAccountBundle_ScopesAndIncludesEverything(t *testing.T) {
	db := dbtest.New(t)
	owner := bundleTestUser(t, db, "bundle-owner")
	other := bundleTestUser(t, db, "bundle-other")
	seedFullAccount(t, db, owner, "OWNER")
	seedFullAccount(t, db, other, "OTHER")

	bundle, stats, err := BuildAccountBundle(db, owner.ID, "")
	require.NoError(t, err)
	assert.Equal(t, models.AccountBundleFormat, bundle.Format)
	assert.Equal(t, models.AccountBundleVersion, bundle.Version)
	assert.Equal(t, 2, stats.Contacts)

	raw, err := json.Marshal(bundle)
	require.NoError(t, err)
	var decoded map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &decoded))
	var plan map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(decoded["plan"], &plan))
	for _, key := range []string{
		"contacts", "relationships", "notes", "reminders", "reminder_completions",
		"activities", "life_events", "gifts", "preferences", "conversation_agenda",
		"cadence_policies", "data_decay_policies", "households", "circles", "tags",
		"custom_field_definitions", "custom_field_values", "occasions", "occasion_events",
	} {
		require.Contains(t, plan, key, "plan.%s must always be present", key)
		assert.NotEqual(t, "null", string(plan[key]), "plan.%s must be [] never null", key)
	}

	// Owner's marker appears; the other user's never does.
	body := string(raw)
	assert.Contains(t, body, "OWNER")
	assert.NotContains(t, body, "OTHER")

	// Full-fidelity: a private and a suggested edge are both present.
	var priv, suggested int
	for _, e := range bundle.Plan.Relationships {
		switch e.Sensitivity {
		case models.RelationshipSensitivityPrivate:
			priv++
		}
		if e.Status == models.RelationshipStatusSuggested {
			suggested++
		}
	}
	assert.Equal(t, 1, priv, "private relationship edges must be included")
	assert.Equal(t, 1, suggested, "suggested edges must be included")

	// One of every entity type is present.
	assert.Len(t, bundle.Plan.Contacts, 2)
	assert.Len(t, bundle.Plan.Notes, 1)
	assert.Len(t, bundle.Plan.Reminders, 1)
	assert.Len(t, bundle.Plan.ReminderCompletions, 1)
	assert.Len(t, bundle.Plan.Activities, 1)
	assert.Len(t, bundle.Plan.LifeEvents, 1)
	assert.Len(t, bundle.Plan.Gifts, 1)
	assert.Len(t, bundle.Plan.Preferences, 1)
	assert.Len(t, bundle.Plan.ConversationAgenda, 1)
	assert.Len(t, bundle.Plan.CadencePolicies, 1)
	assert.Len(t, bundle.Plan.DataDecayPolicies, 1)
	assert.Len(t, bundle.Plan.Households, 1)
	assert.Len(t, bundle.Plan.Circles, 1)
	assert.Len(t, bundle.Plan.Tags, 1)
	assert.Len(t, bundle.Plan.CustomFieldDefinitions, 1)
	assert.Len(t, bundle.Plan.CustomFieldValues, 1)
	assert.Len(t, bundle.Plan.Occasions, 1)
	assert.Len(t, bundle.Plan.OccasionEvents, 1)
}

// TestAccountBundle_RoundTrip is the issue #1260 acceptance test: seed one of
// every entity for owner A, export, import into empty user B with all-add,
// re-export B, and assert the two plans are equal (stable IDs and all, modulo
// the timestamps/uint PKs the bundle never carries).
func TestAccountBundle_RoundTrip(t *testing.T) {
	// A bundle is imported into a *different instance* in production (a remote
	// server or a fresh local profile). UUID primary keys are globally unique,
	// so importing a bundle back into the same database would collide with the
	// source rows — the two databases model that reality.
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "bundle-rt-owner")
	target := bundleTestUser(t, targetDB, "bundle-rt-target")
	seedFullAccount(t, ownerDB, owner, "RT")

	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, "")
	require.NoError(t, err)

	plan := MapAccountBundle(exported)
	report, _, err := ExecuteSourceImportWithActions(
		t.Context(), targetDB, target.ID, plan, map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	require.Empty(t, report.Issues, "a clean round-trip import must report no issues")
	require.Equal(t, 2, report.ContactsCreated)

	reexported, _, err := BuildAccountBundle(targetDB, target.ID, "")
	require.NoError(t, err)

	require.JSONEq(t, bundlePlanJSON(t, exported), bundlePlanJSON(t, reexported),
		"the re-exported bundle must equal the original plan")
}

// TestAccountBundle_RoundTripDropsEntityType is the hand-verification the
// issue asks for: dropping one entity type from the import mapping makes the
// round-trip comparison fail. It proves the equal-plans assertion above is not
// vacuous.
func TestAccountBundle_RoundTripDropsEntityType(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "bundle-drop-owner")
	target := bundleTestUser(t, targetDB, "bundle-drop-target")
	seedFullAccount(t, ownerDB, owner, "DROP")

	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, "")
	require.NoError(t, err)

	plan := MapAccountBundle(exported)
	plan.LifeEvents = nil // simulate a mapper that forgot life events

	_, _, err = ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, plan, map[string]SourceContactAction{}, nil)
	require.NoError(t, err)

	reexported, _, err := BuildAccountBundle(targetDB, target.ID, "")
	require.NoError(t, err)

	assert.NotEqual(t, bundlePlanJSON(t, exported), bundlePlanJSON(t, reexported),
		"dropping life events from the mapping must change the re-exported plan")
}

// TestAccountBundle_ImportIsIdempotent re-runs the same import and asserts no
// new rows are produced (import_source_links ledger).
func TestAccountBundle_ImportIsIdempotent(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "bundle-idem-owner")
	target := bundleTestUser(t, targetDB, "bundle-idem-target")
	seedFullAccount(t, ownerDB, owner, "IDEM")

	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, "")
	require.NoError(t, err)
	plan := MapAccountBundle(exported)

	_, _, err = ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, plan, map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	first, _, err := BuildAccountBundle(targetDB, target.ID, "")
	require.NoError(t, err)

	// Re-run the exact same mapping.
	_, _, err = ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, MapAccountBundle(exported), map[string]SourceContactAction{}, nil)
	require.NoError(t, err)
	second, _, err := BuildAccountBundle(targetDB, target.ID, "")
	require.NoError(t, err)

	require.JSONEq(t, bundlePlanJSON(t, first), bundlePlanJSON(t, second),
		"re-running the same import must not create new rows")
}

// TestAccountBundle_ImportMergeAction covers the #1260 acceptance: a merge
// action on a contact whose vcard_uid already exists in the target follows the
// shared engine's existing merge semantics (rather than creating a duplicate).
func TestAccountBundle_ImportMergeAction(t *testing.T) {
	ownerDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, ownerDB, "bundle-merge-owner")
	target := bundleTestUser(t, targetDB, "bundle-merge-target")

	ada := models.Contact{UserID: owner.ID, Firstname: "Ada", Lastname: "Merge", HowWeMet: "at a conference"}
	require.NoError(t, ownerDB.Create(&ada).Error)
	bob := models.Contact{UserID: owner.ID, Firstname: "Bob", Lastname: "Merge"}
	require.NoError(t, ownerDB.Create(&bob).Error)

	// The target already holds a contact with Ada's stable UID, with an empty
	// HowWeMet the bundle should fill in.
	existing := models.Contact{UserID: target.ID, Firstname: "Ada", Lastname: "Merge", VCardUID: ada.VCardUID}
	require.NoError(t, targetDB.Create(&existing).Error)

	exported, _, err := BuildAccountBundle(ownerDB, owner.ID, "")
	require.NoError(t, err)
	plan := MapAccountBundle(exported)

	actions := map[string]SourceContactAction{
		"contact/" + ada.VCardUID: {Action: SourceActionMerge, MergeTargetUID: ada.VCardUID},
	}
	report, _, err := ExecuteSourceImportWithActions(t.Context(), targetDB, target.ID, plan, actions, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, report.ContactsUpdated, "Ada must be merged, not duplicated")
	assert.Equal(t, 1, report.ContactsCreated, "Bob is still a new contact")

	var contacts []models.Contact
	require.NoError(t, targetDB.Where("user_id = ?", target.ID).Find(&contacts).Error)
	assert.Len(t, contacts, 2, "the merge must not create a duplicate Ada")
	var merged models.Contact
	require.NoError(t, targetDB.Where("user_id = ? AND vcard_uid = ?", target.ID, ada.VCardUID).First(&merged).Error)
	assert.Equal(t, "at a conference", merged.HowWeMet, "the merged contact takes the bundle's value")
}

// TestAccountBundle_UploadRejectsWrongVersion covers the version gate (#1260).
func TestAccountBundle_UploadRejectsWrongVersion(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "bundle-ver")

	mgr := NewMycorrhizalImportManager()
	// A wrong-version bundle must be rejected before any session is created;
	// the check lives in Upload, which takes a multipart header. Exercise the
	// same guard through a minimal header.
	bundle := models.AccountBundle{Format: models.AccountBundleFormat, Version: 999}
	body, err := json.Marshal(bundle)
	require.NoError(t, err)

	header := bundleMultipartHeader(t, body)
	_, appErr := mgr.Upload(user.ID, header)
	require.NotNil(t, appErr, "an unsupported bundle version must be rejected")
	assert.Equal(t, 422, appErr.HTTPStatus)

	// Format mismatch likewise.
	body, err = json.Marshal(models.AccountBundle{Format: "not-a-bundle", Version: 1})
	require.NoError(t, err)
	_, appErr = mgr.Upload(user.ID, bundleMultipartHeader(t, body))
	require.NotNil(t, appErr)
	assert.Equal(t, 422, appErr.HTTPStatus)
}

// TestMycorrhizalImportManager_Lifecycle drives the session manager's own
// methods (upload → fetch → status → preview → confirm), covering the
// orchestration the controller handlers delegate to.
func TestMycorrhizalImportManager_Lifecycle(t *testing.T) {
	sourceDB := dbtest.New(t)
	targetDB := dbtest.New(t)
	owner := bundleTestUser(t, sourceDB, "mgr-owner")
	target := bundleTestUser(t, targetDB, "mgr-target")
	seedFullAccount(t, sourceDB, owner, "MGR")

	exported, _, err := BuildAccountBundle(sourceDB, owner.ID, "")
	require.NoError(t, err)
	data, err := json.Marshal(exported)
	require.NoError(t, err)

	mgr := NewMycorrhizalImportManager()
	log := zerolog.Nop()
	up, appErr := mgr.Upload(target.ID, bundleMultipartHeader(t, data))
	require.Nil(t, appErr)
	require.Equal(t, 1, mgr.CountActive(target.ID))

	require.Nil(t, mgr.StartFetch(targetDB, target.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
	waitForMycorrhizalPhase(t, mgr, target.ID, up.SessionID, models.SourceImportPhaseReady)

	preview, appErr := mgr.Preview(target.ID, up.SessionID)
	require.Nil(t, appErr)
	require.NotNil(t, preview)
	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}
	require.Nil(t, mgr.Confirm(targetDB, target.ID, models.SourceImportConfirmRequest{SessionID: up.SessionID, Actions: actions}, &log))
	status := waitForMycorrhizalPhase(t, mgr, target.ID, up.SessionID, models.SourceImportPhaseDone)
	require.NotNil(t, status.Result)
	assert.Equal(t, 2, status.Result.Created)

	// A second manager session cancelled while still connecting is dropped.
	up2, appErr := mgr.Upload(target.ID, bundleMultipartHeader(t, data))
	require.Nil(t, appErr)
	require.Nil(t, mgr.Cancel(target.ID, up2.SessionID))
	_, appErr = mgr.Status(target.ID, up2.SessionID)
	require.NotNil(t, appErr, "a cancelled connecting session is dropped")
}

func waitForMycorrhizalPhase(t *testing.T, mgr *MycorrhizalImportManager, userID uint, sid, want string) *models.SourceImportStatus {
	t.Helper()
	for i := 0; i < 400; i++ {
		st, appErr := mgr.Status(userID, sid)
		require.Nil(t, appErr)
		if st.Phase == want {
			return st
		}
		if st.Phase == models.SourceImportPhaseFailed {
			t.Fatalf("session failed: %s", st.Error)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for phase %q", want)
	return nil
}

// bundleMultipartHeader builds a real multipart.FileHeader for body, the way a
// router's FormFile would.
func bundleMultipartHeader(t *testing.T, body []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("file", "bundle.json")
	require.NoError(t, err)
	_, err = fw.Write(body)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	r := multipart.NewReader(&buf, w.Boundary())
	form, err := r.ReadForm(int64(buf.Len()) + 1024)
	require.NoError(t, err)
	t.Cleanup(func() { _ = form.RemoveAll() })
	return form.File["file"][0]
}
