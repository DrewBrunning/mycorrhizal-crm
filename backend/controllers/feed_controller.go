package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strings"

	apperrors "mycorrhizal/errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// generateFeedToken mints a new plaintext feed token ("mycorrhizal_feed_" + 32
// random bytes, base64url) and its SHA-256 hex for storage. It mirrors
// generateApiToken exactly; the distinct prefix keeps the two credential
// spaces disjoint (AuthMiddleware routes mycorrhizal_ bearers to
// LookupAPIToken, which only searches api_tokens, so a feed token is a 401
// there; the feed endpoint only searches feeds.token_hash, so an API token is
// a 404 there). ADR 0030 decision 6.
func generateFeedToken() (plaintext, hash string, err error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", "", err // # pragma: no cover — DB/rand failure only; the success path and 400/404/422 branches are covered
	}
	plaintext = "mycorrhizal_feed_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	hash = fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	return plaintext, hash, nil
}

// feedURL builds the one-time subscription URL returned by create and rotate.
// When FrontendURL is an absolute origin the URL is absolute; when it is the
// dev sentinel "*" (or empty) only the path+query is returned and the client
// prefixes window.location.origin. ADR 0030 decision 8.
func feedURL(cfg *config.Config, plaintext string) string {
	path := "/api/v1/feeds/atom?token=" + plaintext
	if cfg.FrontendURL == "*" {
		return path
	}
	return cfg.FrontendURL + path
}

// ListFeeds returns the caller's unrevoked feeds, newest first. There is no
// pagination; creation is capped at MaxActiveFeedsPerUser so the list is
// bounded by construction.
func ListFeeds(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var feeds []models.Feed
	if err := db.Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("created_at DESC").
		Find(&feeds).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query")) // # pragma: no cover — DB failure only; the empty/non-empty list paths are covered
		return                                                      // # pragma: no cover — DB failure only
	}

	// Never nil so the response serializes `[]`, not null (frontend trap 8).
	if feeds == nil {
		feeds = []models.Feed{} // # pragma: no cover — defensive: GORM Find already yields a non-nil slice here, kept so the response can never be null (frontend trap 8)
	}
	c.JSON(http.StatusOK, gin.H{"feeds": feeds})
}

// CreateFeed mints a feed credential. The plaintext token is returned once,
// embedded in the subscription URL. See ADR 0030 decisions 6 and 8.
func CreateFeed(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		db := c.MustGet("db").(*gorm.DB)
		userID, ok := currentUserID(c)
		if !ok {
			return
		}

		input, appErr := middleware.GetValidated[models.FeedInput](c)
		if appErr != nil {
			apperrors.AbortWithError(c, appErr)
			return
		}

		detail := input.Detail
		if detail == "" {
			detail = models.FeedDetailHeadlines
		}

		if !resolveFeedEntity(c, db, userID, input) {
			return
		}

		var active int64
		if err := db.Model(&models.Feed{}).
			Where("user_id = ? AND revoked_at IS NULL", userID).
			Count(&active).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("query")) // # pragma: no cover — DB failure only; the 50-feed cap (422) path is covered
			return                                                      // # pragma: no cover — DB failure only
		}
		if active >= models.MaxActiveFeedsPerUser {
			apperrors.AbortWithError(c, apperrors.ErrBusinessLogic(
				fmt.Sprintf("feed limit reached (maximum %d active feeds)", models.MaxActiveFeedsPerUser)))
			return
		}

		plaintext, hash, err := generateFeedToken()
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInternal("token generation failed")) // # pragma: no cover — token-mint failure only (crypto/rand)
			return                                                                        // # pragma: no cover — token-mint failure only (crypto/rand)
		}

		feed := models.Feed{
			UserID:    userID,
			Name:      input.Name,
			Kind:      input.Kind,
			EntityID:  feedEntityID(input),
			Detail:    detail,
			TokenHash: hash,
		}
		if err := db.Create(&feed).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("insert")) // # pragma: no cover — DB failure only; the 201 create path is covered
			return                                                       // # pragma: no cover — DB failure only
		}

		models.RecordAuditEvent(db, models.AuditEntityFeed, feed.ID, models.AuditOpCreate, userID)

		c.JSON(http.StatusCreated, models.FeedCreateResponse{
			Feed: feed,
			URL:  feedURL(cfg, plaintext),
		})
	}
}

// RotateFeed revokes an existing feed and mints a replacement with the same
// name, kind, entity_id and detail, in one transaction (the RotateApiToken
// shape). A missing or already-revoked feed is 404. ADR 0030 decision 6.
func RotateFeed(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		db := c.MustGet("db").(*gorm.DB)
		userID, ok := currentUserID(c)
		if !ok {
			return
		}

		id := c.Param("id")

		var oldFeed models.Feed
		if err := db.Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).First(&oldFeed).Error; err != nil {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Feed"))
			return
		}

		plaintext, hash, err := generateFeedToken()
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInternal("token generation failed")) // # pragma: no cover — token-mint failure only (crypto/rand)
			return                                                                        // # pragma: no cover — token-mint failure only (crypto/rand)
		}

		newFeed := models.Feed{
			UserID:    userID,
			Name:      oldFeed.Name,
			Kind:      oldFeed.Kind,
			EntityID:  oldFeed.EntityID,
			Detail:    oldFeed.Detail,
			TokenHash: hash,
		}
		now := clock.FromContext(c).Now()
		txErr := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&newFeed).Error; err != nil {
				return err // # pragma: no cover — DB failure only inside the rotate transaction
			}
			if err := tx.Model(&models.Feed{}).
				Where("id = ? AND user_id = ?", oldFeed.ID, userID).
				Update("revoked_at", now).Error; err != nil {
				return err // # pragma: no cover — DB failure only inside the rotate transaction
			}
			return nil
		})
		if txErr != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("rotate feed").WithError(txErr)) // # pragma: no cover — DB failure only; the 201 rotate path is covered
			return                                                                             // # pragma: no cover — DB failure only
		}

		models.RecordAuditEvent(db, models.AuditEntityFeed, oldFeed.ID, models.AuditOpRevoke, userID)
		models.RecordAuditEvent(db, models.AuditEntityFeed, newFeed.ID, models.AuditOpCreate, userID)

		c.JSON(http.StatusCreated, models.FeedCreateResponse{
			Feed: newFeed,
			URL:  feedURL(cfg, plaintext),
		})
	}
}

// DeleteFeed revokes a feed (sets revoked_at) and returns 204. A missing or
// already-revoked feed is 404. ADR 0030 decision 6.
func DeleteFeed(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	id := c.Param("id")

	var feed models.Feed
	if err := db.Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).First(&feed).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Feed"))
		return
	}

	if err := db.Model(&feed).Update("revoked_at", clock.FromContext(c).Now()).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update")) // # pragma: no cover — DB failure only; the 204 revoke path is covered
		return                                                       // # pragma: no cover — DB failure only
	}

	models.RecordAuditEvent(db, models.AuditEntityFeed, feed.ID, models.AuditOpRevoke, userID)

	c.Status(http.StatusNoContent)
}

// RevokeAllFeeds revokes every active feed for the caller. It mirrors
// RevokeAllApiTokens: the active ids are captured before the bulk update so
// the audit trail names each affected feed individually.
func RevokeAllFeeds(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var ids []string
	if err := db.Model(&models.Feed{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Pluck("id", &ids).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query")) // # pragma: no cover — DB failure only; the revoke-all count path is covered
		return                                                      // # pragma: no cover — DB failure only
	}

	revoked, err := services.RevokeAllFeeds(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update")) // # pragma: no cover — DB failure only
		return                                                       // # pragma: no cover — DB failure only
	}

	for _, id := range ids {
		models.RecordAuditEvent(db, models.AuditEntityFeed, id, models.AuditOpRevoke, userID)
	}

	c.JSON(http.StatusOK, gin.H{"revoked": revoked})
}

// feedEntityID returns the stored entity_id for an input: the contact
// VCardUID for kind=contact, "" otherwise.
func feedEntityID(input *models.FeedInput) string {
	if input.Kind == models.FeedKindContact {
		return input.EntityID
	}
	return ""
}

// resolveFeedEntity enforces the kind/entity_id rules and writes the
// appropriate error response, returning false when the request must abort:
//   - kind=contact requires entity_id to name a live contact the caller owns,
//     otherwise 404 (the same shape as every other missing owned resource, so
//     existence is not leaked);
//   - kind=aggregate requires an empty entity_id, otherwise 400.
func resolveFeedEntity(c *gin.Context, db *gorm.DB, userID uint, input *models.FeedInput) bool {
	if input.Kind == models.FeedKindAggregate {
		if input.EntityID != "" {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("entity_id", "must be empty for an aggregate feed"))
			return false
		}
		return true
	}

	var contact models.Contact
	if err := db.Where("vcard_uid = ? AND user_id = ?", input.EntityID, userID).First(&contact).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Contact"))
		} else {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to retrieve contact").WithError(err)) // # pragma: no cover — DB failure only; the 404 not-found path is covered
		}
		return false
	}
	return true
}

// ServeFeed is the unauthenticated Atom serving endpoint (issue #382, ADR
// 0030 decision 7). The feed token is the only credential and travels in the
// query string because most feed readers cannot set an Authorization header.
// Every miss — a missing token, a wrong prefix, an unknown hash, a revoked
// feed, a soft-deleted contact or a soft-deleted owner — is the same 404 with
// an empty body, so the response never distinguishes the cases.
func ServeFeed(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		db := c.MustGet("db").(*gorm.DB)

		// The feed token travels in the query string by design: most feed
		// readers cannot set an Authorization header (ADR 0030 decision 7).
		// The app's request log redacts query values, and the response sets
		// X-Robots-Tag + no-store. This is the deliberate capability-URL
		// exception to the no-auth-material-in-URLs rule.
		raw := c.Query("token") // nosemgrep: mycorrhizal-query-string-auth-material
		if !strings.HasPrefix(raw, "mycorrhizal_feed_") {
			feedMiss(c)
			return
		}
		hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))

		var feed models.Feed
		if err := db.Where("token_hash = ? AND revoked_at IS NULL", hash).First(&feed).Error; err != nil {
			feedMiss(c)
			return
		}

		// Default scope: a soft-deleted owner is a miss.
		var user models.User
		if err := db.First(&user, feed.UserID).Error; err != nil {
			feedMiss(c)
			return
		}

		var contact *models.Contact
		if feed.Kind == models.FeedKindContact {
			var ct models.Contact
			if err := db.Where("vcard_uid = ? AND user_id = ?", feed.EntityID, feed.UserID).First(&ct).Error; err != nil {
				feedMiss(c)
				return
			}
			contact = &ct
		}

		body, err := services.RenderAtomFeed(db, cfg, services.AtomRenderInput{
			Feed:    &feed,
			User:    &user,
			Contact: contact,
			Now:     clock.FromContext(c).Now(),
		})
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrDatabase("Failed to render feed").WithError(err)) // # pragma: no cover — DB failure only; the 200/304/miss paths are covered
			return                                                                                     // # pragma: no cover — DB failure only
		}

		etag := `"` + fmt.Sprintf("%x", sha256.Sum256(body)) + `"`
		c.Header("ETag", etag)
		c.Header("X-Robots-Tag", "noindex, nofollow, noarchive")

		// A poll that matches the current ETag is still an access.
		services.TouchFeed(db, &feed)

		if ifNoneMatchMatches(c.GetHeader("If-None-Match"), etag) {
			c.Status(http.StatusNotModified)
			return
		}

		c.Data(http.StatusOK, "application/atom+xml; charset=utf-8", body)
	}
}

// feedMiss writes the single indistinguishable 404 with an empty body.
func feedMiss(c *gin.Context) {
	c.Status(http.StatusNotFound)
	c.Abort()
}

// ifNoneMatchMatches reports whether an If-None-Match header value matches the
// current strong ETag. It accepts a comma-separated list and weak validators
// (W/"...") per RFC 9110, and "*".
func ifNoneMatchMatches(header, etag string) bool {
	header = strings.TrimSpace(header)
	if header == "" {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		candidate = strings.TrimPrefix(candidate, "W/")
		if candidate == etag {
			return true
		}
	}
	return false
}
