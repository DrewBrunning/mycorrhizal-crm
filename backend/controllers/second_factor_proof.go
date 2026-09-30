package controllers

import (
	"encoding/json"
	"fmt"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// secondFactorProofInput is the live-proof half of a request body: either a
// TOTP / recovery code, or an assertion from WebAuthnProofBegin.
type secondFactorProofInput struct {
	Code      string          `json:"code"`
	Assertion json.RawMessage `json:"assertion"`
}

// proofOptions tunes one call of requireSecondFactorProof.
type proofOptions struct {
	// target, when non-nil, is the passkey being removed: an assertion from it
	// is refused (proving the OTHER factors are still held).
	target *models.WebAuthnCredential
	// field names the request field in the "invalid code" validation error.
	field string
}

// secondFactorProofLockKey is the account-limiter bucket shared by EVERY
// session-authenticated second-factor proof (issue #1352): enrollment,
// disable, recovery-code regeneration, passkey removal and self-deletion.
// One bucket per account (not per endpoint) so an attacker holding a stolen
// session cannot multiply their guesses by spreading them across endpoints.
// It is keyed on the user id and is separate from the login buckets
// (middleware.LoginKey), so guessing here locks further proofs, never the
// owner's sign-in.
func secondFactorProofLockKey(userID uint) string { return fmt.Sprintf("2fa-proof:%d", userID) }

// requireSecondFactorProof is the ONE gate for a live second-factor proof
// behind an authenticated session (ASVS 2.2.1 / 3.7.1). It rejects while the
// account bucket is locked (429, the login-lock shape, even for a correct
// code), otherwise validates the proof (an assertion when waUser is non-nil
// and one was sent, else a TOTP/recovery code), counts a miss toward
// MaxLoginAttempts and resets the bucket on success. Returns false after
// aborting the request. Complete2FALogin is the only other proof site; it is
// pre-session and uses the login buckets. TestProofSitesGoThroughHelper fails
// if any other caller appears.
func requireSecondFactorProof(c *gin.Context, db *gorm.DB, cfg *config.Config, user *models.User, waUser *services.WebAuthnUser, in secondFactorProofInput, opts proofOptions) bool {
	limiter := middleware.GetAccountRateLimiter()
	key := secondFactorProofLockKey(user.ID)
	if locked, secs := limiter.IsLocked(key); locked {
		abortLocked(c, secs, "Too many failed verification attempts. Please try again later.")
		return false
	}
	var proved, sameCredential bool
	if len(in.Assertion) > 0 && waUser != nil {
		proved, sameCredential = verifyProofAssertion(c, cfg, waUser, opts.target, in.Assertion)
	} else {
		proved = valid2FAProof(db, user, in.Code, cfg.JWTSecretKey)
	}
	if sameCredential {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("assertion", "That is the passkey being removed. Verify with a different passkey."))
		return false
	}
	if !proved {
		if locked, secs := limiter.RecordFailedAttempt(key); locked {
			abortLocked(c, secs, "Too many failed verification attempts. Please try again later.")
			return false
		}
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput(opts.field, "Invalid code. Please try again."))
		return false
	}
	limiter.RecordSuccessfulLogin(key)
	return true
}
