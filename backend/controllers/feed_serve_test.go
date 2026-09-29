package controllers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// serveTestEnv builds a router exposing only the unauthenticated feed-serving
// endpoint, plus the DB it reads.
func serveTestEnv(t testing.TB, cfg *config.Config) (*gorm.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.GET("/api/v1/feeds/atom", middleware.FeedRateLimitMiddleware(), ServeFeed(cfg))
	return db, router
}

func seedServeUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	u := models.User{Username: "serve-user", Email: "serve-user@example.com", Password: "password123", Language: "en"}
	require.NoError(t, db.Create(&u).Error)
	return u
}

// servedFeedSeq keeps each minted feed token unique (token_hash is UNIQUE).
var servedFeedSeq int

// mintServedFeed creates a feed with a known plaintext token and returns both.
func mintServedFeed(t *testing.T, db *gorm.DB, userID uint, kind, entityID string) (*models.Feed, string) {
	t.Helper()
	servedFeedSeq++
	plaintext := fmt.Sprintf("mycorrhizal_feed_%043d", servedFeedSeq)
	sum := sha256.Sum256([]byte(plaintext))
	hash := hex.EncodeToString(sum[:])
	feed := models.Feed{UserID: userID, Name: "serve feed", Kind: kind, EntityID: entityID, Detail: models.FeedDetailHeadlines, TokenHash: hash}
	require.NoError(t, db.Create(&feed).Error)
	return &feed, plaintext
}

// awaitFeedTouch waits for ServeFeed's fire-and-forget TouchFeed goroutine to
// land, so it cannot outlive the test and race with the global-logger swap in
// TestServeFeed_RequestLogRedactsToken (or with a test-DB close).
func awaitFeedTouch(t *testing.T, db *gorm.DB, feedID string) {
	t.Helper()
	require.Eventually(t, func() bool {
		var f models.Feed
		if err := db.First(&f, "id = ?", feedID).Error; err != nil {
			return false
		}
		return f.LastAccessedAt != nil
	}, 2*time.Second, 10*time.Millisecond, "a serve must touch last_accessed_at")
}

func atomGet(router *gin.Engine, rawurl string, headers map[string]string) *httptest.ResponseRecorder {
	req, _ := http.NewRequest("GET", rawurl, nil)
	req.RemoteAddr = "203.0.113.9:1234"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestServeFeed_SuccessHeadersAndConditionalGet(t *testing.T) {
	cfg := &config.Config{FrontendURL: "https://crm.example"}
	db, router := serveTestEnv(t, cfg)
	user := seedServeUser(t, db)
	contact := models.Contact{UserID: user.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&contact).Error)
	feed, plaintext := mintServedFeed(t, db, user.ID, models.FeedKindContact, contact.VCardUID)

	w := atomGet(router, "/api/v1/feeds/atom?token="+plaintext, nil)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, "application/atom+xml; charset=utf-8", w.Header().Get("Content-Type"))
	assert.Equal(t, "noindex, nofollow, noarchive", w.Header().Get("X-Robots-Tag"))
	etag := w.Header().Get("ETag")
	require.NotEmpty(t, etag)
	assert.True(t, strings.HasPrefix(etag, `"`) && strings.HasSuffix(etag, `"`), "strong ETag")
	body := w.Body.Bytes()
	assert.Equal(t, `"`+fmt.Sprintf("%x", sha256.Sum256(body))+`"`, etag, "ETag is the SHA-256 of the body")

	// The token must not be echoed into the body.
	assert.NotContains(t, string(body), plaintext)

	// Let the async touch land before the later requests (the row then carries
	// a fresh last_accessed_at, so they take the throttled no-goroutine path).
	awaitFeedTouch(t, db, feed.ID)

	// A matching If-None-Match is a 304 with no body.
	w2 := atomGet(router, "/api/v1/feeds/atom?token="+plaintext, map[string]string{"If-None-Match": etag})
	require.Equal(t, http.StatusNotModified, w2.Code)
	assert.Zero(t, w2.Body.Len())

	// Weak validators also match.
	w3 := atomGet(router, "/api/v1/feeds/atom?token="+plaintext, map[string]string{"If-None-Match": "W/" + etag})
	require.Equal(t, http.StatusNotModified, w3.Code)

	// A non-matching validator re-renders.
	w4 := atomGet(router, "/api/v1/feeds/atom?token="+plaintext, map[string]string{"If-None-Match": `"nope"`})
	require.Equal(t, http.StatusOK, w4.Code)
}

func TestServeFeed_EveryMissIsIdenticalEmpty404(t *testing.T) {
	cfg := &config.Config{FrontendURL: "https://crm.example"}
	db, router := serveTestEnv(t, cfg)
	user := seedServeUser(t, db)

	// A live feed, plus a revoked one.
	live, liveToken := mintServedFeed(t, db, user.ID, models.FeedKindAggregate, "")
	revoked, revokedToken := mintServedFeed(t, db, user.ID, models.FeedKindAggregate, "")
	require.NoError(t, db.Model(revoked).Update("revoked_at", time.Now()).Error)

	// A contact feed whose contact is soft-deleted.
	contact := models.Contact{UserID: user.ID, Firstname: "Gone"}
	require.NoError(t, db.Create(&contact).Error)
	_, contactToken := mintServedFeed(t, db, user.ID, models.FeedKindContact, contact.VCardUID)
	require.NoError(t, db.Delete(&contact).Error)

	// A feed owned by a soft-deleted user.
	ghost := models.User{Username: "ghost", Email: "ghost@example.com", Password: "password123"}
	require.NoError(t, db.Create(&ghost).Error)
	_, ghostToken := mintServedFeed(t, db, ghost.ID, models.FeedKindAggregate, "")
	require.NoError(t, db.Delete(&ghost).Error)

	// An API token in ?token= is a miss (wrong prefix).
	apiPlain := "mycorrhizal_" + strings.Repeat("B", 43)
	apiSum := sha256.Sum256([]byte(apiPlain))
	require.NoError(t, db.Create(&models.ApiToken{UserID: user.ID, Name: "api", TokenHash: hex.EncodeToString(apiSum[:])}).Error)

	liveServes := atomGet(router, "/api/v1/feeds/atom?token="+liveToken, nil)
	require.Equal(t, http.StatusOK, liveServes.Code, "the live feed must resolve (control)")
	awaitFeedTouch(t, db, live.ID)

	cases := []struct {
		name  string
		path  string
		token string
	}{
		{"missing-token", "/api/v1/feeds/atom", ""},
		{"unknown-token", "/api/v1/feeds/atom", "mycorrhizal_feed_" + strings.Repeat("Z", 43)},
		{"wrong-prefix", "/api/v1/feeds/atom", "not-a-feed-token"},
		{"jwt-as-token", "/api/v1/feeds/atom", "eyJhbGciOiJIUzI1NiJ9.e30.x"},
		{"api-token", "/api/v1/feeds/atom", apiPlain},
		{"revoked", "/api/v1/feeds/atom", revokedToken},
		{"soft-deleted-contact", "/api/v1/feeds/atom", contactToken},
		{"soft-deleted-user", "/api/v1/feeds/atom", ghostToken},
	}
	var firstBody []byte
	for i, tc := range cases {
		w := atomGet(router, tc.path, nil)
		if tc.token != "" {
			w = atomGet(router, tc.path+"?token="+tc.token, nil)
		}
		require.Equalf(t, http.StatusNotFound, w.Code, "%s must 404", tc.name)
		require.Zerof(t, w.Body.Len(), "%s must have an empty body", tc.name)
		if i == 0 {
			firstBody = w.Body.Bytes()
			continue
		}
		assert.Equal(t, firstBody, w.Body.Bytes(), "%s body must be identical to the others", tc.name)
	}
}

func TestServeFeed_RequestLogRedactsToken(t *testing.T) {
	// Capture the process logger (LoggingMiddleware writes through it).
	var buf bytes.Buffer
	oldLogger := logger.Logger
	oldLevel := zerolog.GlobalLevel()
	logger.Logger = zerolog.New(&buf)
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	t.Cleanup(func() {
		logger.Logger = oldLogger
		zerolog.SetGlobalLevel(oldLevel)
	})

	cfg := &config.Config{FrontendURL: "https://crm.example"}
	db := dbtest.New(t)
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	router.Use(middleware.LoggingMiddleware())
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("cfg", *cfg)
		c.Next()
	})
	router.GET("/api/v1/feeds/atom", ServeFeed(cfg))

	user := models.User{Username: "log-user", Email: "log-user@example.com", Password: "password123"}
	require.NoError(t, db.Create(&user).Error)
	feed, plaintext := mintServedFeed(t, db, user.ID, models.FeedKindAggregate, "")

	w := atomGet(router, "/api/v1/feeds/atom?token="+plaintext, nil)
	require.Equal(t, http.StatusOK, w.Code)

	// Join the async TouchFeed goroutine before the test's cleanup restores the
	// global logger, or it races that write.
	awaitFeedTouch(t, db, feed.ID)

	logs := buf.String()
	require.NotEmpty(t, logs)
	assert.NotContains(t, logs, plaintext, "the plaintext feed token must never reach the request log")
	assert.Contains(t, logs, "token=[REDACTED]", "the token query value must be redacted")
}
