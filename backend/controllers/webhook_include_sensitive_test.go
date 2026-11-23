package controllers

// Issue #2888: webhooks gain a nullable per-webhook include_sensitive choice.
// A response that still relies on the server default (NULL) announces the
// deprecated full-fidelity default; an explicit true or false is a recorded
// choice and stays silent. PUT is a full replace, so omitting the field resets
// the webhook to the default.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func webhookRequest(t *testing.T, router *gin.Engine, method, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		require.NoError(t, err)
	}
	req, _ := http.NewRequest(method, path, bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func assertWebhookDefaultDeprecated(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Equal(t, "true", w.Header().Get("Deprecation"))
	assert.Equal(t, webhookIncludeSensitiveDeprecationSunset, w.Header().Get("Sunset"))
	link := w.Header().Get("Link")
	assert.Contains(t, link, `rel="deprecation"`)
	assert.Contains(t, link, "deprecation-policy")
}

func assertNotDeprecatedWebhookDefault(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	assert.Empty(t, w.Header().Get("Deprecation"))
	assert.Empty(t, w.Header().Get("Sunset"))
	assert.Empty(t, w.Header().Get("Link"))
}

func createWebhookForTest(t *testing.T, router *gin.Engine, includeSensitive *bool) models.WebhookResponse {
	t.Helper()
	input := models.WebhookInput{
		Name:             "hook",
		URL:              "https://example.com/hook",
		Events:           []string{"contact.created"},
		IsActive:         true,
		IncludeSensitive: includeSensitive,
	}
	w := webhookRequest(t, router, http.MethodPost, "/webhooks", input)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	var resp models.WebhookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	return resp
}

func TestCreateWebhook_IncludeSensitiveOmittedUsesDefaultAndSignals(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	db.First(&user)
	router := routerForUser(db, user.ID)
	router.POST("/webhooks", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), CreateWebhook)

	w := webhookRequest(t, router, http.MethodPost, "/webhooks", models.WebhookInput{
		Name: "default hook", URL: "https://example.com/hook", Events: []string{"contact.created"}, IsActive: true,
	})

	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	assertWebhookDefaultDeprecated(t, w)

	var resp models.WebhookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.IncludeSensitive)
}

func TestCreateWebhook_ExplicitIncludeSensitiveDoesNotSignal(t *testing.T) {
	for _, explicit := range []bool{true, false} {
		t.Run(map[bool]string{true: "true", false: "false"}[explicit], func(t *testing.T) {
			db, _ := setupRouter(t)
			var user models.User
			db.First(&user)
			router := routerForUser(db, user.ID)
			router.POST("/webhooks", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), CreateWebhook)

			value := explicit
			w := webhookRequest(t, router, http.MethodPost, "/webhooks", models.WebhookInput{
				Name: "explicit hook", URL: "https://example.com/hook", Events: []string{"contact.created"},
				IsActive: true, IncludeSensitive: &value,
			})

			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			assertNotDeprecatedWebhookDefault(t, w)

			var resp models.WebhookResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
			require.NotNil(t, resp.IncludeSensitive)
			assert.Equal(t, explicit, *resp.IncludeSensitive)
		})
	}
}

func TestListWebhooks_SignalsWhileAnyRowUsesTheDefault(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	db.First(&user)
	router := routerForUser(db, user.ID)
	router.GET("/webhooks", ListWebhooks)

	require.NoError(t, db.Create(&models.Webhook{
		UserID: user.ID, Name: "default", URL: "https://example.com/a", Events: []string{"contact.created"},
		Secret: "s1", IsActive: true,
	}).Error)
	on := true
	require.NoError(t, db.Create(&models.Webhook{
		UserID: user.ID, Name: "explicit", URL: "https://example.com/b", Events: []string{"contact.created"},
		Secret: "s2", IsActive: true, IncludeSensitive: &on,
	}).Error)

	w := webhookRequest(t, router, http.MethodGet, "/webhooks", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assertWebhookDefaultDeprecated(t, w)

	var body struct {
		Webhooks []models.WebhookResponse `json:"webhooks"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.Len(t, body.Webhooks, 2)
}

func TestListWebhooks_SilentWhenEveryRowIsExplicit(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	db.First(&user)
	router := routerForUser(db, user.ID)
	router.GET("/webhooks", ListWebhooks)

	on := true
	require.NoError(t, db.Create(&models.Webhook{
		UserID: user.ID, Name: "explicit", URL: "https://example.com/a", Events: []string{"contact.created"},
		Secret: "s1", IsActive: true, IncludeSensitive: &on,
	}).Error)

	w := webhookRequest(t, router, http.MethodGet, "/webhooks", nil)
	require.Equal(t, http.StatusOK, w.Code)
	assertNotDeprecatedWebhookDefault(t, w)
}

func TestGetWebhook_SignalsForADefaultWebhook(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	db.First(&user)
	router := routerForUser(db, user.ID)
	router.GET("/webhooks/:id", GetWebhook)

	wh := models.Webhook{
		UserID: user.ID, Name: "default", URL: "https://example.com/a", Events: []string{"contact.created"},
		Secret: "s1", IsActive: true,
	}
	require.NoError(t, db.Create(&wh).Error)

	w := webhookRequest(t, router, http.MethodGet, "/webhooks/"+strconv.FormatUint(uint64(wh.ID), 10), nil)
	require.Equal(t, http.StatusOK, w.Code)
	assertWebhookDefaultDeprecated(t, w)
}

func TestUpdateWebhook_OmittingResetsToTheDefaultAndSignals(t *testing.T) {
	db, _ := setupRouter(t)
	var user models.User
	db.First(&user)
	router := routerForUser(db, user.ID)
	router.PUT("/webhooks/:id", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), UpdateWebhook)

	on := true
	wh := models.Webhook{
		UserID: user.ID, Name: "hook", URL: "https://example.com/a", Events: []string{"contact.created"},
		Secret: "s1", IsActive: true, IncludeSensitive: &on,
	}
	require.NoError(t, db.Create(&wh).Error)
	id := strconv.FormatUint(uint64(wh.ID), 10)

	// An explicit choice stays silent.
	off := false
	w := webhookRequest(t, router, http.MethodPut, "/webhooks/"+id, models.WebhookInput{
		Name: "hook", URL: "https://example.com/a", Events: []string{"contact.created"}, IsActive: true,
		IncludeSensitive: &off,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertNotDeprecatedWebhookDefault(t, w)

	var stored models.Webhook
	require.NoError(t, db.First(&stored, wh.ID).Error)
	require.NotNil(t, stored.IncludeSensitive)
	assert.False(t, *stored.IncludeSensitive)

	// Omitting the field is a full replace back to the server default: NULL,
	// and therefore the deprecated behaviour is announced.
	w = webhookRequest(t, router, http.MethodPut, "/webhooks/"+id, models.WebhookInput{
		Name: "hook", URL: "https://example.com/a", Events: []string{"contact.created"}, IsActive: true,
	})
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assertWebhookDefaultDeprecated(t, w)

	var resp models.WebhookResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Nil(t, resp.IncludeSensitive)

	require.NoError(t, db.First(&stored, wh.ID).Error)
	assert.Nil(t, stored.IncludeSensitive)
}
