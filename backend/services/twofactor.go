package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// TOTP (N8 — issue #158). RFC 6238 shared-secret one-time passwords.
//
// The shared secret is encrypted at rest (EncryptCredential) and never
// persisted in plaintext; the plaintext only ever appears in the /setup
// response, once, so the client can render the QR/manual entry.
//
// Validation uses the RFC 6238 defaults every authenticator app assumes
// (HMAC-SHA1, 6 digits, 30-second period) with a skew of exactly one step on
// either side — the ±1 window the ticket specifies, no more.
// ---------------------------------------------------------------------------

// TOTPIssuer is the issuer label in the generated otpauth:// URL. Shows up in
// the authenticator app next to the account name.
const TOTPIssuer = "mycorrhizal"

// RecoveryCodeAlphabet avoids visually confusable characters (0/O, 1/I/L),
// so the codes survive being typed by hand.
const recoveryCodeAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

// recoveryCodeGroupLength and recoveryCodeGroups describe the human-readable
// XXXX-XXXX-XXXX shape recovery codes are displayed in.
const (
	recoveryCodeGroupLength = 5
	recoveryCodeGroups      = 3
)

// twoFactorChallengeTTL is how long a step-2 login challenge stays valid. Long
// enough to complete a login, short enough that a leaked token is useless once
// the session is forgotten.
const twoFactorChallengeTTL = 10 * time.Minute

// GenerateTOTPSecret mints a fresh RFC 6238 secret and its otpauth:// URL.
// Returns the base32 secret (what the authenticator app stores and what
// ValidateTOTP compares against) and the URL (for the QR code).
func GenerateTOTPSecret(accountName string) (secret string, otpauthURL string, err error) {
	if accountName == "" {
		return "", "", errors.New("account name cannot be empty")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TOTPIssuer,
		AccountName: accountName,
		SecretSize:  20,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP reports whether code is a valid RFC 6238 code for secret,
// accepting a ±1 step window (current step plus the one before and after it).
func ValidateTOTP(secret, code string) bool {
	if secret == "" || code == "" {
		return false
	}
	valid, err := totp.ValidateCustom(code, secret, time.Now().UTC(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && valid
}

// GenerateRecoveryCodes mints n single-use login fallback codes in the
// human-readable XXXXX-XXXXX-XXXXX shape. The plaintext is returned to the
// caller to show exactly once; only hashes (HashRecoveryCode) are stored.
func GenerateRecoveryCodes(n int) ([]string, error) {
	if n <= 0 {
		return nil, errors.New("recovery code count must be positive")
	}
	codes := make([]string, 0, n)
	for i := 0; i < n; i++ {
		code, err := randomRecoveryCode()
		if err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func randomRecoveryCode() (string, error) {
	// Each code is recoveryCodeGroups*recoveryCodeGroupLength = 15 chars from a
	// 32-symbol alphabet. 15 bytes gives exactly 15 chars (one symbol each).
	buf := make([]byte, recoveryCodeGroups*recoveryCodeGroupLength)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	alphabet := []byte(recoveryCodeAlphabet)
	var sb strings.Builder
	for _, b := range buf {
		sb.WriteByte(alphabet[int(b)%len(alphabet)])
	}
	raw := sb.String()
	var groups []string
	for i := 0; i < recoveryCodeGroups; i++ {
		start := i * recoveryCodeGroupLength
		groups = append(groups, raw[start:start+recoveryCodeGroupLength])
	}
	return strings.Join(groups, "-"), nil
}

// HashRecoveryCode hashes a recovery code for storage. SHA-256, matching the
// password-reset tokens and API tokens — the code carries ~75 bits of
// randomness so a fast hash is acceptable and consistent with the codebase.
// The code is normalized first (see normalizeRecoveryCode) so the hash is
// stable regardless of how the user retypes it.
func HashRecoveryCode(code string) string {
	sum := sha256.Sum256([]byte(normalizeRecoveryCode(code)))
	return hex.EncodeToString(sum[:])
}

// normalizeRecoveryCode strips formatting so a recovery code matches however
// it is retyped: uppercase (authenticator apps and keyboards differ), and with
// or without the display dashes ("AAAAA-BBBBB-CCCCC" == "aaaaabbbbbccccc").
// The plaintext shown to the user always uses the dashed, uppercase form, but
// the stored hash is of this canonical form so login is forgiving.
func normalizeRecoveryCode(code string) string {
	upper := strings.ToUpper(code)
	var sb strings.Builder
	for _, r := range upper {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// StoreRecoveryCodes persists the hashed forms of the given plaintext codes.
// It does not delete existing codes — callers that need replacement semantics
// (regeneration) must delete first.
func StoreRecoveryCodes(db *gorm.DB, userID uint, codes []string) error {
	rows := make([]models.RecoveryCode, 0, len(codes))
	for _, code := range codes {
		rows = append(rows, models.RecoveryCode{
			UserID:   userID,
			CodeHash: HashRecoveryCode(code),
		})
	}
	if len(rows) == 0 {
		return nil
	}
	return db.Create(&rows).Error
}

// ConsumeRecoveryCode verifies code against the user's stored hashes and, on
// a match, deletes it so it can never be used again. Single-use by deletion:
// the lookup and the delete are one WHERE clause, so there is no race window
// where two concurrent requests both see the code as valid.
func ConsumeRecoveryCode(db *gorm.DB, userID uint, code string) bool {
	hash := HashRecoveryCode(code)
	res := db.Where("user_id = ? AND code_hash = ?", userID, hash).Delete(&models.RecoveryCode{})
	return res.Error == nil && res.RowsAffected == 1
}

// ---------------------------------------------------------------------------
// Step-2 login challenge (N8).
//
// After the password is verified, a user with 2FA enabled receives a short-
// lived JWT (purpose: "2fa") instead of a session. It proves "the password was
// correct, moments ago" without minting any usable session — AuthMiddleware
// rejects purpose=2fa tokens outright — and is exchanged for the real session
// only after a valid TOTP/recovery code.
// ---------------------------------------------------------------------------

// twoFactorChallengePurpose is the JWT claim that marks a pending-2FA login
// challenge. AuthMiddleware refuses any token carrying it, so a stolen or
// leaked challenge can never double as a session.
const twoFactorChallengePurpose = "2fa"

// Generate2FAChallengeToken builds the step-2 login challenge for a user whose
// password has just been verified. It deliberately carries NO token_version
// claim (AuthMiddleware requires one for sessions) and expires in minutes.
func Generate2FAChallengeToken(user models.User, cfg *config.Config) (string, error) {
	if cfg.JWTSecretKey == "" {
		return "", errors.New("JWT secret key is empty")
	}
	claims := jwt.MapClaims{
		"authorized": true,
		"username":   user.Username,
		"user_id":    user.ID,
		"purpose":    twoFactorChallengePurpose,
		"exp":        time.Now().Add(twoFactorChallengeTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWTSecretKey))
}

// Parse2FAChallengeToken validates a step-2 challenge JWT and returns the
// user it was minted for. Returns ok=false for anything that is not a valid,
// unexpired 2fa-purpose token.
func Parse2FAChallengeToken(raw string, cfg *config.Config) (userID uint, username string, ok bool) {
	if raw == "" || cfg == nil {
		return 0, "", false
	}
	token, err := jwt.Parse(raw, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(cfg.JWTSecretKey), nil
	})
	if err != nil || !token.Valid {
		return 0, "", false
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return 0, "", false
	}
	if purpose, _ := claims["purpose"].(string); purpose != twoFactorChallengePurpose {
		return 0, "", false
	}
	userIDFloat, ok := claims["user_id"].(float64)
	if !ok || userIDFloat <= 0 {
		return 0, "", false
	}
	username, _ = claims["username"].(string)
	return uint(userIDFloat), username, true
}
