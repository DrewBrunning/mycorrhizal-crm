package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"mycorrhizal/middleware"
	"mycorrhizal/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeNtfyServer is a minimal ntfy stand-in for the controller tests: it
// records whether it was hit and answers 200.
type fakeNtfyServer struct {
	server *httptest.Server
	mu     sync.Mutex
	hit    bool
	path   string
}

func newFakeNtfyServer(t *testing.T) *fakeNtfyServer {
	t.Helper()
	f := &fakeNtfyServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body) //nolint:errcheck
		f.mu.Lock()
		f.hit = true
		f.path = r.URL.Path
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeNtfyServer) URL() string { return f.server.URL }

func (f *fakeNtfyServer) wasHit() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hit
}

func TestNotificationConfig_GetReturnsDefaults(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notifications/config", GetNotificationConfig)

	req, _ := http.NewRequest("GET", "/notifications/config", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp notificationConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Empty(t, resp.NtfyURL)
	assert.Empty(t, resp.GotifyURL)
	assert.False(t, resp.GotifyHasToken)
	assert.False(t, resp.NotifyNtfy)
	assert.False(t, resp.NotifyGotify)
	assert.False(t, resp.NotifyPush)
	assert.NotEmpty(t, resp.VAPIDPublicKey, "the VAPID public key must be available for the browser to subscribe")

	// The generated VAPID keys are persisted for reuse.
	var count int64
	require.NoError(t, db.Model(&models.ServerSetting{}).Count(&count).Error)
	assert.Equal(t, int64(2), count)
}

func TestNotificationConfig_SaveAndReload(t *testing.T) {
	_, router := setupRouter()
	router.PUT("/notifications/config", withValidated(func() any { return &models.NotificationConfigInput{} }), SaveNotificationConfig)
	router.GET("/notifications/config", GetNotificationConfig)

	trueVal := true
	body, _ := json.Marshal(models.NotificationConfigInput{
		NtfyURL:     "https://ntfy.example.com",
		NtfyTopic:   "alerts",
		GotifyURL:   "https://gotify.example.com",
		GotifyToken: "secret",
		NotifyNtfy:  &trueVal,
	})
	req, _ := http.NewRequest("PUT", "/notifications/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp notificationConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://ntfy.example.com", resp.NtfyURL)
	assert.Equal(t, "alerts", resp.NtfyTopic)
	assert.Equal(t, "https://gotify.example.com", resp.GotifyURL)
	assert.True(t, resp.GotifyHasToken)
	assert.True(t, resp.NotifyNtfy)

	// Reload via GET sees the saved state.
	req, _ = http.NewRequest("GET", "/notifications/config", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "https://ntfy.example.com", resp.NtfyURL)
	assert.True(t, resp.NotifyNtfy)

	// The token is never returned.
	assert.NotContains(t, w.Body.String(), "secret")
}

func TestNotificationConfig_InvalidURLRejected(t *testing.T) {
	_, router := setupRouter()
	router.PUT("/notifications/config", withValidated(func() any { return &models.NotificationConfigInput{} }), SaveNotificationConfig)

	// A malformed URL is rejected at save time with a 400, not accepted and
	// left to fail silently on first dispatch.
	body, _ := json.Marshal(models.NotificationConfigInput{NtfyURL: "not a url"})
	req, _ := http.NewRequest("PUT", "/notifications/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	// A dangerous scheme is rejected by the middleware's safeurl validator too.
	verrs := middleware.ValidateStruct(&models.NotificationConfigInput{NtfyURL: "javascript:alert(1)"})
	require.NotEmpty(t, verrs, "a scriptable scheme must be rejected by safeurl")
}

func TestNotificationConfig_TestNtfy(t *testing.T) {
	db, router := setupRouter()
	router.PUT("/notifications/config", withValidated(func() any { return &models.NotificationConfigInput{} }), SaveNotificationConfig)
	router.POST("/notifications/config/test", TestNotificationChannel)

	fake := newFakeNtfyServer(t)
	trueVal := true
	body, _ := json.Marshal(models.NotificationConfigInput{
		NtfyURL:    fake.URL(),
		NtfyTopic:  "alerts",
		NotifyNtfy: &trueVal,
	})
	req, _ := http.NewRequest("PUT", "/notifications/config", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	req, _ = http.NewRequest("POST", "/notifications/config/test", bytes.NewBufferString(`{"channel":"ntfy"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, true, result["ok"])
	assert.True(t, fake.wasHit(), "the test button must reach the ntfy server")

	// No reminder-scoped delivery rows are written by a test notification.
	var deliveries []models.NotificationDelivery
	require.NoError(t, db.Find(&deliveries).Error)
	assert.Empty(t, deliveries)
}

func TestNotificationConfig_TestUnconfiguredChannel(t *testing.T) {
	_, router := setupRouter()
	router.POST("/notifications/config/test", TestNotificationChannel)

	req, _ := http.NewRequest("POST", "/notifications/config/test", bytes.NewBufferString(`{"channel":"ntfy"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// A diagnosed failure is a successful response carrying the reason.
	assert.Equal(t, http.StatusOK, w.Code)
	var result map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, false, result["ok"])
	assert.Contains(t, result["error"], "not configured")
}

func TestNotificationConfig_TestUnknownChannel(t *testing.T) {
	_, router := setupRouter()
	router.POST("/notifications/config/test", TestNotificationChannel)

	req, _ := http.NewRequest("POST", "/notifications/config/test", bytes.NewBufferString(`{"channel":"sms"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPushSubscription_CRUD(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notifications/push-subscriptions", ListPushSubscriptions)
	router.POST("/notifications/push-subscriptions", withValidated(func() any { return &models.PushSubscriptionInput{} }), CreatePushSubscription)
	router.DELETE("/notifications/push-subscriptions/:id", DeletePushSubscription)

	body, _ := json.Marshal(models.PushSubscriptionInput{
		Endpoint:    "https://push.example.com/abc",
		P256dh:      "key1",
		Auth:        "auth1",
		DeviceLabel: "Desktop",
	})
	req, _ := http.NewRequest("POST", "/notifications/push-subscriptions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var created models.PushSubscription
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "https://push.example.com/abc", created.Endpoint)

	req, _ = http.NewRequest("GET", "/notifications/push-subscriptions", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Subscriptions []models.PushSubscription `json:"subscriptions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Subscriptions, 1)

	req, _ = http.NewRequest("DELETE", "/notifications/push-subscriptions/"+itoa(created.ID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var remaining int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Count(&remaining).Error)
	assert.Zero(t, remaining)
}

func TestPushSubscription_DeleteOwnershipScoping(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/notifications/push-subscriptions/:id", DeletePushSubscription)

	// The seeded "tester" is the current user. A subscription owned by a
	// different user must be untouchable (404) and survive the attempt.
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	others := models.PushSubscription{
		UserID:   other.ID,
		Endpoint: "https://push.example.com/other",
		P256dh:   "key",
		Auth:     "auth",
	}
	require.NoError(t, db.Create(&others).Error)

	req, _ := http.NewRequest("DELETE", "/notifications/push-subscriptions/"+itoa(others.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var stillThere models.PushSubscription
	require.NoError(t, db.First(&stillThere, others.ID).Error)
	assert.Equal(t, others.Endpoint, stillThere.Endpoint)
}

func TestPushSubscription_CreateOwnershipScoped(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notifications/push-subscriptions", ListPushSubscriptions)

	// Another user's subscription must not leak into the current user's list.
	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	others := models.PushSubscription{
		UserID:   other.ID,
		Endpoint: "https://push.example.com/other",
		P256dh:   "key",
		Auth:     "auth",
	}
	require.NoError(t, db.Create(&others).Error)

	req, _ := http.NewRequest("GET", "/notifications/push-subscriptions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Subscriptions []models.PushSubscription `json:"subscriptions"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Empty(t, listResp.Subscriptions, "a user must only ever see their own subscriptions")
}

func TestDeviceRegistration_CRUD(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notifications/devices", ListDeviceRegistrations)
	router.POST("/notifications/devices", withValidated(func() any { return &models.DeviceRegistrationInput{} }), CreateDeviceRegistration)
	router.DELETE("/notifications/devices/:id", DeleteDeviceRegistration)

	body, _ := json.Marshal(models.DeviceRegistrationInput{
		Token:       "fcm-token-123",
		Client:      "fcm",
		DeviceLabel: "Pixel 8",
	})
	req, _ := http.NewRequest("POST", "/notifications/devices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var created models.DeviceRegistration
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "fcm-token-123", created.Token)
	assert.Equal(t, "fcm", created.Client)

	req, _ = http.NewRequest("GET", "/notifications/devices", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Devices []models.DeviceRegistration `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	require.Len(t, listResp.Devices, 1)

	req, _ = http.NewRequest("DELETE", "/notifications/devices/"+itoa(created.ID), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var remaining int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Count(&remaining).Error)
	assert.Zero(t, remaining)
}

func TestDeviceRegistration_InvalidClientRejected(t *testing.T) {
	_, router := setupRouter()
	router.POST("/notifications/devices", middleware.ValidateJSONMiddleware(&models.DeviceRegistrationInput{}), CreateDeviceRegistration)

	// The middleware's oneof validator rejects an unknown push client — the
	// registration surface must not grow ad-hoc platform strings.
	body, _ := json.Marshal(models.DeviceRegistrationInput{
		Token:  "token",
		Client: "bogus",
	})
	req, _ := http.NewRequest("POST", "/notifications/devices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestDeviceRegistration_DeleteOwnershipScoping(t *testing.T) {
	db, router := setupRouter()
	router.DELETE("/notifications/devices/:id", DeleteDeviceRegistration)

	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	others := models.DeviceRegistration{
		UserID: other.ID,
		Token:  "fcm-other",
		Client: "fcm",
	}
	require.NoError(t, db.Create(&others).Error)

	req, _ := http.NewRequest("DELETE", "/notifications/devices/"+itoa(others.ID), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var stillThere models.DeviceRegistration
	require.NoError(t, db.First(&stillThere, others.ID).Error)
	assert.Equal(t, others.Token, stillThere.Token)
}

func TestDeviceRegistration_ListOwnershipScoped(t *testing.T) {
	db, router := setupRouter()
	router.GET("/notifications/devices", ListDeviceRegistrations)

	other := models.User{Username: "other", Password: "password123", Email: "other@example.com"}
	require.NoError(t, db.Create(&other).Error)
	others := models.DeviceRegistration{
		UserID: other.ID,
		Token:  "fcm-other",
		Client: "fcm",
	}
	require.NoError(t, db.Create(&others).Error)

	req, _ := http.NewRequest("GET", "/notifications/devices", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var listResp struct {
		Devices []models.DeviceRegistration `json:"devices"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Empty(t, listResp.Devices, "a user must only ever see their own device registrations")
}

// TestPushSubscription_OverCapRejected pins issue #415's per-user push-sub
// bound: once a user holds MaxPushSubscriptionsPerUser registrations, the
// next one is rejected with 409, and another user's quota is independent.
func TestPushSubscription_OverCapRejected(t *testing.T) {
	db, router := setupRouter()
	router.POST("/notifications/push-subscriptions", withValidated(func() any { return &models.PushSubscriptionInput{} }), CreatePushSubscription)

	var user models.User
	db.First(&user)

	for i := 0; i < MaxPushSubscriptionsPerUser; i++ {
		body, _ := json.Marshal(models.PushSubscriptionInput{
			Endpoint:    fmt.Sprintf("https://push.example.com/sub-%d", i),
			P256dh:      "key",
			Auth:        "auth",
			DeviceLabel: "Desktop",
		})
		req, _ := http.NewRequest("POST", "/notifications/push-subscriptions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code, "registrations up to the cap must succeed")
	}

	body, _ := json.Marshal(models.PushSubscriptionInput{
		Endpoint:    "https://push.example.com/over-cap",
		P256dh:      "key",
		Auth:        "auth",
		DeviceLabel: "Desktop",
	})
	req, _ := http.NewRequest("POST", "/notifications/push-subscriptions", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, "a registration beyond the cap must be rejected")

	var count int64
	require.NoError(t, db.Model(&models.PushSubscription{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(MaxPushSubscriptionsPerUser), count, "the over-cap registration must not be persisted")
}

// TestDeviceRegistration_OverCapRejected pins issue #415's per-user device
// bound, mirroring TestPushSubscription_OverCapRejected.
func TestDeviceRegistration_OverCapRejected(t *testing.T) {
	db, router := setupRouter()
	router.POST("/notifications/devices", withValidated(func() any { return &models.DeviceRegistrationInput{} }), CreateDeviceRegistration)

	var user models.User
	db.First(&user)

	for i := 0; i < MaxDeviceRegistrationsPerUser; i++ {
		body, _ := json.Marshal(models.DeviceRegistrationInput{
			Token:       fmt.Sprintf("fcm-token-%d", i),
			Client:      "fcm",
			DeviceLabel: "Pixel",
		})
		req, _ := http.NewRequest("POST", "/notifications/devices", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		assert.Equal(t, http.StatusCreated, w.Code, "registrations up to the cap must succeed")
	}

	body, _ := json.Marshal(models.DeviceRegistrationInput{
		Token:       "fcm-token-over-cap",
		Client:      "fcm",
		DeviceLabel: "Pixel",
	})
	req, _ := http.NewRequest("POST", "/notifications/devices", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code, "a registration beyond the cap must be rejected")

	var count int64
	require.NoError(t, db.Model(&models.DeviceRegistration{}).Where("user_id = ?", user.ID).Count(&count).Error)
	assert.Equal(t, int64(MaxDeviceRegistrationsPerUser), count, "the over-cap registration must not be persisted")
}

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
