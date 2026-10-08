package controllers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newFeedTestEnv builds a real migrated DB (dbtest.New — CLAUDE.md trap #1)
// with one seeded owner and a router that authenticates every request as that
// owner. frontendURL controls the one-time URL shape.
func newFeedTestEnv(t testing.TB, frontendURL string) (*gorm.DB, *gin.Engine, models.User, *config.Config) {
	t.Helper()
	gin.SetMode(gin.ReleaseMode)

	db := dbtest.New(t)
	owner := models.User{Username: "feedowner", Email: "feedowner@example.com", Password: "password123"}
	require.NoError(t, db.Create(&owner).Error)

	cfg := &config.Config{FrontendURL: frontendURL, ProfilePhotoDir: t.TempDir()}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", owner.ID)
		c.Set("username", owner.Username)
		c.Set("cfg", *cfg)
		c.Next()
	})

	router.GET("/feeds", ListFeeds)
	router.POST("/feeds", middleware.ValidateJSONMiddleware(&models.FeedInput{}), CreateFeed(cfg))
	router.POST("/feeds/revoke-all", RevokeAllFeeds)
	router.DELETE("/feeds/:id", DeleteFeed)
	router.POST("/feeds/:id/rotate", RotateFeed(cfg))

	return db, router, owner, cfg
}

// postFeed issues POST /feeds with the JSON body and returns the recorder.
func postFeed(router *gin.Engine, input models.FeedInput) *httptest.ResponseRecorder {
	body, _ := json.Marshal(input)
	req, _ := http.NewRequest("POST", "/feeds", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

// feedTokenFromURL extracts the plaintext token from a returned subscription
// URL.
func feedTokenFromURL(t *testing.T, raw string) string {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	tok := u.Query().Get("token")
	require.NotEmpty(t, tok, "url must carry a token query value")
	return tok
}

func hashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func TestCreateFeed_ContactKind_ReturnsURLAndStoresOnlyHash(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	contact := models.Contact{UserID: owner.ID, Firstname: "Ada", Lastname: "Lovelace"}
	require.NoError(t, db.Create(&contact).Error)

	w := postFeed(router, models.FeedInput{Name: "Ada feed", Kind: models.FeedKindContact, EntityID: contact.VCardUID})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.FeedCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "Ada feed", resp.Feed.Name)
	assert.Equal(t, models.FeedKindContact, resp.Feed.Kind)
	assert.Equal(t, contact.VCardUID, resp.Feed.EntityID)
	assert.Equal(t, models.FeedDetailHeadlines, resp.Feed.Detail, "missing detail must default to headlines")
	assert.True(t, len(resp.URL) > 0)
	assert.Contains(t, resp.URL, "https://crm.example/api/v1/feeds/atom?token=mycorrhizal_feed_")

	plaintext := feedTokenFromURL(t, resp.URL)

	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", resp.Feed.ID).Error)
	assert.Equal(t, hashToken(plaintext), stored.TokenHash, "stored hash must be the SHA-256 of the plaintext")
	assert.NotEqual(t, plaintext, stored.TokenHash, "the plaintext must never be stored")

	// The plaintext must appear nowhere in the row's persisted columns.
	assert.NotContains(t, stored.TokenHash, plaintext)
}

func TestCreateFeed_AggregateKind_ReturnsRelativeURLInDevMode(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "*")
	_ = owner

	w := postFeed(router, models.FeedInput{Name: "All", Kind: models.FeedKindAggregate, Detail: models.FeedDetailFull})
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.FeedCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, models.FeedKindAggregate, resp.Feed.Kind)
	assert.Equal(t, "", resp.Feed.EntityID)
	assert.Equal(t, models.FeedDetailFull, resp.Feed.Detail)
	assert.True(t, len(resp.URL) > 0)
	assert.Equal(t, "/api/v1/feeds/atom?token="+feedTokenFromURL(t, resp.URL), resp.URL)

	// The stored row exists and is unrevoked.
	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", resp.Feed.ID).Error)
	assert.Nil(t, stored.RevokedAt)
}

func TestCreateFeed_AnotherUsersContact_Returns404(t *testing.T) {
	db, router, _, _ := newFeedTestEnv(t, "https://crm.example")

	other := models.User{Username: "other", Email: "other@example.com", Password: "password123"}
	require.NoError(t, db.Create(&other).Error)
	otherContact := models.Contact{UserID: other.ID, Firstname: "Grace"}
	require.NoError(t, db.Create(&otherContact).Error)

	w := postFeed(router, models.FeedInput{Name: "Nope", Kind: models.FeedKindContact, EntityID: otherContact.VCardUID})
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestCreateFeed_AggregateWithEntityID_Returns400(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")
	_ = owner

	w := postFeed(router, models.FeedInput{Name: "Bad", Kind: models.FeedKindAggregate, EntityID: "11111111-1111-4111-8111-111111111111"})
	alAssertError(t, w, http.StatusBadRequest, "INVALID_INPUT", "entity_id")
	var n int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&n).Error)
	assert.Zero(t, n, "a rejected create must not insert a feed")
}

func TestCreateFeed_FiftyFirst_Returns422(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	for i := 0; i < models.MaxActiveFeedsPerUser; i++ {
		require.NoError(t, db.Create(&models.Feed{
			UserID: owner.ID, Name: fmt.Sprintf("feed-%d", i), Kind: models.FeedKindAggregate,
			Detail: models.FeedDetailHeadlines, TokenHash: fmt.Sprintf("hash-%d", i),
		}).Error)
	}

	w := postFeed(router, models.FeedInput{Name: "one too many", Kind: models.FeedKindAggregate})
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
}

func TestListFeeds_ExcludesRevoked(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	live := models.Feed{UserID: owner.ID, Name: "live", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "live-hash"}
	require.NoError(t, db.Create(&live).Error)
	revoked := models.Feed{UserID: owner.ID, Name: "revoked", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "revoked-hash"}
	require.NoError(t, db.Create(&revoked).Error)
	require.NoError(t, db.Model(&revoked).Update("revoked_at", time.Now()).Error)

	req, _ := http.NewRequest("GET", "/feeds", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Feeds []models.Feed `json:"feeds"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Feeds, 1)
	assert.Equal(t, live.ID, resp.Feeds[0].ID)

	// token_hash must never appear in any response body.
	assert.NotContains(t, w.Body.String(), "token_hash")
	assert.NotContains(t, w.Body.String(), "live-hash")
}

func TestRotateFeed_NewHashAndOldRevoked(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	old := models.Feed{UserID: owner.ID, Name: "rotate me", Kind: models.FeedKindAggregate, Detail: models.FeedDetailFull, TokenHash: "old-hash"}
	require.NoError(t, db.Create(&old).Error)

	req, _ := http.NewRequest("POST", "/feeds/"+old.ID+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	var resp models.FeedCreateResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.NotEqual(t, old.ID, resp.Feed.ID)
	assert.Equal(t, "rotate me", resp.Feed.Name)
	assert.Equal(t, models.FeedKindAggregate, resp.Feed.Kind)
	assert.Equal(t, models.FeedDetailFull, resp.Feed.Detail)

	newPlaintext := feedTokenFromURL(t, resp.URL)
	var newStored models.Feed
	require.NoError(t, db.First(&newStored, "id = ?", resp.Feed.ID).Error)
	assert.Equal(t, hashToken(newPlaintext), newStored.TokenHash)
	assert.NotEqual(t, "old-hash", newStored.TokenHash)

	var oldStored models.Feed
	require.NoError(t, db.First(&oldStored, "id = ?", old.ID).Error)
	assert.NotNil(t, oldStored.RevokedAt, "rotation must revoke the old row")
}

func TestRotateFeed_RevokedFeed_Returns404(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	revoked := models.Feed{UserID: owner.ID, Name: "gone", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "gone-hash"}
	require.NoError(t, db.Create(&revoked).Error)
	require.NoError(t, db.Model(&revoked).Update("revoked_at", time.Now()).Error)

	req, _ := http.NewRequest("POST", "/feeds/"+revoked.ID+"/rotate", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, w.Body.String())
}

func TestDeleteFeed_RevokesAndIsIdempotently404(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	feed := models.Feed{UserID: owner.ID, Name: "delete me", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "del-hash"}
	require.NoError(t, db.Create(&feed).Error)

	req, _ := http.NewRequest("DELETE", "/feeds/"+feed.ID, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusNoContent, w.Code, w.Body.String())

	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", feed.ID).Error)
	assert.NotNil(t, stored.RevokedAt)

	// Second delete is 404.
	req2, _ := http.NewRequest("DELETE", "/feeds/"+feed.ID, nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code)
}

func TestRevokeAllFeeds_CountsAndRevokes(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&models.Feed{
			UserID: owner.ID, Name: fmt.Sprintf("f%d", i), Kind: models.FeedKindAggregate,
			Detail: models.FeedDetailHeadlines, TokenHash: fmt.Sprintf("h-%d", i),
		}).Error)
	}
	already := models.Feed{UserID: owner.ID, Name: "already", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "already"}
	require.NoError(t, db.Create(&already).Error)
	require.NoError(t, db.Model(&already).Update("revoked_at", time.Now()).Error)

	req, _ := http.NewRequest("POST", "/feeds/revoke-all", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var resp struct {
		Revoked int64 `json:"revoked"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.EqualValues(t, 3, resp.Revoked)

	var active int64
	require.NoError(t, db.Model(&models.Feed{}).Where("user_id = ? AND revoked_at IS NULL", owner.ID).Count(&active).Error)
	assert.Zero(t, active)
}

func TestFeedRoutes_IDOR_BothReturn404AndOwnerFeedStaysActive(t *testing.T) {
	db, _, owner, _ := newFeedTestEnv(t, "https://crm.example")

	feed := models.Feed{UserID: owner.ID, Name: "owner feed", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "idor-hash"}
	require.NoError(t, db.Create(&feed).Error)

	intruder := models.User{Username: "intruder", Email: "intruder@example.com", Password: "password123"}
	require.NoError(t, db.Create(&intruder).Error)

	// A separate router authenticating as the intruder against the same DB.
	intruderRouter := gin.New()
	intruderRouter.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", intruder.ID)
		c.Set("cfg", config.Config{FrontendURL: "https://crm.example"})
		c.Next()
	})
	intruderRouter.DELETE("/feeds/:id", DeleteFeed)
	intruderRouter.POST("/feeds/:id/rotate", RotateFeed(&config.Config{FrontendURL: "https://crm.example"}))

	req, _ := http.NewRequest("DELETE", "/feeds/"+feed.ID, nil)
	w := httptest.NewRecorder()
	intruderRouter.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code, "another user's DELETE must 404")

	req2, _ := http.NewRequest("POST", "/feeds/"+feed.ID+"/rotate", nil)
	w2 := httptest.NewRecorder()
	intruderRouter.ServeHTTP(w2, req2)
	assert.Equal(t, http.StatusNotFound, w2.Code, "another user's rotate must 404")

	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", feed.ID).Error)
	assert.Nil(t, stored.RevokedAt, "the owner's feed must stay active")
}

// --- revocation sites ------------------------------------------------------

func TestConfirmPasswordReset_RevokesExistingFeeds(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	live := models.Feed{UserID: owner.ID, Name: "live", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "reset-live"}
	require.NoError(t, db.Create(&live).Error)
	already := models.Feed{UserID: owner.ID, Name: "already", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "reset-already"}
	require.NoError(t, db.Create(&already).Error)
	priorRevoke := time.Now().Add(-24 * time.Hour)
	require.NoError(t, db.Model(&already).Update("revoked_at", priorRevoke).Error)

	token, tokenHash, _ := services.GeneratePasswordResetToken()
	expires := time.Now().Add(time.Hour)
	requested := time.Now()
	require.NoError(t, db.Model(&owner).Updates(map[string]any{
		"password_reset_token_hash":   tokenHash,
		"password_reset_expires_at":   expires,
		"password_reset_requested_at": requested,
	}).Error)

	router.POST("/password-reset/confirm", func(c *gin.Context) {
		c.Set("validated", &models.PasswordResetConfirmInput{Token: token, Password: "NewPassw0rd!xY"})
		ConfirmPasswordReset(c, &config.Config{})
	})
	req, _ := http.NewRequest("POST", "/password-reset/confirm", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var updatedLive models.Feed
	require.NoError(t, db.First(&updatedLive, "id = ?", live.ID).Error)
	assert.NotNil(t, updatedLive.RevokedAt, "a live feed must be revoked by a password reset")

	var updatedAlready models.Feed
	require.NoError(t, db.First(&updatedAlready, "id = ?", already.ID).Error)
	require.NotNil(t, updatedAlready.RevokedAt)
	assert.WithinDuration(t, priorRevoke, *updatedAlready.RevokedAt, time.Second,
		"an already-revoked feed's original revocation time must not be overwritten")
}

func TestAdminPasswordReset_RevokesFeeds(t *testing.T) {
	db, router, _, _ := newFeedTestEnv(t, "https://crm.example")

	target := models.User{Username: "target", Email: "target@example.com", Password: "password123"}
	require.NoError(t, db.Create(&target).Error)
	feed := models.Feed{UserID: target.ID, Name: "target feed", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "admin-reset"}
	require.NoError(t, db.Create(&feed).Error)

	router.PATCH("/users/:id", withValidated(func() any { return &models.AdminUserUpdateInput{} }), UpdateUser)

	newPassword := "brandNewPassw0rd!"
	body, _ := json.Marshal(models.AdminUserUpdateInput{Password: &newPassword})
	req, _ := http.NewRequest("PATCH", "/users/"+strconv.Itoa(int(target.ID)), bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", feed.ID).Error)
	assert.NotNil(t, stored.RevokedAt, "an admin password reset must revoke the user's feeds")
}

func TestChangePassword_DoesNotRevokeFeeds(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")
	hashed, _ := services.HashPassword("password123")
	require.NoError(t, db.Model(&owner).Update("password", hashed).Error)

	feed := models.Feed{UserID: owner.ID, Name: "keep", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "change-keep"}
	require.NoError(t, db.Create(&feed).Error)

	router.POST("/change-password", func(c *gin.Context) {
		c.Set("username", owner.Username)
		c.Set("validated", &models.ChangePasswordInput{CurrentPassword: "password123", NewPassword: "NewPassw0rd!xY"})
		ChangePassword(c, &config.Config{})
	})
	req, _ := http.NewRequest("POST", "/change-password", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var stored models.Feed
	require.NoError(t, db.First(&stored, "id = ?", feed.ID).Error)
	assert.Nil(t, stored.RevokedAt, "self-service ChangePassword must leave feeds alone")
}

func TestDeleteContact_RevokesOnlyThatContactsFeeds(t *testing.T) {
	db, router, owner, _ := newFeedTestEnv(t, "https://crm.example")

	cA := models.Contact{UserID: owner.ID, Firstname: "Alice"}
	require.NoError(t, db.Create(&cA).Error)
	cB := models.Contact{UserID: owner.ID, Firstname: "Bob"}
	require.NoError(t, db.Create(&cB).Error)

	feedA := models.Feed{UserID: owner.ID, Name: "A", Kind: models.FeedKindContact, EntityID: cA.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "contact-a"}
	require.NoError(t, db.Create(&feedA).Error)
	feedB := models.Feed{UserID: owner.ID, Name: "B", Kind: models.FeedKindContact, EntityID: cB.VCardUID, Detail: models.FeedDetailHeadlines, TokenHash: "contact-b"}
	require.NoError(t, db.Create(&feedB).Error)
	aggregate := models.Feed{UserID: owner.ID, Name: "All", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "contact-agg"}
	require.NoError(t, db.Create(&aggregate).Error)

	router.DELETE("/contacts/:id", DeleteContact)
	req, _ := http.NewRequest("DELETE", "/contacts/"+strconv.Itoa(int(cA.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var gotA, gotB, gotAgg models.Feed
	require.NoError(t, db.First(&gotA, "id = ?", feedA.ID).Error)
	require.NoError(t, db.First(&gotB, "id = ?", feedB.ID).Error)
	require.NoError(t, db.First(&gotAgg, "id = ?", aggregate.ID).Error)
	assert.NotNil(t, gotA.RevokedAt, "the deleted contact's feed must be revoked")
	assert.Nil(t, gotB.RevokedAt, "another contact's feed must stay active")
	assert.Nil(t, gotAgg.RevokedAt, "the aggregate feed must stay active")
}

func TestDeleteUser_HardDeletesFeeds(t *testing.T) {
	db, router, _, _ := newFeedTestEnv(t, "https://crm.example")

	target := models.User{Username: "target-del", Email: "target-del@example.com", Password: "password123"}
	require.NoError(t, db.Create(&target).Error)
	require.NoError(t, db.Create(&models.Feed{UserID: target.ID, Name: "f", Kind: models.FeedKindAggregate, Detail: models.FeedDetailHeadlines, TokenHash: "deluser"}).Error)

	router.DELETE("/users/:id", DeleteUser)
	req, _ := http.NewRequest("DELETE", "/users/"+strconv.Itoa(int(target.ID)), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	// Raw count so a future deleted_at column cannot make a soft-delete look
	// like a hard delete (CLAUDE.md trap #6).
	var rawCount int64
	require.NoError(t, db.Raw("SELECT COUNT(*) FROM feeds WHERE user_id = ?", target.ID).Scan(&rawCount).Error)
	assert.Zero(t, rawCount, "DeleteUser must leave zero feeds rows")

	var scopedCount int64
	require.NoError(t, db.Unscoped().Model(&models.Feed{}).Where("user_id = ?", target.ID).Count(&scopedCount).Error)
	assert.Zero(t, scopedCount)
}

func TestListFeeds_EmptyIsArrayNotNull(t *testing.T) {
	_, router, _, _ := newFeedTestEnv(t, "https://crm.example")

	req, _ := http.NewRequest("GET", "/feeds", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.JSONEq(t, `{"feeds":[]}`, w.Body.String())
}

func TestFeedHandlers_RequireAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	cfg := &config.Config{FrontendURL: "https://crm.example"}

	owner := models.User{Username: "feedunauth", Email: "feedunauth@example.com", Password: "password123"}
	require.NoError(t, db.Create(&owner).Error)
	feed := models.Feed{UserID: owner.ID, Name: "keep", Kind: models.FeedKindAggregate, Detail: "headlines", TokenHash: "feedunauth-hash"}
	require.NoError(t, db.Create(&feed).Error)

	// A router with the DB but no authenticated userID in context.
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	router.GET("/feeds", ListFeeds)
	router.POST("/feeds", middleware.ValidateJSONMiddleware(&models.FeedInput{}), CreateFeed(cfg))
	router.POST("/feeds/:id/rotate", RotateFeed(cfg))
	router.DELETE("/feeds/:id", DeleteFeed)
	router.POST("/feeds/revoke-all", RevokeAllFeeds)

	body := `{"name":"x","kind":"aggregate"}`
	for _, tc := range []struct{ method, path, body string }{
		{"GET", "/feeds", ""},
		{"POST", "/feeds", body},
		{"POST", "/feeds/x/rotate", ""},
		{"DELETE", "/feeds/x", ""},
		{"POST", "/feeds/revoke-all", ""},
	} {
		req, _ := http.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		alAssertError(t, w, http.StatusUnauthorized, "UNAUTHORIZED", "")
	}
	var n int64
	require.NoError(t, db.Model(&models.Feed{}).Count(&n).Error)
	assert.EqualValues(t, 1, n, "unauthenticated requests must not create feeds")
	var after models.Feed
	require.NoError(t, db.First(&after, "id = ?", feed.ID).Error)
	assert.Nil(t, after.RevokedAt, "unauthenticated requests must not revoke feeds")
	assert.Equal(t, feed.TokenHash, after.TokenHash, "unauthenticated requests must not rotate feeds")
}

func TestCreateFeed_MissingValidatedInput(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	db := dbtest.New(t)
	user := models.User{Username: "nobody", Email: "nobody@example.com", Password: "password123"}
	require.NoError(t, db.Create(&user).Error)
	cfg := &config.Config{FrontendURL: "https://crm.example"}

	// No ValidateJSONMiddleware ran, so GetValidated finds nothing.
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Set("userID", user.ID)
		c.Next()
	})
	router.POST("/feeds", CreateFeed(cfg))

	req, _ := http.NewRequest("POST", "/feeds", strings.NewReader(`{"name":"x","kind":"aggregate"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())
}

func TestIfNoneMatchMatches(t *testing.T) {
	etag := `"abc123"`
	assert.True(t, ifNoneMatchMatches("*", etag))
	assert.True(t, ifNoneMatchMatches(etag, etag))
	assert.True(t, ifNoneMatchMatches("W/"+etag, etag))
	assert.True(t, ifNoneMatchMatches(`"nope", `+etag, etag))
	assert.False(t, ifNoneMatchMatches("", etag))
	assert.False(t, ifNoneMatchMatches(`"nope"`, etag))
}
