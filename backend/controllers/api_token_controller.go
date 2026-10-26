package controllers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"mycorrhizal/internal/clock"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"
	"net/http"
	"strconv"
	"time"

	apperrors "mycorrhizal/errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// generateApiToken mints a new plaintext token ("mycorrhizal_" + 32 random
// bytes, base64url) and its SHA-256 hash for storage. Shared by CreateApiToken
// and RotateApiToken so a reissued token is generated exactly the same way as
// a freshly created one.
func generateApiToken() (plaintext, hash string, err error) {
	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return "", "", err
	}
	plaintext = "mycorrhizal_" + base64.RawURLEncoding.EncodeToString(rawBytes)
	hash = fmt.Sprintf("%x", sha256.Sum256([]byte(plaintext)))
	return plaintext, hash, nil
}

func apiTokenResponse(token models.ApiToken) models.ApiTokenResponse {
	scopes := token.EffectiveScopes()
	return models.ApiTokenResponse{
		ID:         token.ID,
		Name:       token.Name,
		CreatedAt:  token.CreatedAt,
		LastUsedAt: token.LastUsedAt,
		RevokedAt:  token.RevokedAt,
		ExpiresAt:  token.ExpiresAt,
		Scope:      models.LegacyScopeForScopes(scopes),
		Scopes:     scopes,
	}
}

func resolveTokenScopes(input models.ApiTokenInput) ([]string, *apperrors.AppError) {
	if input.Scopes != nil {
		scopes := models.NormalizeTokenScopes(*input.Scopes)
		if len(scopes) == 0 {
			return nil, apperrors.ErrInvalidInput("scopes", "scopes must contain at least one scope")
		}
		for _, scope := range scopes {
			if !models.IsValidTokenScope(scope) {
				return nil, apperrors.ErrInvalidInput("scopes", "Unknown scope: "+scope)
			}
		}
		return scopes, nil
	}
	scope := models.DefaultApiTokenScope
	if input.Scope != "" {
		if input.Scope != models.LegacyTokenScopeFull && input.Scope != models.LegacyTokenScopeCardDAV {
			return nil, apperrors.ErrInvalidInput("scope", "Unknown scope: "+input.Scope)
		}
		scope = input.Scope
	}
	return models.ScopesForLegacyScope(scope), nil
}

func ListApiTokens(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	var tokens []models.ApiToken
	if err := db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&tokens).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		return
	}

	response := make([]models.ApiTokenResponse, len(tokens))
	for i, t := range tokens {
		response[i] = apiTokenResponse(t)
	}

	c.JSON(http.StatusOK, gin.H{"tokens": response})
}

func CreateApiToken(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	input, appErr := middleware.GetValidated[models.ApiTokenInput](c)
	if appErr != nil {
		apperrors.AbortWithError(c, appErr)
		return
	}

	plaintext, hash, err := generateApiToken()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("token generation failed"))
		return
	}

	scopes, scopeErr := resolveTokenScopes(*input)
	if scopeErr != nil {
		apperrors.AbortWithError(c, scopeErr)
		return
	}

	var expiresAt *time.Time
	if input.NeverExpires {
		if input.ExpiresInDays != nil {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("expires_in_days", "expires_in_days cannot be combined with never_expires"))
			return
		}
		if !models.IsDAVOnlyScopes(scopes) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("never_expires", "never_expires is only allowed for CardDAV/CalDAV app passwords"))
			return
		}
	} else {
		expiryDays := models.DefaultApiTokenExpiryDays
		if input.ExpiresInDays != nil {
			expiryDays = *input.ExpiresInDays
		}
		expiry := clock.FromContext(c).Now().Add(time.Duration(expiryDays) * 24 * time.Hour)
		expiresAt = &expiry
	}

	token := models.ApiToken{
		UserID:    userID,
		Name:      input.Name,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		Scope:     models.LegacyScopeForScopes(scopes),
		Scopes:    scopes,
	}
	if err := db.Create(&token).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("insert"))
		return
	}

	// T18 audit: API-token issuance (issue #381).
	models.RecordAuditEvent(db, models.AuditEntityAPIToken, fmt.Sprintf("%d", token.ID), models.AuditOpCreate, userID)

	c.JSON(http.StatusCreated, models.ApiTokenCreateResponse{
		ApiTokenResponse: apiTokenResponse(token),
		Token:            plaintext,
	})
}

func RevokeApiToken(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "must be a positive integer"))
		return
	}

	var token models.ApiToken
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&token).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("API token"))
		return
	}

	now := clock.FromContext(c).Now()
	if err := db.Model(&token).Update("revoked_at", now).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update"))
		return
	}

	// T18 audit: API-token revocation (issue #381).
	models.RecordAuditEvent(db, models.AuditEntityAPIToken, fmt.Sprintf("%d", token.ID), models.AuditOpRevoke, userID)

	c.JSON(http.StatusOK, gin.H{"message": "Token revoked successfully"})
}

// RevokeAllApiTokens is the self-service complement to an admin password
// reset (admin_user_controller.go's UpdateUser) and the recovery-path
// password reset (user_controller.go's ConfirmPasswordReset): a user who
// suspects a token leaked (e.g. lost device) can end every standing token at
// once without knowing which one leaked. Issue #413.
func RevokeAllApiTokens(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	// Capture which tokens are about to be revoked *before* revoking them, so
	// the audit trail can name each one individually afterward -- once
	// revoked_at is set there's no way to tell "revoked just now" apart from
	// "already revoked earlier" by querying alone.
	var ids []uint
	if err := db.Model(&models.ApiToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Pluck("id", &ids).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query"))
		return
	}

	revoked, err := services.RevokeAllAPITokens(db, userID)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update"))
		return
	}

	// One audit event per token actually revoked, matching RevokeApiToken's
	// per-token event so the audit trail names every affected token
	// individually rather than one opaque bulk event.
	for _, id := range ids {
		models.RecordAuditEvent(db, models.AuditEntityAPIToken, fmt.Sprintf("%d", id), models.AuditOpRevoke, userID)
	}

	c.JSON(http.StatusOK, gin.H{"revoked": revoked})
}

// RotateApiToken revokes an existing token and immediately reissues a new one
// with the same name and scope, for callers that want continuity (a script
// keeps running under a fresh credential) instead of the create-then-revoke
// two-step. The new plaintext is shown exactly once, like CreateApiToken's.
// Issue #413.
func RotateApiToken(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	userID, ok := currentUserID(c)
	if !ok {
		return
	}

	idParam := c.Param("id")
	id, err := strconv.ParseUint(idParam, 10, 64)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("id", "must be a positive integer"))
		return
	}

	var oldToken models.ApiToken
	if err := db.Where("id = ? AND user_id = ?", id, userID).First(&oldToken).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("API token"))
		return
	}
	if oldToken.RevokedAt != nil {
		apperrors.AbortWithError(c, apperrors.ErrConflict("API token is already revoked; nothing to rotate"))
		return
	}

	plaintext, hash, err := generateApiToken()
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("token generation failed"))
		return
	}

	// A rotated token gets a fresh default expiry window, not the old
	// token's remaining time -- reissuing resets the countdown. A DAV app
	// password that never expired stays that way.
	scopes := oldToken.EffectiveScopes()
	var expiresAt *time.Time
	if oldToken.ExpiresAt != nil || !models.IsDAVOnlyScopes(scopes) {
		expiry := clock.FromContext(c).Now().Add(time.Duration(models.DefaultApiTokenExpiryDays) * 24 * time.Hour)
		expiresAt = &expiry
	}

	newToken := models.ApiToken{
		UserID:    userID,
		Name:      oldToken.Name,
		TokenHash: hash,
		ExpiresAt: expiresAt,
		Scope:     models.LegacyScopeForScopes(scopes),
		Scopes:    scopes,
	}
	if err := db.Create(&newToken).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("insert"))
		return
	}

	now := clock.FromContext(c).Now()
	if err := db.Model(&oldToken).Update("revoked_at", now).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("update"))
		return
	}

	// T18 audit: same two events CreateApiToken/RevokeApiToken each fire
	// individually, both recorded here since rotation does both at once.
	models.RecordAuditEvent(db, models.AuditEntityAPIToken, fmt.Sprintf("%d", oldToken.ID), models.AuditOpRevoke, userID)
	models.RecordAuditEvent(db, models.AuditEntityAPIToken, fmt.Sprintf("%d", newToken.ID), models.AuditOpCreate, userID)

	c.JSON(http.StatusCreated, models.ApiTokenCreateResponse{
		ApiTokenResponse: apiTokenResponse(newToken),
		Token:            plaintext,
	})
}
