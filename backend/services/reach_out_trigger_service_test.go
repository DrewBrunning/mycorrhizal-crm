package services

import (
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// setupReachOutTestDB opens a real migrated schema (CLAUDE.md backend trap 1
// — the hand-written gorm column tags on ReachOutSuggestion/ReachOutCursor
// must be verified against the actual migration SQL, not AutoMigrate) and
// registers the audit recorder so Contact saves actually produce the
// AuditEvent rows the detection job reads.
func setupReachOutTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	models.RegisterAuditDB(db)
	t.Cleanup(func() { models.RegisterAuditDB(nil) })
	return db
}

func createTestUserAndContact(t *testing.T, db *gorm.DB, org, jobTitle string) (models.User, models.Contact) {
	t.Helper()
	user := models.User{Username: "reachoutuser", Password: "password123!A", Email: "reachout@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice", Lastname: "Smith", Organization: org, JobTitle: jobTitle}
	require.NoError(t, db.Create(&contact).Error)
	models.AuditFlush()
	return user, contact
}

func TestDetectReachOutSuggestions_OrganizationChange(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	user, contact := createTestUserAndContact(t, db, "OldCo", "")

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	wh := models.Webhook{UserID: user.ID, Name: "n8n", URL: server.URL, Events: []string{"reach_out_suggested"}, Secret: "s", IsActive: true}
	require.NoError(t, db.Create(&wh).Error)

	contact.Organization = "NewCo"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var suggestions []models.ReachOutSuggestion
	require.NoError(t, db.Find(&suggestions).Error)
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.ReachOutKindOrganization, suggestions[0].Kind)
	assert.Equal(t, "OldCo", suggestions[0].OldValue)
	assert.Equal(t, "NewCo", suggestions[0].NewValue)
	assert.Equal(t, models.ReachOutStatusPending, suggestions[0].Status)
	require.NotNil(t, suggestions[0].ReminderID)

	var reminder models.Reminder
	require.NoError(t, db.First(&reminder, *suggestions[0].ReminderID).Error)
	assert.Equal(t, "once", reminder.Recurrence)
	assert.Equal(t, contact.ID, *reminder.ContactID)

	require.Eventually(t, func() bool {
		return atomic.LoadInt32(&hits) >= 1
	}, 3*time.Second, 10*time.Millisecond, "the org change must trigger a reach_out_suggested webhook")
	time.Sleep(150 * time.Millisecond)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))

	var deliveries []models.WebhookDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	require.Len(t, deliveries, 1)
	assert.Equal(t, "reach_out_suggested", deliveries[0].EventType)
	var envelope struct {
		Data struct {
			Kind     string `json:"kind"`
			OldValue string `json:"old_value"`
			NewValue string `json:"new_value"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal([]byte(deliveries[0].Payload), &envelope))
	assert.Equal(t, "organization", envelope.Data.Kind)
	assert.Equal(t, "NewCo", envelope.Data.NewValue)
}

func TestDetectReachOutSuggestions_TitleChange(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "", "Engineer")

	contact.JobTitle = "Senior Engineer"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var suggestions []models.ReachOutSuggestion
	require.NoError(t, db.Find(&suggestions).Error)
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.ReachOutKindTitle, suggestions[0].Kind)
	assert.Equal(t, "Engineer", suggestions[0].OldValue)
	assert.Equal(t, "Senior Engineer", suggestions[0].NewValue)
}

func TestDetectReachOutSuggestions_AddressMove(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	user, contact := createTestUserAndContact(t, db, "", "")

	oldAddr := models.ContactAddress{Street: "1 Old Rd", City: "Springfield", Region: "IL", Country: "USA"}
	contact.Addresses = []models.ContactAddress{oldAddr}
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	// Seed the cursor past this first update (as if a prior run already
	// processed it) so the batch below contains only the second update,
	// giving a genuine old-address -> new-address diff rather than
	// comparing against the contact's pre-existing (empty) baseline.
	var firstEvent models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).
		Order("id DESC").First(&firstEvent).Error)
	require.NoError(t, db.Create(&models.ReachOutCursor{UserID: user.ID, LastAuditEventID: firstEvent.ID}).Error)

	newAddr := models.ContactAddress{Street: "2 New Ave", City: "Shelbyville", Region: "IL", Country: "USA"}
	contact.Addresses = []models.ContactAddress{newAddr}
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var suggestions []models.ReachOutSuggestion
	require.NoError(t, db.Find(&suggestions).Error)
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.ReachOutKindAddress, suggestions[0].Kind)
	assert.Contains(t, suggestions[0].OldValue, "1 Old Rd")
	assert.Contains(t, suggestions[0].NewValue, "2 New Ave")
}

// TestDetectReachOutSuggestions_AddressPostalOnlyChangeFires covers a code
// review fix: AddressNormalizedKey includes the postal code, but the
// suggestion's rendered old/new value must too (via models.FormatAddress) —
// otherwise a postal-only change produced two different normalized keys that
// rendered as the identical display string ("X → X"), a meaningless
// suggestion the "no noisy/empty triggers" rule is supposed to prevent.
func TestDetectReachOutSuggestions_AddressPostalOnlyChangeFires(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	user, contact := createTestUserAndContact(t, db, "", "")

	oldAddr := models.ContactAddress{Street: "1 Main St", City: "Springfield", Postal: "12345", Country: "USA"}
	contact.Addresses = []models.ContactAddress{oldAddr}
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	var firstEvent models.AuditEvent
	require.NoError(t, db.Where("entity_type = ? AND entity_id = ? AND operation = ?",
		models.AuditEntityContact, contact.VCardUID, models.AuditOpUpdate).
		Order("id DESC").First(&firstEvent).Error)
	require.NoError(t, db.Create(&models.ReachOutCursor{UserID: user.ID, LastAuditEventID: firstEvent.ID}).Error)

	newAddr := models.ContactAddress{Street: "1 Main St", City: "Springfield", Postal: "54321", Country: "USA"}
	contact.Addresses = []models.ContactAddress{newAddr}
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var suggestions []models.ReachOutSuggestion
	require.NoError(t, db.Find(&suggestions).Error)
	require.Len(t, suggestions, 1)
	assert.Equal(t, models.ReachOutKindAddress, suggestions[0].Kind)
	assert.NotEqual(t, suggestions[0].OldValue, suggestions[0].NewValue, "the rendered old/new values must differ, not just the normalized key")
	assert.Contains(t, suggestions[0].OldValue, "12345")
	assert.Contains(t, suggestions[0].NewValue, "54321")
}

func TestDetectReachOutSuggestions_UnchangedValueDoesNotFire(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "SameCo", "")

	// Touch an unrelated field; organization stays identical.
	contact.Lastname = "Jones"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.Zero(t, count, "no field actually changed value, so nothing should fire")
}

func TestDetectReachOutSuggestions_ClearedValueDoesNotFire(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "SomeCo", "")

	contact.Organization = ""
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.Zero(t, count, "a value being cleared is not a reason to reach out")
}

func TestDetectReachOutSuggestions_CreationOnlyDoesNotFire(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	createTestUserAndContact(t, db, "BrandNewCo", "")

	// Only a create event exists (no prior state to diff against).
	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.Zero(t, count, "a freshly created contact has no prior state to compare against")
}

func TestDetectReachOutSuggestions_DeletedContactSkipped(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "OldCo", "")

	contact.Organization = "NewCo"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	require.NoError(t, db.Delete(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.Zero(t, count, "a contact deleted before detection runs must not get a reach-out suggestion")
}

func TestDetectReachOutSuggestions_CursorPreventsDoubleFire(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "OldCo", "")

	contact.Organization = "NewCo"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)
	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "a second run with no new changes must not double-fire")
}

func TestDetectReachOutSuggestions_JobLockSkipsRapidRerun(t *testing.T) {
	db := setupReachOutTestDB(t)
	cfg := config.Config{}
	_, contact := createTestUserAndContact(t, db, "OldCo", "")

	contact.Organization = "NewCo"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	var count int64
	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	require.EqualValues(t, 1, count)

	// A second org change immediately after — the job-lock (23h min interval)
	// must refuse to run again this soon, so the new change is not detected
	// yet (it will be on the next real run, once the lock clears).
	contact.Organization = "ThirdCo"
	require.NoError(t, db.Save(&contact).Error)
	models.AuditFlush()

	DetectReachOutSuggestions(db, cfg)

	require.NoError(t, db.Model(&models.ReachOutSuggestion{}).Count(&count).Error)
	assert.EqualValues(t, 1, count, "the job-lock must prevent a rapid rerun from processing the new change")
}
