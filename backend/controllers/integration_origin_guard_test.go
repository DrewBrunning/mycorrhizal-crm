package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func putJSON(t *testing.T, router *gin.Engine, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, _ := http.NewRequest("PUT", path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// Credential-exfiltration guard for the per-user integrations that store a
// write-only secret beside a base URL: an empty secret means "keep the stored
// one", so moving the URL to a different origin without re-entering it is a 400
// on the secret field and changes nothing; a path-only change keeps the secret.
func TestSaveIntegrationConfig_OriginChangeRequiresSecret(t *testing.T) {
	cases := []struct {
		name      string
		path      string
		secretKey string
		secretMsg string
		router    func(t *testing.T) (*gin.Engine, *gorm.DB)
		storedURL func(db *gorm.DB) string
	}{
		{
			name: "immich", path: "/immich/config", secretKey: "api_key", secretMsg: "re-enter the API key",
			router: func(t *testing.T) (*gin.Engine, *gorm.DB) {
				db := seedImmichControllerDB(t)
				return immichTestRouter(t, db), db
			},
			storedURL: func(db *gorm.DB) string {
				var s models.ImmichConfig
				require.NoError(t, db.Where("user_id = ?", uint(1)).First(&s).Error)
				return s.BaseURL
			},
		},
		{
			name: "paperless", path: "/paperless/config", secretKey: "api_token", secretMsg: "re-enter the API token",
			router: func(t *testing.T) (*gin.Engine, *gorm.DB) {
				db := seedPaperlessControllerDB(t)
				return paperlessTestRouter(t, db), db
			},
			storedURL: func(db *gorm.DB) string {
				var s models.PaperlessConfig
				require.NoError(t, db.Where("user_id = ?", uint(1)).First(&s).Error)
				return s.BaseURL
			},
		},
		{
			name: "seafile", path: "/seafile/config", secretKey: "api_token", secretMsg: "re-enter the API token",
			router: func(t *testing.T) (*gin.Engine, *gorm.DB) {
				db := seedSeafileControllerDB(t)
				return seafileTestRouter(t, db), db
			},
			storedURL: func(db *gorm.DB) string {
				var s models.SeafileConfig
				require.NoError(t, db.Where("user_id = ?", uint(1)).First(&s).Error)
				return s.BaseURL
			},
		},
		{
			name: "nextcloud", path: "/nextcloud/config", secretKey: "app_password", secretMsg: "re-enter the app password",
			router: func(t *testing.T) (*gin.Engine, *gorm.DB) {
				db := seedWebDAVControllerDB(t)
				return webdavTestRouter(t, db), db
			},
			storedURL: func(db *gorm.DB) string {
				var s models.WebDAVConfig
				require.NoError(t, db.Where("user_id = ?", uint(1)).First(&s).Error)
				return s.BaseURL
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			router, db := tc.router(t)
			save := func(url, secret string) *httptest.ResponseRecorder {
				body := map[string]any{"base_url": url, "username": "alice"}
				if secret != "" {
					body[tc.secretKey] = secret
				}
				return putJSON(t, router, tc.path, body)
			}
			require.Equal(t, http.StatusOK, save("https://svc.example", "first-secret").Code)

			w := save("https://attacker.example", "")
			require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
			assert.Contains(t, w.Body.String(), tc.secretKey)
			assert.Contains(t, w.Body.String(), tc.secretMsg)
			assert.Equal(t, "https://svc.example", tc.storedURL(db), "a refused save must change nothing")

			// Same origin, different path keeps the stored secret.
			require.Equal(t, http.StatusOK, save("https://SVC.example:443/sub", "").Code)

			// Re-entering the secret makes the origin change legal.
			require.Equal(t, http.StatusOK, save("https://other.example", "second-secret").Code)
			assert.Equal(t, "https://other.example", tc.storedURL(db))
		})
	}
}

// The same rule for calendar and contact subscriptions, whose password is sent
// as Basic auth to the subscription URL on every sync.
func TestUpdateSubscription_OriginChangeRequiresPassword(t *testing.T) {
	cfg := config.Config{JWTSecretKey: "test-jwt-secret-key-with-32-chars!!"}
	enc, err := services.EncryptCredential(cfg.JWTSecretKey, "hunter2")
	require.NoError(t, err)

	t.Run("calendar", func(t *testing.T) {
		db, _ := setupRouter(t)
		var user models.User
		db.First(&user)
		sub := models.CalendarSubscription{UserID: user.ID, Name: "Sub", URL: "https://cal.example/a/", PasswordEncrypted: enc, SyncEnabled: true}
		require.NoError(t, db.Create(&sub).Error)
		router := routerForUser(db, user.ID)
		router.Use(func(c *gin.Context) { c.Set("cfg", cfg); c.Next() })
		router.PUT("/calendars/:id", withValidated(func() any { return &models.CalendarSubscriptionInput{} }), UpdateCalendarSubscription)
		path := "/calendars/" + strconv.Itoa(int(sub.ID))

		w := putJSON(t, router, path, models.CalendarSubscriptionInput{Name: "Sub", URL: "https://attacker.example/a/"})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "password")
		var stored models.CalendarSubscription
		require.NoError(t, db.First(&stored, sub.ID).Error)
		assert.Equal(t, "https://cal.example/a/", stored.URL)

		// webcal:// normalizes to https://, so it is a same-origin path change.
		w = putJSON(t, router, path, models.CalendarSubscriptionInput{Name: "Sub", URL: "webcal://cal.example/b/"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = putJSON(t, router, path, models.CalendarSubscriptionInput{Name: "Sub", URL: "https://other.example/a/", Password: "new"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		// Clearing the password is also a legal way to change the origin.
		w = putJSON(t, router, path, models.CalendarSubscriptionInput{Name: "Sub", URL: "https://third.example/a/", ClearPassword: true})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})

	t.Run("contacts", func(t *testing.T) {
		db, _ := setupRouter(t)
		var user models.User
		db.First(&user)
		sub := models.ContactSubscription{UserID: user.ID, Name: "Sub", URL: "https://dav.example/book/", PasswordEncrypted: enc, SyncEnabled: true}
		require.NoError(t, db.Create(&sub).Error)
		router := routerForUser(db, user.ID)
		router.Use(func(c *gin.Context) { c.Set("cfg", cfg); c.Next() })
		router.PUT("/contact-subscriptions/:id", withValidated(func() any { return &models.ContactSubscriptionInput{} }), UpdateContactSubscription)
		path := "/contact-subscriptions/" + strconv.Itoa(int(sub.ID))

		w := putJSON(t, router, path, models.ContactSubscriptionInput{Name: "Sub", URL: "https://attacker.example/book/"})
		require.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
		assert.Contains(t, w.Body.String(), "password")
		var stored models.ContactSubscription
		require.NoError(t, db.First(&stored, sub.ID).Error)
		assert.Equal(t, "https://dav.example/book/", stored.URL)

		w = putJSON(t, router, path, models.ContactSubscriptionInput{Name: "Sub", URL: "https://dav.example/other/"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		w = putJSON(t, router, path, models.ContactSubscriptionInput{Name: "Sub", URL: "https://other.example/book/", Password: "new"})
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	})
}
