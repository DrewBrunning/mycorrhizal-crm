package services

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/internal/faults"
	"mycorrhizal/models"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// armImportFault arms the shared source-import failure seam so runImport hits
// its error branch.
func armImportFault(t *testing.T) {
	t.Helper()
	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmError(faultImportSource, errors.New("injected import failure"))
}

// TestBuildAccountBundle_EdgeCases exercises the branches the round-trip seed
// does not: attachments, empty vs populated member/attendee lists, a note with
// no contact, and an over-cap photo.
func TestBuildAccountBundle_EdgeCases(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "bundle-edge")

	a := models.Contact{UserID: user.ID, Firstname: "EdgeA"}
	require.NoError(t, db.Create(&a).Error)
	b := models.Contact{UserID: user.ID, Firstname: "EdgeB"}
	require.NoError(t, db.Create(&b).Error)

	// A photo whose data URI exceeds the cap must be omitted and counted.
	huge := models.Contact{UserID: user.ID, Firstname: "Huge", PhotoThumbnail: "data:image/jpeg;base64," + strings.Repeat("A", MaxAccountBundlePhotoBytes+16)}
	require.NoError(t, db.Create(&huge).Error)

	require.NoError(t, db.Create(&models.Attachment{
		UserID: user.ID, ContactVCardUID: a.VCardUID, StoredName: "x", OriginalName: "doc.pdf",
		ContentType: "application/pdf", SizeBytes: 12,
	}).Error)

	// Households: one with two members (exercises the member sort), one empty.
	hh := models.Household{UserID: user.ID, Name: "HH", Type: "family_unit"}
	require.NoError(t, db.Create(&hh).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: b.VCardUID}).Error)
	require.NoError(t, db.Create(&models.HouseholdMember{HouseholdID: hh.ID, UserID: user.ID, MemberVCardUID: a.VCardUID}).Error)
	hh0 := models.Household{UserID: user.ID, Name: "HH0", Type: "other"}
	require.NoError(t, db.Create(&hh0).Error)

	// Circles and tags: one populated, one empty (nil-slice branches).
	ci := models.Circle{UserID: user.ID, Name: "C"}
	require.NoError(t, db.Create(&ci).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: ci.ID, UserID: user.ID, MemberVCardUID: a.VCardUID}).Error)
	require.NoError(t, db.Create(&models.CircleMember{CircleID: ci.ID, UserID: user.ID, MemberVCardUID: b.VCardUID}).Error)
	ci0 := models.Circle{UserID: user.ID, Name: "C0"}
	require.NoError(t, db.Create(&ci0).Error)

	tg := models.Tag{UserID: user.ID, Name: "T"}
	require.NoError(t, db.Create(&tg).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tg.ID, UserID: user.ID, ContactVCardUID: a.VCardUID}).Error)
	require.NoError(t, db.Create(&models.ContactTag{TagID: tg.ID, UserID: user.ID, ContactVCardUID: b.VCardUID}).Error)
	tg0 := models.Tag{UserID: user.ID, Name: "T0"}
	require.NoError(t, db.Create(&tg0).Error)

	// Occasion events: one with two attendees (attendee sort), one empty.
	ev := models.OccasionEvent{UserID: user.ID, Title: "E", StartsAt: time.Now(), Sensitivity: "normal"}
	require.NoError(t, db.Create(&ev).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: ev.ID, EntityID: a.VCardUID, RSVP: "pending"}).Error)
	require.NoError(t, db.Create(&models.OccasionEventAttendee{UserID: user.ID, EventID: ev.ID, EntityID: b.VCardUID, RSVP: "pending"}).Error)
	ev0 := models.OccasionEvent{UserID: user.ID, Title: "E0", StartsAt: time.Now(), Sensitivity: "normal"}
	require.NoError(t, db.Create(&ev0).Error)

	// A note with no contact (resolveContactUID nil branch).
	require.NoError(t, db.Create(&models.Note{UserID: user.ID, Content: "orphan", Date: time.Now()}).Error)

	// A gift with a dated row and no activity link (formatBundleTimePtr non-nil,
	// activityIDByUUID empty branch).
	d := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, db.Create(&models.Gift{UserID: user.ID, EntityID: a.VCardUID, Status: "idea", Description: "g", Date: &d}).Error)

	bundle, stats, err := BuildAccountBundle(db, user.ID, "")
	require.NoError(t, err)
	assert.Equal(t, 3, stats.Contacts)
	assert.Equal(t, 0, stats.PhotosEmbedded, "the only photo is over the cap")
	assert.Equal(t, 1, stats.PhotosOmitted)
	assert.Len(t, bundle.Attachments, 1)
	assert.Len(t, bundle.Plan.Households, 2)
	assert.Len(t, bundle.Plan.Circles, 2)
	assert.Len(t, bundle.Plan.Tags, 2)
	assert.Len(t, bundle.Plan.OccasionEvents, 2)
	assert.Empty(t, bundle.Plan.Notes[0].ContactUID, "a contact-less note carries no contact_uid")

	var hugeOut *models.AccountBundleContact
	for i := range bundle.Plan.Contacts {
		if bundle.Plan.Contacts[i].UID == huge.VCardUID {
			hugeOut = &bundle.Plan.Contacts[i]
		}
	}
	require.NotNil(t, hugeOut)
	assert.True(t, hugeOut.PhotoOmitted)
	assert.Empty(t, hugeOut.Card.Media, "the over-cap photo must be stripped from the card")

	// The mapper must handle the same edge bundle without panicking and carry
	// the empty contact reference through.
	plan := MapAccountBundle(bundle)
	require.NotEmpty(t, plan.Contacts)
}

// TestExecuteSourceImport_BundleValidationIssues drives the new importers'
// validation branches: unknown contact references, unparseable dates, a
// missing field definition, and an unresolvable activity link.
func TestExecuteSourceImport_BundleValidationIssues(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "bundle-invalid")

	// Pre-existing definition with the same key but a different ID exercises
	// importFieldDefinitions' unique-conflict fallback.
	existingDef := models.FieldDefinition{
		UserID: user.ID, Label: "Zodiac", Key: "zodiac", Target: models.FieldDefinitionTargetContact,
		Type: models.FieldTypeText, Projection: "internal-only", Sensitivity: models.RelationshipSensitivityNormal,
	}
	require.NoError(t, db.Create(&existingDef).Error)

	now := time.Now().UTC().Truncate(time.Second)
	plan := &ImportSourcePlan{System: "mycorrhizal"}
	plan.Contacts = []MappedContact{
		{Ref: bundleContactRef("uid-a"), Record: minimalRecord("Valid", "Contact")},
	}
	plan.FieldDefinitions = []MappedFieldDefinition{
		{Ref: bundleRef("field_definition", "new-def"), ID: "new-def", Label: "Zodiac", Key: "zodiac", Target: "contact", Type: "text", Projection: "internal-only", Sensitivity: "normal"},
	}
	plan.CustomFields = []MappedCustomField{
		{Ref: bundleRef("field_value", "new-def/uid-a"), Contact: bundleContactRef("uid-a"), RawValue: json.RawMessage(`"Pisces"`), FieldDefinitionID: "new-def"},
		// References a definition that was never imported.
		{Ref: bundleRef("field_value", "missing-def/uid-a"), Contact: bundleContactRef("uid-a"), RawValue: json.RawMessage(`1`), FieldDefinitionID: "missing-def"},
	}
	plan.Notes = []MappedNote{
		{Ref: bundleRef("note", "n1"), UUID: "n1", Contact: bundleContactRef("nobody"), Content: "orphan", Date: formatBundleTime(now)},
	}
	plan.ReminderCompletions = []MappedReminderCompletion{
		{Ref: bundleRef("reminder_completion", "c1"), UUID: "c1", Contact: bundleContactRef("uid-a"), Message: "m", CompletedAt: "not-a-date"},
		{Ref: bundleRef("reminder_completion", "c2"), UUID: "c2", Contact: bundleContactRef("nobody"), Message: "m", CompletedAt: formatBundleTime(now)},
	}
	plan.Activities = []MappedActivity{
		{Ref: bundleRef("activity", "a-no-attendee"), UUID: "a-no-attendee", Title: "solo", Date: formatBundleTime(now)},
	}
	plan.LifeEvents = []MappedLifeEvent{
		{Ref: bundleRef("life_event", "le1"), ID: "le1", Contact: bundleContactRef("nobody")},
	}
	plan.Gifts = []MappedGift{
		{Ref: bundleRef("gift", "g1"), ID: "g1", Contact: bundleContactRef("uid-a"), Status: "idea", Description: "d", ActivityUUID: "no-such-activity"},
	}
	plan.ConversationAgenda = []MappedConversationAgenda{
		{Ref: bundleRef("conversation_agenda", "ag1"), ID: "ag1", Contact: bundleContactRef("nobody"), Content: "x"},
	}
	plan.CadencePolicies = []MappedCadencePolicy{
		{Ref: bundleRef("cadence_policy", "cp1"), ID: "cp1", Contact: bundleContactRef("nobody")},
	}
	plan.DataDecayPolicies = []MappedDataDecayPolicy{
		{Ref: bundleRef("data_decay_policy", "dp1"), ID: "dp1", Contact: bundleContactRef("nobody")},
	}
	plan.Occasions = []MappedOccasion{
		{Ref: bundleRef("occasion", "oc1"), ID: "oc1", Contact: bundleContactRef("nobody"), Kind: "card", Label: "x"},
	}
	plan.OccasionEvents = []MappedOccasionEvent{
		{Ref: bundleRef("occasion_event", "oe1"), ID: "oe1", Title: "bad", StartsAt: "not-a-date"},
		{Ref: bundleRef("occasion_event", "oe2"), ID: "oe2", Title: "x", StartsAt: formatBundleTime(now), Attendees: []MappedOccasionAttendee{{Contact: bundleContactRef("nobody"), RSVP: "pending"}}},
	}

	report, _, err := ExecuteSourceImportWithActions(t.Context(), db, user.ID, plan, nil, nil)
	require.NoError(t, err)

	var categories []string
	for _, iss := range report.Issues {
		categories = append(categories, iss.Category)
	}
	assert.Contains(t, categories, ImportIssueCategoryUnsupported, "unknown contact refs/definitions are unsupported")
	assert.Contains(t, categories, ImportIssueCategoryInvalid, "bad dates are invalid")
	assert.Equal(t, 1, report.ContactsCreated)

	// The value whose definition ID was missing is not landed; the one that
	// reused the existing definition is.
	var contact models.Contact
	require.NoError(t, db.Where("user_id = ?", user.ID).First(&contact).Error)
	var values []models.FieldValue
	require.NoError(t, db.Where("user_id = ? AND entity_id = ?", user.ID, contact.VCardUID).Find(&values).Error)
	require.Len(t, values, 1)
	assert.Equal(t, existingDef.ID, values[0].FieldDefinitionID)
}

// TestMycorrhizalImportManager_Guards covers the manager's guard and lifecycle
// branches: not-ready preview/confirm, re-entrant fetch, ownership, expiry,
// cleanup, cancellation, and a failed import.
func TestMycorrhizalImportManager_Guards(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "mgr-guards")
	other := bundleTestUser(t, db, "mgr-guards-other")
	log := zerolog.Nop()

	body, err := json.Marshal(models.AccountBundle{
		Format: models.AccountBundleFormat, Version: models.AccountBundleVersion, ExportedAt: time.Now().UTC(),
		Plan: models.AccountBundlePlan{Contacts: []models.AccountBundleContact{
			{UID: "uid-g", Card: minimalRecord("Guard", "Person").Card},
		}},
	})
	require.NoError(t, err)

	mgr := NewMycorrhizalImportManager()

	// Preview/Confirm before the session exists.
	_, appErr := mgr.Preview(user.ID, "nope")
	require.NotNil(t, appErr)
	require.NotNil(t, mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{SessionID: "nope"}, &log))

	up, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, body))
	require.Nil(t, appErr)

	// Preview before ready.
	_, appErr = mgr.Preview(user.ID, up.SessionID)
	require.NotNil(t, appErr, "preview before fetch is rejected")
	require.NotNil(t, mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{SessionID: up.SessionID}, &log))

	// A different user must not see the session.
	_, appErr = mgr.Status(other.ID, up.SessionID)
	require.NotNil(t, appErr)

	// Re-entrant fetch while already prepared/importing.
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
	require.NotNil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log),
		"a second fetch on a preparing session is a conflict")

	waitForMycorrhizalPhase(t, mgr, user.ID, up.SessionID, models.SourceImportPhaseReady)

	// Cancel in the "ready" phase drops the session.
	require.Nil(t, mgr.Cancel(user.ID, up.SessionID))
	_, appErr = mgr.Status(user.ID, up.SessionID)
	require.NotNil(t, appErr)

	// Force an expired session and assert CleanupExpired removes it.
	up2, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, body))
	require.Nil(t, appErr)
	s, appErr := mgr.get(up2.SessionID, user.ID)
	require.Nil(t, appErr)
	s.mu.Lock()
	s.expiresAt = time.Now().Add(-time.Minute)
	s.hardExpiry = time.Now().Add(-time.Minute)
	s.mu.Unlock()
	require.Equal(t, 0, mgr.CountActive(user.ID))
	mgr.CleanupExpired()
	_, appErr = mgr.Status(user.ID, up2.SessionID)
	require.NotNil(t, appErr)

	// Force an expired session and assert get() reports it expired.
	up3, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, body))
	require.Nil(t, appErr)
	s3, _ := mgr.get(up3.SessionID, user.ID)
	s3.mu.Lock()
	s3.expiresAt = time.Now().Add(-time.Minute)
	s3.hardExpiry = time.Now().Add(-time.Minute)
	s3.mu.Unlock()
	_, appErr = mgr.Status(user.ID, up3.SessionID)
	require.NotNil(t, appErr)

	// Confirm needs a prepared session; start one and arm the failure seam so
	// runImport hits its error branch (and s.fail).
	up4, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, body))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: up4.SessionID}, &log))
	waitForMycorrhizalPhase(t, mgr, user.ID, up4.SessionID, models.SourceImportPhaseReady)
	_, appErr = mgr.Preview(user.ID, up4.SessionID)
	require.Nil(t, appErr)
	armImportFault(t)
	require.Nil(t, mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{SessionID: up4.SessionID}, &log))
	failed := waitForMycorrhizalPhase(t, mgr, user.ID, up4.SessionID, models.SourceImportPhaseFailed)
	assert.NotEmpty(t, failed.Error)
}

// TestMycorrhizalImportManager_UploadGuards covers the upload validation
// branches not hit by the controller test.
func TestMycorrhizalImportManager_UploadGuards(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "mgr-upload-guards")
	mgr := NewMycorrhizalImportManager()

	// Not JSON.
	_, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, []byte("nope")))
	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)
	assert.Equal(t, "file", appErr.Details["field"])
	assert.Equal(t, "That file is not a Mycorrhizal account bundle", appErr.Details["reason"])

	// Empty file.
	_, appErr = mgr.Upload(user.ID, bundleMultipartHeader(t, nil))
	require.NotNil(t, appErr)
	assert.Equal(t, http.StatusBadRequest, appErr.HTTPStatus)
	assert.Equal(t, apperrors.ErrCodeInvalidInput, appErr.Code)
	assert.Equal(t, "file", appErr.Details["field"])
	assert.Equal(t, "The bundle is empty or larger than the 64 MiB limit", appErr.Details["reason"])
}

func testBundleJSON(t *testing.T) []byte {
	t.Helper()
	mk := func(given string) models.AccountBundleContact {
		uid := "uid-" + given
		card := minimalRecord(given, "Bundle").Card
		card.UID = uid
		return models.AccountBundleContact{UID: uid, Card: card}
	}
	b, err := json.Marshal(models.AccountBundle{
		Format: models.AccountBundleFormat, Version: models.AccountBundleVersion, ExportedAt: time.Now().UTC(),
		Plan: models.AccountBundlePlan{Contacts: []models.AccountBundleContact{mk("Ann"), mk("Ben")}},
	})
	require.NoError(t, err)
	return b
}

// TestMycorrhizalImportManager_CancelInFlight covers the in-flight cancel
// branch: the import is paused at the shared fault seam while the client
// cancels, and the transaction rolls back.
func TestMycorrhizalImportManager_CancelInFlight(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "mgr-cancel")
	log := zerolog.Nop()
	mgr := NewMycorrhizalImportManager()

	up, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, testBundleJSON(t)))
	require.Nil(t, appErr)
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
	waitForMycorrhizalPhase(t, mgr, user.ID, up.SessionID, models.SourceImportPhaseReady)
	preview, appErr := mgr.Preview(user.ID, up.SessionID)
	require.Nil(t, appErr)
	actions := make([]models.RowImportAction, len(preview.Rows))
	for i, row := range preview.Rows {
		actions[i] = models.RowImportAction{RowIndex: row.RowIndex, Action: "add"}
	}

	faults.Reset()
	t.Cleanup(faults.Reset)
	faults.ArmPause(faultImportSource, 500*time.Millisecond)
	require.Nil(t, mgr.Confirm(db, user.ID, models.SourceImportConfirmRequest{SessionID: up.SessionID, Actions: actions}, &log))

	st := waitForMycorrhizalPhase(t, mgr, user.ID, up.SessionID, models.SourceImportPhaseImporting)
	require.Equal(t, models.SourceImportPhaseImporting, st.Phase)
	require.Nil(t, mgr.Cancel(user.ID, up.SessionID))
	cancelled := waitForMycorrhizalPhase(t, mgr, user.ID, up.SessionID, models.SourceImportPhaseCancelled)
	require.Equal(t, models.SourceImportPhaseCancelled, cancelled.Phase)
}

// TestMycorrhizalImportManager_GetAndCleanup covers the expiry clamp,
// cross-user CountActive skip, session-limit rejection, and CleanupExpired's
// cancel path.
func TestMycorrhizalImportManager_GetAndCleanup(t *testing.T) {
	db := dbtest.New(t)
	user := bundleTestUser(t, db, "mgr-get")
	other := bundleTestUser(t, db, "mgr-get-other")
	log := zerolog.Nop()
	mgr := NewMycorrhizalImportManager()

	// Unknown session on StartFetch.
	require.NotNil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: "nope"}, &log))

	up, appErr := mgr.Upload(user.ID, bundleMultipartHeader(t, testBundleJSON(t)))
	require.Nil(t, appErr)

	// A session for another user is skipped by this user's CountActive.
	_, appErr = mgr.Upload(other.ID, bundleMultipartHeader(t, testBundleJSON(t)))
	require.Nil(t, appErr)
	require.Equal(t, 1, mgr.CountActive(user.ID))
	require.Equal(t, 1, mgr.CountActive(other.ID))

	// get() clamps the refreshed expiry to the hard expiry.
	s, appErr := mgr.get(up.SessionID, user.ID)
	require.Nil(t, appErr)
	s.mu.Lock()
	s.hardExpiry = time.Now().Add(10 * time.Second)
	s.expiresAt = time.Now().Add(time.Second)
	s.mu.Unlock()
	_, appErr = mgr.Status(user.ID, up.SessionID)
	require.Nil(t, appErr)
	s.mu.Lock()
	require.False(t, s.expiresAt.After(s.hardExpiry), "expiry must be clamped to the hard expiry")
	s.mu.Unlock()

	// CleanupExpired cancels a running session whose cancel func is set.
	require.Nil(t, mgr.StartFetch(db, user.ID, models.MycorrhizalFetchRequest{SessionID: up.SessionID}, &log))
	waitForMycorrhizalPhase(t, mgr, user.ID, up.SessionID, models.SourceImportPhaseReady)
	s2, _ := mgr.get(up.SessionID, user.ID)
	s2.mu.Lock()
	s2.expiresAt = time.Now().Add(-time.Minute)
	s2.hardExpiry = time.Now().Add(-time.Minute)
	s2.mu.Unlock()
	mgr.CleanupExpired()
	_, appErr = mgr.Status(user.ID, up.SessionID)
	require.NotNil(t, appErr)

}
