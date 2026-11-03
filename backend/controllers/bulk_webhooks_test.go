package controllers

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #2311: bulk update_fields and delete must fire the same webhooks as
// their single-contact routes (contact.updated / contact.deleted).

type capturedWebhook struct {
	event string
	data  map[string]any
}

// bulkWebhookCapture subscribes the user to events and records every delivery.
func bulkWebhookCapture(t *testing.T, db *gorm.DB, userID uint, events ...string) func() []capturedWebhook {
	t.Helper()
	var mu sync.Mutex
	var got []capturedWebhook
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var env struct {
			Event string         `json:"event"`
			Data  map[string]any `json:"data"`
		}
		_ = json.Unmarshal(body, &env)
		mu.Lock()
		got = append(got, capturedWebhook{event: env.Event, data: env.Data})
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	require.NoError(t, db.Create(&models.Webhook{
		UserID: userID, Name: "bulk", URL: srv.URL, Events: events, Secret: "s", IsActive: true,
	}).Error)
	return func() []capturedWebhook {
		services.WaitForWebhookGoroutines()
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedWebhook(nil), got...)
	}
}

func TestBulkUpdateFieldsFiresContactUpdatedPerContact(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	bob := createBulkContact(t, db, user.ID, "Bob")
	events := bulkWebhookCapture(t, db, user.ID, "contact.updated", "contact.deleted")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{alice.VCardUID, bob.VCardUID},
		Set: map[string]string{"organization": "Acme"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, result.Succeeded)

	got := events()
	require.Len(t, got, 2, "one contact.updated per updated contact")
	names := map[any]bool{}
	for _, e := range got {
		assert.Equal(t, "contact.updated", e.event)
		names[e.data["firstname"]] = true
		assert.Equal(t, "Acme", e.data["organization"], "payload carries the saved contact")
	}
	assert.True(t, names["Alice"] && names["Bob"])
}

func TestBulkUpdateFieldsFailedContactFiresNoWebhook(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	good := createBulkContact(t, db, user.ID, "Good")
	bad := createBulkContact(t, db, user.ID, "Bad")
	require.NoError(t, db.Exec(`CREATE TRIGGER bulk_wh_fail BEFORE UPDATE ON contacts WHEN NEW.firstname = 'Bad' BEGIN SELECT RAISE(ABORT, 'forced failure'); END`).Error)
	events := bulkWebhookCapture(t, db, user.ID, "contact.updated")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "update_fields", VCardUIDs: []string{good.VCardUID, bad.VCardUID},
		Set: map[string]string{"organization": "Acme"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, result.Succeeded)
	require.Equal(t, 1, result.Failed)

	got := events()
	require.Len(t, got, 1, "the failed contact delivers nothing")
	assert.Equal(t, "Good", got[0].data["firstname"])
}

func TestBulkDeleteFiresContactDeletedPerSuccess(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	alice := createBulkContact(t, db, user.ID, "Alice")
	bob := createBulkContact(t, db, user.ID, "Bob")
	events := bulkWebhookCapture(t, db, user.ID, "contact.deleted")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "delete", VCardUIDs: []string{alice.VCardUID, bob.VCardUID, "00000000-0000-4000-8000-000000000000"},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 2, result.Succeeded)
	require.Equal(t, 1, result.Failed)

	got := events()
	require.Len(t, got, 2, "one contact.deleted per deleted contact, none for the failure")
	ids := map[float64]bool{}
	for _, e := range got {
		assert.Equal(t, "contact.deleted", e.event)
		ids[e.data["id"].(float64)] = true
	}
	assert.True(t, ids[float64(alice.ID)] && ids[float64(bob.ID)], "payload is {id} like the single route")
}

func TestBulkDeleteFailedContactFiresNoWebhook(t *testing.T) {
	db, router := bulkTestRouter(t)
	user := createBulkTestUser(t, db)
	good := createBulkContact(t, db, user.ID, "Good")
	bad := createBulkContact(t, db, user.ID, "Bad")
	// Soft delete is an UPDATE of deleted_at; fail it for one contact only.
	require.NoError(t, db.Exec(`CREATE TRIGGER bulk_del_fail BEFORE UPDATE ON contacts WHEN NEW.firstname = 'Bad' BEGIN SELECT RAISE(ABORT, 'forced failure'); END`).Error)
	events := bulkWebhookCapture(t, db, user.ID, "contact.deleted")

	code, result := doBulk(t, router, models.BulkContactOperationInput{
		Action: "delete", VCardUIDs: []string{good.VCardUID, bad.VCardUID},
	})
	require.Equal(t, http.StatusOK, code)
	require.Equal(t, 1, result.Succeeded)
	require.Equal(t, 1, result.Failed)

	got := events()
	require.Len(t, got, 1, "the failed delete delivers nothing")
	assert.Equal(t, float64(good.ID), got[0].data["id"])
}
