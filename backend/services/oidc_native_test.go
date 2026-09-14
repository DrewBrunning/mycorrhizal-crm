package services

import (
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func nativeTestConfig() *config.Config {
	return &config.Config{JWTSecretKey: "native-oidc-test-secret-key"}
}

// RFC 7636 Appendix B pins the S256 transform itself: if the verifier->challenge
// computation ever drifts (wrong hash, wrong base64 alphabet, wrong padding),
// every real exchange would fail closed but this test would say why.
func TestVerifyOIDCNativePKCE_RFC7636Vector(t *testing.T) {
	verifier := "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge := "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	assert.True(t, VerifyOIDCNativePKCE(verifier, challenge))
	assert.False(t, VerifyOIDCNativePKCE(verifier, "a-different-challenge"))
	assert.False(t, VerifyOIDCNativePKCE("a-different-verifier", challenge))
	assert.False(t, VerifyOIDCNativePKCE("", challenge))
	assert.False(t, VerifyOIDCNativePKCE(verifier, ""))
}

func TestOIDCNativeExchangeCode_RoundTrip(t *testing.T) {
	cfg := nativeTestConfig()
	user := models.User{Username: "native-user"}
	user.ID = 42

	code, err := MintOIDCNativeExchangeCode(user, "the-challenge", cfg)
	require.NoError(t, err)
	require.NotEmpty(t, code)

	userID, challenge, ok := ParseOIDCNativeExchangeCode(code, cfg)
	require.True(t, ok)
	assert.Equal(t, uint(42), userID)
	assert.Equal(t, "the-challenge", challenge)
}

func TestOIDCNativeExchangeCode_RequiresChallenge(t *testing.T) {
	_, err := MintOIDCNativeExchangeCode(models.User{Username: "u"}, "", nativeTestConfig())
	assert.Error(t, err)
}

func TestMintOIDCNativeExchangeCode_RejectsMissingSecret(t *testing.T) {
	user := models.User{Username: "u"}
	_, err := MintOIDCNativeExchangeCode(user, "challenge", &config.Config{})
	assert.Error(t, err)
	_, err = MintOIDCNativeExchangeCode(user, "challenge", nil)
	assert.Error(t, err)
}

func TestParseOIDCNativeExchangeCode_Rejections(t *testing.T) {
	cfg := nativeTestConfig()
	user := models.User{Username: "native-user"}
	user.ID = 7
	code, err := MintOIDCNativeExchangeCode(user, "challenge", cfg)
	require.NoError(t, err)

	// Wrong secret: signature check must fail.
	other := &config.Config{JWTSecretKey: "not-the-signing-secret"}
	_, _, ok := ParseOIDCNativeExchangeCode(code, other)
	assert.False(t, ok)

	// Tampered payload: signature check must fail.
	_, _, ok = ParseOIDCNativeExchangeCode(code+"x", cfg)
	assert.False(t, ok)

	// Empty / nil config are rejected rather than defaulting open.
	_, _, ok = ParseOIDCNativeExchangeCode("", cfg)
	assert.False(t, ok)
	_, _, ok = ParseOIDCNativeExchangeCode(code, nil)
	assert.False(t, ok)
}

// A code carrying a different purpose (e.g. a leaked 2FA challenge) must never
// be accepted as a native exchange code.
func TestParseOIDCNativeExchangeCode_RejectsOtherPurpose(t *testing.T) {
	cfg := nativeTestConfig()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose":        twoFactorChallengePurpose,
		"user_id":        float64(1),
		"code_challenge": "challenge",
		"exp":            time.Now().Add(time.Minute).Unix(),
	})
	raw, err := token.SignedString([]byte(cfg.JWTSecretKey))
	require.NoError(t, err)

	_, _, ok := ParseOIDCNativeExchangeCode(raw, cfg)
	assert.False(t, ok)
}

func TestParseOIDCNativeExchangeCode_RejectsExpired(t *testing.T) {
	cfg := nativeTestConfig()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"purpose":        OIDCNativeExchangePurpose,
		"user_id":        float64(1),
		"code_challenge": "challenge",
		"iat":            time.Now().Add(-10 * time.Minute).Unix(),
		"exp":            time.Now().Add(-time.Minute).Unix(),
	})
	raw, err := token.SignedString([]byte(cfg.JWTSecretKey))
	require.NoError(t, err)

	_, _, ok := ParseOIDCNativeExchangeCode(raw, cfg)
	assert.False(t, ok)
}

func TestParseOIDCNativeExchangeCode_RejectsMissingFields(t *testing.T) {
	cfg := nativeTestConfig()
	for name, claims := range map[string]jwt.MapClaims{
		"no user_id": {
			"purpose":        OIDCNativeExchangePurpose,
			"code_challenge": "challenge",
			"exp":            time.Now().Add(time.Minute).Unix(),
		},
		"no challenge": {
			"purpose": OIDCNativeExchangePurpose,
			"user_id": float64(1),
			"exp":     time.Now().Add(time.Minute).Unix(),
		},
		"zero user_id": {
			"purpose":        OIDCNativeExchangePurpose,
			"user_id":        float64(0),
			"code_challenge": "challenge",
			"exp":            time.Now().Add(time.Minute).Unix(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(cfg.JWTSecretKey))
			require.NoError(t, err)
			_, _, ok := ParseOIDCNativeExchangeCode(raw, cfg)
			assert.False(t, ok)
		})
	}
}
