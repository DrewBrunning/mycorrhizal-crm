package services

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/oauth2"
)

// ---------------------------------------------------------------------------
// Android OIDC native return (issue #965).
//
// The custom-scheme deep link an Android app receives is first-come-first-
// served: any app on the device may register `mycorrhizal://` and receive the
// intent. Delivering the session JWT in that URI therefore hands it to whatever
// app wins the race. Instead the backend hands back a short-lived, single-
// purpose exchange code bound to a PKCE challenge the app generated and never
// transmits until it redeems the code over a direct HTTPS request. An
// interceptor gets only a code that is useless without the verifier.
//
// Like the 2FA step-2 challenge, the code is a JWT carrying a `purpose` claim
// that AuthMiddleware refuses, so it can never double as a session bearer.
// Redemption is bound to the app's PKCE verifier (RFC 7636), so an interceptor
// of the interceptable custom-scheme redirect cannot use the code.
// ---------------------------------------------------------------------------

const (
	// OIDCNativeExchangePurpose is the JWT claim marking the Android OIDC
	// native-return exchange code. AuthMiddleware rejects any token carrying a
	// purpose claim, so this code is redeemable only at the exchange endpoint.
	OIDCNativeExchangePurpose = "oidc_native_exchange"

	// oidcNativeExchangeTTL bounds how long the code is redeemable. The app
	// redeems it within the same deep-link handling that received it, so a
	// couple of minutes is generous while keeping the window tiny.
	oidcNativeExchangeTTL = 2 * time.Minute
)

// MintOIDCNativeExchangeCode builds the short-lived code the OIDC callback
// returns to the Android app. It binds the authenticated user to the app's
// PKCE challenge (RFC 7636 S256), so redeeming it requires the code verifier
// that never left the app.
func MintOIDCNativeExchangeCode(user models.User, codeChallenge string, cfg *config.Config) (string, error) {
	if cfg == nil || cfg.JWTSecretKey == "" {
		return "", errors.New("JWT secret key is empty")
	}
	if codeChallenge == "" {
		return "", errors.New("PKCE code challenge is required")
	}

	jtiBytes := make([]byte, 32)
	if _, err := rand.Read(jtiBytes); err != nil {
		return "", err // # pragma: no cover — crypto/rand only fails on catastrophic OS entropy exhaustion
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"purpose":        OIDCNativeExchangePurpose,
		"user_id":        user.ID,
		"username":       user.Username,
		"code_challenge": codeChallenge,
		"jti":            base64.RawURLEncoding.EncodeToString(jtiBytes),
		"iat":            now.Unix(),
		"exp":            now.Add(oidcNativeExchangeTTL).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(cfg.JWTSecretKey))
}

// ParseOIDCNativeExchangeCode validates an exchange code and returns the user
// it was minted for plus the bound PKCE challenge. ok is false for anything
// that is not a valid, unexpired, correctly-signed exchange code.
func ParseOIDCNativeExchangeCode(raw string, cfg *config.Config) (userID uint, codeChallenge string, ok bool) {
	if raw == "" || cfg == nil || cfg.JWTSecretKey == "" {
		return 0, "", false
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"HS256"}))
	token, err := parser.Parse(raw, func(t *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecretKey), nil
	})
	if err != nil || token == nil || !token.Valid {
		return 0, "", false
	}

	claims, isMap := token.Claims.(jwt.MapClaims)
	if !isMap {
		return 0, "", false // # pragma: no cover — jwt.Parse always constructs MapClaims; defensive
	}
	if purpose, _ := claims["purpose"].(string); purpose != OIDCNativeExchangePurpose {
		return 0, "", false
	}

	idFloat, isFloat := claims["user_id"].(float64)
	if !isFloat || idFloat <= 0 {
		return 0, "", false
	}
	challenge, _ := claims["code_challenge"].(string)
	if challenge == "" {
		return 0, "", false
	}

	return uint(idFloat), challenge, true
}

// VerifyOIDCNativePKCE reports whether verifier hashes to challenge under PKCE
// S256 (RFC 7636 §4.6). The comparison is constant-time so a wrong verifier
// cannot be recovered byte-by-byte.
func VerifyOIDCNativePKCE(verifier, challenge string) bool {
	if verifier == "" || challenge == "" {
		return false
	}
	computed := oauth2.S256ChallengeFromVerifier(verifier)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(challenge)) == 1
}
