package controllers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"mycorrhizal/config"
	apperrors "mycorrhizal/errors"
	"mycorrhizal/internal/clock"
	"mycorrhizal/logger"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// Issue #593 — WebAuthn / passkeys (backend half of #560).
//
// A passkey is an ALTERNATIVE second factor to TOTP: login demands either, and
// a TOTP-enrolled account may also enroll a passkey and use either. Enrollment
// and management ride an authenticated session (same trust level as
// SetupTwoFactor/ConfirmTwoFactor); the login ceremony is gated by the same
// 2fa_pending challenge cookie POST /login mints for TOTP. OIDC-provisioned
// accounts are excluded, exactly as for TOTP (oidcUserErr) — extending that is
// a deliberate later decision.
// ---------------------------------------------------------------------------

// webAuthnRelyingParty builds the relying party or aborts with a clear config
// error (FRONTEND_URL="*" has no WebAuthn meaning).
func webAuthnRelyingParty(c *gin.Context, cfg *config.Config) (*webauthn.WebAuthn, bool) {
	wa, err := services.NewWebAuthn(cfg)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrConflict(err.Error()))
		return nil, false
	}
	return wa, true
}

// loadWebAuthnCaller loads the authenticated caller and their credentials.
func loadWebAuthnCaller(c *gin.Context, db *gorm.DB) (*services.WebAuthnUser, []models.WebAuthnCredential, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return nil, nil, false // # pragma: no cover — currentUserID already aborted; AuthMiddleware guarantees an id
	}
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query user").WithError(err)) // # pragma: no cover — the user row vanishing between AuthMiddleware and this handler; only a failing store trips this
		return nil, nil, false                                                          // # pragma: no cover — same as above
	}
	waUser, rows, err := services.LoadWebAuthnUser(db, user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query passkeys").WithError(err))
		return nil, nil, false
	}
	return waUser, rows, true
}

// errCloneWarning marks an assertion whose signature counter did not advance.
var errCloneWarning = errors.New("authenticator signature counter did not increase (possible clone)")

func isOIDCUser(u models.User) bool { return u.OIDCSubject != nil && *u.OIDCSubject != "" }

// requireEnrollmentProof gates enrolling an ADDITIONAL second factor (issue
// #1337). When the account holds no confirmed factor (no TOTP, no passkey) this
// is the first factor and needs no proof. Otherwise the caller must present a
// live proof (a TOTP/recovery code or an assertion from an existing passkey),
// the same bar as removal (DeleteWebAuthnCredential / DisableTwoFactor), so a
// stolen session alone cannot add its own authenticator and then use it to
// remove the owner's. Proof checking and the attempt limiter live in
// requireSecondFactorProof. Returns false after aborting the request.
func requireEnrollmentProof(c *gin.Context, db *gorm.DB, cfg *config.Config, waUser *services.WebAuthnUser, in secondFactorProofInput) bool {
	user := waUser.User
	confirmedTOTP := user.TOTPEnabled && user.TOTPSecretEncrypted != nil && *user.TOTPSecretEncrypted != ""
	if !confirmedTOTP && len(waUser.Credentials) == 0 {
		return true
	}
	if in.Code == "" && len(in.Assertion) == 0 {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("code"))
		return false
	}
	return requireSecondFactorProof(c, db, cfg, &user, waUser, in, proofOptions{field: "code"})
}

// WebAuthnRegisterBegin starts the enrollment ceremony. The optional `name`
// (a user label like "YubiKey") is remembered and stored on finish. Once the
// account already holds a second factor it also needs a live proof (`code` or
// `assertion`, issue #1337); finish is only reachable through the ceremony
// this step stores, so gating begin gates the whole enrollment.
func WebAuthnRegisterBegin(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)
	wa, ok := webAuthnRelyingParty(c, &cfg)
	if !ok {
		return
	}
	waUser, rows, ok := loadWebAuthnCaller(c, db)
	if !ok {
		return
	}
	if isOIDCUser(waUser.User) {
		apperrors.AbortWithError(c, apperrors.ErrForbidden(oidcUserErr))
		return
	}

	var input struct {
		secondFactorProofInput
		Name string `json:"name"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
			apperrors.AbortWithError(c, apperrors.ErrInvalidInput("body", "Invalid JSON"))
			return
		}
	}
	name := strings.TrimSpace(input.Name)
	if len(name) > 100 {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("name", "Name must be 100 characters or fewer"))
		return
	}
	if name == "" {
		name = services.RandomCredentialLabel()
	}
	if !requireEnrollmentProof(c, db, &cfg, waUser, input.secondFactorProofInput) {
		return
	}

	exclusions := make([]protocol.CredentialDescriptor, 0, len(rows))
	for _, cred := range waUser.Credentials {
		exclusions = append(exclusions, cred.Descriptor())
	}
	creation, session, err := wa.BeginRegistration(waUser,
		webauthn.WithExclusions(exclusions),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementPreferred),
	)
	if err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", waUser.User.ID).Msg("Failed to begin WebAuthn registration") // # pragma: no cover — go-webauthn only errors here on an invalid Config, which NewWebAuthn already rejected
		apperrors.AbortWithError(c, apperrors.ErrInternal("Could not start passkey registration").WithError(err))           // # pragma: no cover — same as above
		return
	}
	services.DefaultCeremonies.Put(services.CeremonyRegister, waUser.User.ID, *session, name)
	c.JSON(http.StatusOK, creation)
}

// WebAuthnRegisterFinish completes enrollment: verifies the attestation,
// stores the credential, and — if this is the account's first second factor of
// any kind — mints recovery codes and ends other sessions, mirroring TOTP's
// ConfirmTwoFactor.
func WebAuthnRegisterFinish(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)
	wa, ok := webAuthnRelyingParty(c, &cfg)
	if !ok {
		return
	}
	waUser, _, ok := loadWebAuthnCaller(c, db)
	if !ok {
		return
	}
	user := waUser.User
	if isOIDCUser(user) {
		apperrors.AbortWithError(c, apperrors.ErrForbidden(oidcUserErr))
		return
	}

	session, name, found := services.DefaultCeremonies.Take(services.CeremonyRegister, user.ID)
	if !found {
		apperrors.AbortWithError(c, apperrors.ErrConflict("No passkey registration in progress. Start again."))
		return
	}
	credential, err := wa.FinishRegistration(waUser, session, c.Request)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("credential", "Passkey registration could not be verified"))
		return
	}

	firstFactor := !user.TOTPEnabled && len(waUser.Credentials) == 0
	var recoveryCount int64
	if err := db.Model(&models.RecoveryCode{}).Where("user_id = ?", user.ID).Count(&recoveryCount).Error; err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query recovery codes").WithError(err))
		return
	}
	recoveryCodes := []string{}
	if recoveryCount == 0 {
		recoveryCodes, err = services.GenerateRecoveryCodes(recoveryCodeCount)
		if err != nil {
			apperrors.AbortWithError(c, apperrors.ErrInternal("Could not generate recovery codes").WithError(err)) // # pragma: no cover — crypto/rand failure only
			return
		}
	}

	row := services.RowFromCredential(user.ID, name, credential)
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if err := services.StoreRecoveryCodes(tx, user.ID, recoveryCodes); err != nil {
			return err // # pragma: no cover — transaction-body failures are DB/disk faults only (schema/FK integrity is proven elsewhere)
		}
		if firstFactor {
			return tx.Model(&user).Update("token_version", gorm.Expr("token_version + 1")).Error
		}
		return nil
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			apperrors.AbortWithError(c, apperrors.ErrAlreadyExists("Passkey"))
			return
		}
		logger.FromContext(c).Error().Err(err).Uint("user_id", user.ID).Msg("Failed to store passkey") // # pragma: no cover — DB/disk failure only; the UNIQUE conflict branch above is the reachable one
		apperrors.AbortWithError(c, apperrors.ErrDatabase("store passkey").WithError(err))             // # pragma: no cover — same as above
		return
	}

	models.RecordAuditEvent(models.AuditEntityUser, fmt.Sprintf("%d", user.ID), models.AuditOpWebAuthnRegister, user.ID)
	if firstFactor {
		endOtherSessions(c, db, user.ID)
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":             row.ID,
		"name":           row.Name,
		"created_at":     row.CreatedAt,
		"recovery_codes": recoveryCodes,
	})
}

// endOtherSessions is the posture-change tail shared with ConfirmTwoFactor: a
// first second factor invalidates every other session and device grant (issue
// #722/#866) and re-mints the caller's own session from a reloaded row.
func endOtherSessions(c *gin.Context, db *gorm.DB, userID uint) {
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", userID).Msg("Failed to reload user after 2FA posture change") // # pragma: no cover — best-effort post-success tail; only a failing store trips this
		return
	}
	if _, err := services.RevokeAllDeviceGrants(db, userID); err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", userID).Msg("Failed to revoke device grants") // # pragma: no cover — best-effort post-success revocation; only a failing store trips this
	}
	if _, err := services.RevokeAllSessions(db, userID); err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", userID).Msg("Failed to revoke sessions") // # pragma: no cover — best-effort post-success revocation; only a failing store trips this
	}
	reissueSessionToken(c, user)
}

// WebAuthnLoginBegin starts the login assertion ceremony for the account named
// by the 2fa_pending challenge cookie.
func WebAuthnLoginBegin(c *gin.Context, cfg *config.Config) {
	wa, ok := webAuthnRelyingParty(c, cfg)
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	user, _, ok := pendingChallengeUser(c, db, cfg)
	if !ok {
		return
	}
	waUser, _, err := services.LoadWebAuthnUser(db, user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query passkeys").WithError(err))
		return
	}
	if len(waUser.Credentials) == 0 {
		apperrors.AbortWithError(c, apperrors.ErrConflict("No passkey is registered for this account"))
		return
	}
	assertion, session, err := wa.BeginLogin(waUser)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Could not start passkey login").WithError(err)) // # pragma: no cover — go-webauthn only errors here on an invalid Config, which NewWebAuthn already rejected
		return
	}
	services.DefaultCeremonies.Put(services.CeremonyLogin, user.ID, *session, "")
	c.JSON(http.StatusOK, assertion)
}

// WebAuthnLoginFinish verifies the assertion and, on success, mints the same
// session Complete2FALogin does.
func WebAuthnLoginFinish(c *gin.Context, cfg *config.Config) {
	wa, ok := webAuthnRelyingParty(c, cfg)
	if !ok {
		return
	}
	db := c.MustGet("db").(*gorm.DB)
	user, username, ok := pendingChallengeUser(c, db, cfg)
	if !ok {
		return
	}

	clientIP := c.ClientIP()
	accountLimiter := middleware.GetAccountRateLimiter()
	if isLocked, remainingSecs := accountLimiter.IsLoginLocked(username, clientIP); isLocked {
		abortLoginLocked(c, remainingSecs)
		return
	}

	waUser, _, err := services.LoadWebAuthnUser(db, user)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query passkeys").WithError(err))
		return
	}
	if len(waUser.Credentials) == 0 {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("No passkey is registered for this account. Please sign in again."))
		return
	}
	session, _, found := services.DefaultCeremonies.Take(services.CeremonyLogin, user.ID)
	if !found {
		apperrors.AbortWithError(c, apperrors.ErrConflict("No passkey login in progress. Start again."))
		return
	}

	credential, err := wa.FinishLogin(waUser, session, c.Request)
	// The library reports a non-increasing signature counter (a cloned
	// authenticator) as CloneWarning on an otherwise-valid assertion rather
	// than an error; a login must refuse it.
	if err == nil && credential.Authenticator.CloneWarning {
		err = errCloneWarning
	}
	if err != nil {
		models.RecordAuditEvent(models.AuditEntityAuth, user.Username, models.AuditOpLoginFailed, user.ID)
		if _, lockoutSecs := accountLimiter.RecordLoginFailure(username, clientIP); lockoutSecs > 0 {
			abortLoginLocked(c, lockoutSecs)
			return
		}
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Passkey could not be verified"))
		return
	}
	if err := services.RecordWebAuthnUse(db, user.ID, credential); err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", user.ID).Msg("Failed to record passkey use") // # pragma: no cover — only a failing store trips this; the assertion itself already verified
	}
	accountLimiter.RecordLoginSuccess(username, clientIP)
	issueLoginSession(c, cfg, db, user)
}

// pendingChallengeUser resolves the 2fa_pending cookie to its user. Aborts
// with 401 on a missing/invalid/expired challenge or a vanished account.
func pendingChallengeUser(c *gin.Context, db *gorm.DB, cfg *config.Config) (models.User, string, bool) {
	pending, err := c.Cookie("2fa_pending")
	if err != nil || pending == "" {
		// A precondition failure (no /login step yet), not an authentication
		// failure: public routes must not answer 401 to an anonymous caller.
		apperrors.AbortWithError(c, apperrors.ErrValidation("No pending two-factor login found. Please sign in again."))
		return models.User{}, "", false
	}
	userID, username, ok := services.Parse2FAChallengeToken(pending, cfg)
	if !ok {
		apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Invalid or expired two-factor session. Please sign in again."))
		return models.User{}, "", false
	}
	var user models.User
	if err := db.First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apperrors.AbortWithError(c, apperrors.ErrUnauthorized("Invalid or expired two-factor session. Please sign in again."))
			return models.User{}, "", false
		}
		apperrors.AbortWithError(c, apperrors.ErrDatabase("query user").WithError(err))
		return models.User{}, "", false
	}
	return user, username, true
}

func abortLoginLocked(c *gin.Context, secs int) {
	abortLocked(c, secs, "Too many failed login attempts. Please try again later.")
}

// abortLocked is the shared 429 lockout body; only the human message varies.
func abortLocked(c *gin.Context, secs int, message string) {
	c.JSON(http.StatusTooManyRequests, gin.H{
		"error":          "Account temporarily locked",
		"message":        message,
		"retry_after":    secs,
		"retry_after_at": clock.FromContext(c).Now().Add(time.Duration(secs) * time.Second).Format(time.RFC3339),
	})
	c.Abort()
}

// ListWebAuthnCredentials returns the caller's own passkeys — never the public
// key or raw credential id.
func ListWebAuthnCredentials(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	_, rows, ok := loadWebAuthnCaller(c, db)
	if !ok {
		return
	}
	out := make([]models.WebAuthnCredential, 0, len(rows))
	out = append(out, rows...)
	c.JSON(http.StatusOK, gin.H{"credentials": out})
}

// WebAuthnProofBegin starts a re-authentication assertion the caller can
// present as proof when deleting a passkey (see DeleteWebAuthnCredential).
func WebAuthnProofBegin(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)
	wa, ok := webAuthnRelyingParty(c, &cfg)
	if !ok {
		return
	}
	waUser, rows, ok := loadWebAuthnCaller(c, db)
	if !ok {
		return
	}
	if len(waUser.Credentials) == 0 {
		apperrors.AbortWithError(c, apperrors.ErrConflict("No passkey is registered for this account"))
		return
	}
	// Optional body: {"exclude_id": "<passkey id>"} names the passkey being
	// removed, which must not be offered as the proof (it would be rejected
	// after the user already spent the ceremony on it — issue #1317).
	var input struct {
		ExcludeID string `json:"exclude_id"`
	}
	if err := c.ShouldBindJSON(&input); err != nil && !errors.Is(err, io.EOF) {
		apperrors.AbortWithError(c, apperrors.ErrInvalidInput("exclude_id", "Invalid request body"))
		return
	}
	var opts []webauthn.LoginOption
	if input.ExcludeID != "" {
		// rows is already scoped to the caller, so another user's id is
		// indistinguishable from an unknown one.
		var excluded *models.WebAuthnCredential
		for i := range rows {
			if rows[i].ID == input.ExcludeID {
				excluded = &rows[i]
			}
		}
		if excluded == nil {
			apperrors.AbortWithError(c, apperrors.ErrNotFound("Passkey"))
			return
		}
		allowed := make([]protocol.CredentialDescriptor, 0, len(rows))
		for _, r := range rows {
			if r.ID != excluded.ID {
				allowed = append(allowed, protocol.CredentialDescriptor{Type: protocol.PublicKeyCredentialType, CredentialID: r.CredentialID})
			}
		}
		if len(allowed) == 0 {
			apperrors.AbortWithError(c, apperrors.ErrConflict("No other passkey is registered to verify with"))
			return
		}
		opts = append(opts, webauthn.WithAllowedCredentials(allowed))
	}
	assertion, session, err := wa.BeginLogin(waUser, opts...)
	if err != nil {
		apperrors.AbortWithError(c, apperrors.ErrInternal("Could not start passkey verification").WithError(err)) // # pragma: no cover — go-webauthn only errors here on an invalid Config, which NewWebAuthn already rejected
		return
	}
	services.DefaultCeremonies.Put(services.CeremonyProof, waUser.User.ID, *session, "")
	c.JSON(http.StatusOK, assertion)
}

// DeleteWebAuthnCredential removes one of the caller's passkeys. Gated on a
// live second-factor proof (ASVS 3.7.1, same bar as DisableTwoFactor): either
// {"code": …} (a live TOTP or recovery code) or {"assertion": …} — an assertion
// from WebAuthnProofBegin made with a DIFFERENT remaining passkey. Hard-deletes
// the row and records an audit event. Removing the account's last factor
// also drops its recovery codes and ends other sessions.
func DeleteWebAuthnCredential(c *gin.Context) {
	db := c.MustGet("db").(*gorm.DB)
	cfg := currentConfig(c)
	waUser, rows, ok := loadWebAuthnCaller(c, db)
	if !ok {
		return
	}
	user := waUser.User

	var target *models.WebAuthnCredential
	for i := range rows {
		if rows[i].ID == c.Param("id") {
			target = &rows[i]
		}
	}
	if target == nil {
		apperrors.AbortWithError(c, apperrors.ErrNotFound("Passkey"))
		return
	}

	var input secondFactorProofInput
	if err := c.ShouldBindJSON(&input); err != nil || (input.Code == "" && len(input.Assertion) == 0) {
		apperrors.AbortWithError(c, apperrors.ErrMissingField("code"))
		return
	}
	if !requireSecondFactorProof(c, db, &cfg, &user, waUser, input, proofOptions{target: target, field: "code"}) {
		return
	}

	lastFactor := false
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", target.ID, user.ID).Delete(&models.WebAuthnCredential{}).Error; err != nil {
			return err // # pragma: no cover — transaction-body failures are DB/disk faults only (schema/FK integrity is proven elsewhere)
		}
		var remaining int64
		if err := tx.Model(&models.WebAuthnCredential{}).Where("user_id = ?", user.ID).Count(&remaining).Error; err != nil {
			return err // # pragma: no cover — transaction-body failures are DB/disk faults only (schema/FK integrity is proven elsewhere)
		}
		if remaining == 0 && !user.TOTPEnabled {
			lastFactor = true
			if err := tx.Where("user_id = ?", user.ID).Delete(&models.RecoveryCode{}).Error; err != nil {
				return err // # pragma: no cover — transaction-body failures are DB/disk faults only (schema/FK integrity is proven elsewhere)
			}
			return tx.Model(&user).Update("token_version", gorm.Expr("token_version + 1")).Error
		}
		return nil
	})
	if err != nil {
		logger.FromContext(c).Error().Err(err).Uint("user_id", user.ID).Msg("Failed to delete passkey") // # pragma: no cover — DB/disk failure only
		apperrors.AbortWithError(c, apperrors.ErrDatabase("delete passkey").WithError(err))             // # pragma: no cover — same as above
		return
	}

	models.RecordAuditEvent(models.AuditEntityUser, fmt.Sprintf("%d", user.ID), models.AuditOpWebAuthnRevoke, user.ID)
	if lastFactor {
		endOtherSessions(c, db, user.ID)
	}
	c.JSON(http.StatusOK, gin.H{"message": "Passkey removed"})
}

// verifyProofAssertion validates an assertion produced for the CeremonyProof
// begun by WebAuthnProofBegin. When target is non-nil it must come from a
// passkey other than the one being deleted (proving another factor is still
// held); a nil target (enrollment, issue #1337) accepts any of the caller's
// passkeys.
func verifyProofAssertion(c *gin.Context, cfg *config.Config, waUser *services.WebAuthnUser, target *models.WebAuthnCredential, raw []byte) (proved, sameCredential bool) {
	wa, err := services.NewWebAuthn(cfg)
	if err != nil {
		return false, false
	}
	session, _, found := services.DefaultCeremonies.Take(services.CeremonyProof, waUser.User.ID)
	if !found {
		return false, false
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(raw))
	if err != nil {
		return false, false
	}
	cred, err := wa.ValidateLogin(waUser, session, parsed)
	if err != nil || cred.Authenticator.CloneWarning {
		return false, false
	}
	if target != nil && bytes.Equal(cred.ID, target.CredentialID) {
		return false, true
	}
	db := c.MustGet("db").(*gorm.DB)
	if err := services.RecordWebAuthnUse(db, waUser.User.ID, cred); err != nil {
		logger.FromContext(c).Error().Err(err).Msg("Failed to record passkey use") // # pragma: no cover — only a failing store trips this
	}
	return true, false
}
