package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"mycorrhizal/config"
	"mycorrhizal/i18n"
	"mycorrhizal/logger"
	"time"
)

const (
	passwordResetTokenBytes = 32
	passwordResetTTL        = time.Hour
)

// GeneratePasswordResetToken creates a secure token and its hashed representation.
func GeneratePasswordResetToken() (string, string, error) {
	raw := make([]byte, passwordResetTokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := HashPasswordResetToken(token)
	return token, hash, nil
}

// HashPasswordResetToken hashes a reset token for database storage.
func HashPasswordResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// PasswordResetExpiry returns when a reset token should expire.
func PasswordResetExpiry() time.Time {
	return time.Now().Add(passwordResetTTL)
}

// SendPasswordResetEmail dispatches a reset email when Resend is configured.
// The lang parameter specifies the user's preferred language for the email content.
func SendPasswordResetEmail(email, token, lang string, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	if !cfg.EmailEnabled() {
		logger.Warn().Str("email", logger.MaskEmail(email)).Msg("No email channel configured; password reset email not sent")
		return nil
	}

	// Default to English if language not set
	if lang == "" {
		lang = i18n.DefaultLanguage
	}

	htmlBody, err := renderPasswordResetEmail(PasswordResetEmailData{
		Intro:       i18n.T(lang, "email.passwordReset.intro"),
		Instruction: i18n.T(lang, "email.passwordReset.instruction"),
		Token:       token,
		Ignore:      i18n.T(lang, "email.passwordReset.ignore"),
		Footer:      i18n.T(lang, "email.footer"),
	})
	if err != nil {
		return fmt.Errorf("failed to render password reset email: %w", err)
	}

	if err := SendEmail(*cfg, EmailMessage{
		To:      email,
		Subject: i18n.T(lang, "email.passwordReset.subject"),
		HTML:    htmlBody,
	}); err != nil {
		return err
	}

	logger.Info().Str("email", logger.MaskEmail(email)).Str("language", lang).Msg("Password reset email sent")
	return nil
}

// SendPasswordChangedEmail notifies the account owner that their password
// was just changed (issue #411 / ASVS 2.2.3), so they notice if a reset
// wasn't initiated by them. Sent after ConfirmPasswordReset succeeds; the
// caller treats a failure here as non-fatal -- the password change itself
// has already happened and must not be undone by a notification error.
func SendPasswordChangedEmail(email, lang string, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is required")
	}

	if !cfg.EmailEnabled() {
		logger.Warn().Str("email", logger.MaskEmail(email)).Msg("No email channel configured; password-changed notification not sent")
		return nil
	}

	if lang == "" {
		lang = i18n.DefaultLanguage
	}

	htmlBody, err := renderPasswordChangedEmail(PasswordChangedEmailData{
		Intro:   i18n.T(lang, "email.passwordChanged.intro"),
		Warning: i18n.T(lang, "email.passwordChanged.warning"),
		Footer:  i18n.T(lang, "email.footer"),
	})
	if err != nil {
		return fmt.Errorf("failed to render password-changed email: %w", err)
	}

	if err := SendEmail(*cfg, EmailMessage{
		To:      email,
		Subject: i18n.T(lang, "email.passwordChanged.subject"),
		HTML:    htmlBody,
	}); err != nil {
		return err
	}

	logger.Info().Str("email", logger.MaskEmail(email)).Str("language", lang).Msg("Password-changed notification email sent")
	return nil
}
