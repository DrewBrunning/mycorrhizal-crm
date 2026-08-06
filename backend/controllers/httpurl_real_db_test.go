package controllers

import (
	"bytes"
	"encoding/json"
	"mycorrhizal/config"
	"mycorrhizal/database"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTTPURLAllowlist_RealMigratedSchema is T41's real-DB check. Every one of
// the four web-link fields that moved from the `safeurl` blocklist to the
// `httpurl` allowlist must reject a previously-accepted non-http scheme with a
// 400 at the binding layer, and still accept a plain http(s) value. It uses
// the real ValidateJSONMiddleware (not the withValidated test shim, which
// skips struct-tag validation) against a database.InitDB-migrated real file
// database, per CLAUDE.md trap 1.
//
// The "rejected" schemes are deliberately ones `safeurl` used to allow —
// mailto:, intent:, ftp: — not ones it already blocked. A javascript: value
// would have been rejected before this ticket too, and proves nothing about
// the new validator.
func TestHTTPURLAllowlist_RealMigratedSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "httpurl-real.db")
	db, err := database.InitDB(dbPath)
	require.NoError(t, err)

	user := models.User{Username: "httpurl-realdb", Password: "password123!A", Email: "httpurl-realdb@example.com"}
	require.NoError(t, db.Create(&user).Error)
	contact := models.Contact{UserID: user.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&contact).Error)

	gin.SetMode(gin.ReleaseMode)
	router := gin.Default()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Set("cfg", config.Config{JWTSecretKey: "test-jwt-secret-0123456789abcdef0123456789abcdef"})
		c.Next()
	})
	router.POST("/gifts", middleware.ValidateJSONMiddleware(&models.GiftInput{}), CreateGift)
	router.POST("/conversation-agenda", middleware.ValidateJSONMiddleware(&models.ConversationAgendaInput{}), CreateConversationAgenda)
	router.POST("/external-identities", middleware.ValidateJSONMiddleware(&models.ExternalIdentityInput{}), CreateExternalIdentity)
	router.PUT("/immich/config", middleware.ValidateJSONMiddleware(&models.ImmichConfigInput{}), SaveImmichConfig)

	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&buf).Encode(body))
		}
		req, _ := http.NewRequest(method, path, &buf)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	cases := []struct {
		name             string
		expectedAccepted int
		accepted         func() *httptest.ResponseRecorder
		rejected         func() *httptest.ResponseRecorder
	}{
		{
			name:             "gift URL",
			expectedAccepted: http.StatusCreated,
			accepted: func() *httptest.ResponseRecorder {
				return do("POST", "/gifts", models.GiftInput{
					EntityID: contact.VCardUID, Description: "The mug", URL: "https://shop.example.com/mug",
				})
			},
			rejected: func() *httptest.ResponseRecorder {
				return do("POST", "/gifts", models.GiftInput{
					EntityID: contact.VCardUID, Description: "The mug", URL: "mailto:a@b.com",
				})
			},
		},
		{
			name:             "agenda reference_url",
			expectedAccepted: http.StatusCreated,
			accepted: func() *httptest.ResponseRecorder {
				return do("POST", "/conversation-agenda", models.ConversationAgendaInput{
					EntityID: contact.VCardUID, Content: "Ask about the article", ReferenceURL: "https://example.com/article",
				})
			},
			rejected: func() *httptest.ResponseRecorder {
				return do("POST", "/conversation-agenda", models.ConversationAgendaInput{
					EntityID: contact.VCardUID, Content: "Ask about the article", ReferenceURL: "intent://example.com/#Intent;scheme=zebra;end",
				})
			},
		},
		{
			name:             "external identity URL",
			expectedAccepted: http.StatusCreated,
			accepted: func() *httptest.ResponseRecorder {
				return do("POST", "/external-identities", models.ExternalIdentityInput{
					EntityID: contact.VCardUID, System: "paperless", ExternalID: "doc-1", URL: "https://paperless.example/documents/1",
				})
			},
			rejected: func() *httptest.ResponseRecorder {
				return do("POST", "/external-identities", models.ExternalIdentityInput{
					EntityID: contact.VCardUID, System: "paperless", ExternalID: "doc-2", URL: "ftp://paperless.example/documents/2",
				})
			},
		},
		{
			name:             "immich base_url",
			expectedAccepted: http.StatusOK,
			accepted: func() *httptest.ResponseRecorder {
				return do("PUT", "/immich/config", models.ImmichConfigInput{BaseURL: "https://immich.example", APIKey: "key-1"})
			},
			rejected: func() *httptest.ResponseRecorder {
				return do("PUT", "/immich/config", models.ImmichConfigInput{BaseURL: "ftp://immich.example", APIKey: "key-1"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			acceptedResp := tc.accepted()
			assert.Equal(t, tc.expectedAccepted, acceptedResp.Code, "an http(s) value must still be accepted: %s", acceptedResp.Body.String())

			rejectedResp := tc.rejected()
			assert.Equal(t, http.StatusBadRequest, rejectedResp.Code, "a previously-accepted non-http scheme must now be rejected: %s", rejectedResp.Body.String())
		})
	}
}
