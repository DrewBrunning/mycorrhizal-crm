package controllers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"mycorrhizal/config"
	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// idempotencyE2ERouter wires CreateContact behind the real production
// middleware chain fragment — IdempotencyMiddleware then ValidateJSONMiddleware
// — against a real migrated schema, plus a webhook subscription to
// contact.created pointed at a counting httptest server so the "one side
// effect, not two" property (ADR 0010 bucket 3) is observable.
func idempotencyE2ERouter(t *testing.T) (*gin.Engine, *gorm.DB, models.User, *atomic.Int64) {
	t.Helper()
	db := dbtest.New(t)
	user := models.User{Username: "idem-e2e", Email: "idem-e2e@example.com", Password: "password123!A"}
	require.NoError(t, db.Create(&user).Error)

	var hits atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	require.NoError(t, db.Create(&models.Webhook{
		UserID: user.ID, Name: "created-hook", URL: target.URL,
		Events: []string{"contact.created"}, IsActive: true,
	}).Error)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{})
		c.Next()
	})
	r.Use(middleware.IdempotencyMiddleware())
	r.POST("/contacts", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), CreateContact)
	return r, db, user, &hits
}

func contactCreateBody(t *testing.T, given string) []byte {
	t.Helper()
	b, err := json.Marshal(models.ContactRecordInput{Card: contactmodel.Card{
		Name: &contactmodel.Name{Components: []contactmodel.NameComponent{{Kind: "given", Value: given}}},
	}})
	require.NoError(t, err)
	return b
}

// TestIdempotency_E2E_DoubleSubmitCreate_OneContactOneWebhook is the ticket's
// "Double-submitting a create produces one contact, not two" plus "A retried
// webhook or notification delivers once" — through the real handler.
func TestIdempotency_E2E_DoubleSubmitCreate_OneContactOneWebhook(t *testing.T) {
	r, db, user, hits := idempotencyE2ERouter(t)
	body := contactCreateBody(t, "Double Submit")

	post := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/contacts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "dbl-submit-1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	first := post()
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())

	// The user "did not see" the response and submits again (ambiguous failure).
	second := post()
	require.Equal(t, http.StatusCreated, second.Code, second.Body.String())
	require.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	require.JSONEq(t, first.Body.String(), second.Body.String())

	// A third retry, for good measure.
	require.Equal(t, "true", post().Header().Get("Idempotency-Replayed"))

	services.WaitForWebhookGoroutines()

	var contacts, deliveries int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	require.NoError(t, db.Model(&models.WebhookDelivery{}).Count(&deliveries).Error)
	require.EqualValues(t, 1, contacts, "double-submit produced exactly one contact")
	require.EqualValues(t, 1, deliveries, "the contact.created webhook was delivered exactly once")
	require.EqualValues(t, 1, hits.Load(), "the webhook target was hit exactly once — the replay never re-entered the handler")
}

// TestIdempotency_E2E_AmbiguousFailure_CommitThenLoseResponse simulates #434's
// ambiguous-failure shape directly: the first request's response is dropped on
// the wire (the client's recorder is discarded), the write having already
// committed; the client retries and must get the stored result, not a second
// contact.
func TestIdempotency_E2E_AmbiguousFailure_CommitThenLoseResponse(t *testing.T) {
	r, db, user, hits := idempotencyE2ERouter(t)
	body := contactCreateBody(t, "Ambiguous")

	send := func() *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/contacts", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "ambiguous-key")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Request 1: the handler runs, the row commits — then we throw the response
	// away, exactly as a dropped connection would.
	lost := send()
	require.Equal(t, http.StatusCreated, lost.Code, "the write did commit server-side")
	_ = lost.Body // discarded, as if never received

	// Request 2: the client, unable to tell "did it land?", retries.
	retry := send()
	require.Equal(t, http.StatusCreated, retry.Code, retry.Body.String())
	require.Equal(t, "true", retry.Header().Get("Idempotency-Replayed"))

	services.WaitForWebhookGoroutines()

	var contacts int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	require.EqualValues(t, 1, contacts, "the retry after an ambiguous failure produced no second contact")
	require.EqualValues(t, 1, hits.Load(), "and no second side effect")
}

// TestIdempotency_E2E_KeyReusedForDifferentContact_422 pins that a client bug —
// the same key on a genuinely different create — is refused loudly rather than
// silently returning the first contact.
func TestIdempotency_E2E_KeyReusedForDifferentContact_422(t *testing.T) {
	r, db, user, _ := idempotencyE2ERouter(t)

	mk := func(given string) *httptest.ResponseRecorder {
		req, _ := http.NewRequest("POST", "/contacts", bytes.NewReader(contactCreateBody(t, given)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "shared-key")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	require.Equal(t, http.StatusCreated, mk("First Person").Code)
	clash := mk("Different Person")
	require.Equal(t, http.StatusUnprocessableEntity, clash.Code, clash.Body.String())

	var contacts int64
	require.NoError(t, db.Model(&models.Contact{}).Where("user_id = ?", user.ID).Count(&contacts).Error)
	require.EqualValues(t, 1, contacts, "the clashing key must not have created a second contact")
}
