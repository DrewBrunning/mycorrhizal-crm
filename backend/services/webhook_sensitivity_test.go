package services

// Issue #2888: a webhook with include_sensitive explicitly false receives a
// contact payload without private/secret addresses or their coordinates, and
// the filtered body is what a failed delivery stores for retry (a retry must
// not resurrect the sensitive data). The default (nil) and explicit true are
// unchanged in v1.x; the v2.0.0 flip is #2071.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sensitiveContactFixture() models.Contact {
	contact := models.Contact{
		Firstname: "Ada",
		Lastname:  "Lovelace",
		Addresses: []models.ContactAddress{
			{
				Street:      "1 Secret Way",
				City:        "Hidden",
				Sensitivity: models.RelationshipSensitivitySecret,
				Coordinates: "geo:51.5,-0.1",
			},
			{
				Street:      "2 Open Road",
				City:        "Visible",
				Sensitivity: models.RelationshipSensitivityNormal,
			},
			{
				Street:      "3 Private Lane",
				City:        "Quiet",
				Sensitivity: models.RelationshipSensitivityPrivate,
			},
		},
	}
	// The flat scalar is normally the first address (deriveDenormalized); point
	// it at the secret one to prove the filter recomputes it.
	contact.Address = models.FormatAddress(contact.Addresses[0])
	return contact
}

func TestTriggerWebhooksIncludeSensitiveFalseFiltersContactAddresses(t *testing.T) {
	db := setupWebhookRetryTestDB(t)

	received := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	off := false
	webhook := newTestWebhook(server.URL, "secret")
	webhook.IncludeSensitive = &off
	require.NoError(t, db.Create(&webhook).Error)

	TriggerWebhooks(context.Background(), db, config.Config{}, 1, "contact.created", sensitiveContactFixture())
	WaitForWebhookGoroutines()

	var body []byte
	select {
	case body = <-received:
	case <-time.After(2 * time.Second):
		t.Fatal("the filtered webhook was not delivered")
	}

	assert.Contains(t, string(body), "2 Open Road", "a normal address must survive")
	assert.NotContains(t, string(body), "1 Secret Way", "a secret address must not be sent")
	assert.NotContains(t, string(body), "3 Private Lane", "a private address must not be sent")
	assert.NotContains(t, string(body), "geo:51.5,-0.1", "the secret address's coordinate must not be sent")
	assert.NotContains(t, string(body), `"address":"1 Secret Way`, "the flat scalar is recomputed from the first kept address")
}

func TestTriggerWebhooksDefaultAndExplicitTrueKeepEveryAddress(t *testing.T) {
	db := setupWebhookRetryTestDB(t)

	received := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// One webhook follows the server default (nil); the other explicitly keeps
	// full fidelity. Both must be unchanged in v1.x.
	byDefault := newTestWebhook(server.URL, "s1")
	require.NoError(t, db.Create(&byDefault).Error)

	on := true
	explicit := newTestWebhook(server.URL, "s2")
	explicit.IncludeSensitive = &on
	require.NoError(t, db.Create(&explicit).Error)

	TriggerWebhooks(context.Background(), db, config.Config{}, 1, "contact.created", sensitiveContactFixture())
	WaitForWebhookGoroutines()

	for i := 0; i < 2; i++ {
		select {
		case body := <-received:
			assert.Contains(t, string(body), "1 Secret Way")
			assert.Contains(t, string(body), "geo:51.5,-0.1")
			assert.Contains(t, string(body), "3 Private Lane")
		case <-time.After(2 * time.Second):
			t.Fatal("both full-fidelity webhooks must be delivered")
		}
	}
}

func TestTriggerWebhooksFailedFilteredDeliveryStoresNoSensitiveData(t *testing.T) {
	db := setupWebhookRetryTestDB(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	off := false
	webhook := newTestWebhook(server.URL, "secret")
	webhook.IncludeSensitive = &off
	require.NoError(t, db.Create(&webhook).Error)

	TriggerWebhooks(context.Background(), db, config.Config{}, 1, "contact.created", sensitiveContactFixture())
	WaitForWebhookGoroutines()

	var delivery models.WebhookDelivery
	require.NoError(t, db.First(&delivery).Error)
	assert.NotEmpty(t, delivery.Payload, "a failed delivery keeps its body for retry")
	assert.NotContains(t, delivery.Payload, "1 Secret Way",
		"the stored retry body must be the filtered one")
	assert.NotContains(t, delivery.Payload, "geo:51.5,-0.1")
	assert.Contains(t, delivery.Payload, "2 Open Road")
}

func TestRedactSensitiveJSONDropsNestedObjects(t *testing.T) {
	input := map[string]interface{}{
		"name": "Ada",
		"addresses": []interface{}{
			map[string]interface{}{"street": "Open", "sensitivity": "normal"},
			map[string]interface{}{"street": "Hidden", "sensitivity": "secret"},
		},
		"wrapper": map[string]interface{}{
			"sensitivity": "private",
			"nested":      map[string]interface{}{"street": "Nested Hidden"},
		},
	}

	redacted, keep := redactSensitiveJSON(input)
	require.True(t, keep)
	encoded, err := json.Marshal(redacted)
	require.NoError(t, err)

	assert.Contains(t, string(encoded), "Open")
	assert.NotContains(t, string(encoded), "Hidden")
	assert.NotContains(t, string(encoded), "Nested Hidden")
	assert.NotContains(t, string(encoded), `"sensitivity":"secret"`)
}

func TestFilteredPayloadBodyKeepsTheEnvelopeShape(t *testing.T) {
	contact := sensitiveContactFixture()
	body, err := buildFilteredPayloadBody("contact.created", &contact)
	require.NoError(t, err)

	var envelope struct {
		ID        string          `json:"id"`
		Event     string          `json:"event"`
		Timestamp string          `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	assert.NotEmpty(t, envelope.ID)
	assert.Equal(t, "contact.created", envelope.Event)
	assert.NotEmpty(t, envelope.Timestamp)
	assert.Contains(t, string(envelope.Data), "2 Open Road")
}

func TestFilteredPayloadBodyKeepsAContactWithNoSensitiveAddressesWhole(t *testing.T) {
	contact := models.Contact{
		Firstname: "Ada",
		Addresses: []models.ContactAddress{
			{Street: "1 Open Road", City: "Visible", Sensitivity: models.RelationshipSensitivityNormal},
		},
	}
	contact.Address = models.FormatAddress(contact.Addresses[0])

	body, err := buildFilteredPayloadBody("contact.created", contact)
	require.NoError(t, err)
	assert.Contains(t, string(body), "1 Open Road")
	assert.Contains(t, string(body), `"address":"1 Open Road, Visible"`)
}

func TestFilteredPayloadBodyAllSensitiveAddressesClearsTheFlatScalar(t *testing.T) {
	contact := models.Contact{
		Firstname: "Ada",
		Addresses: []models.ContactAddress{
			{Street: "1 Secret Way", Sensitivity: models.RelationshipSensitivitySecret},
		},
	}
	contact.Address = models.FormatAddress(contact.Addresses[0])

	body, err := buildFilteredPayloadBody("contact.created", contact)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "1 Secret Way")
	assert.Contains(t, string(body), `"address":""`)
}

func TestFilteredPayloadBodyNilContactIsSafe(t *testing.T) {
	body, err := buildFilteredPayloadBody("contact.created", (*models.Contact)(nil))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"data":null`)
}
