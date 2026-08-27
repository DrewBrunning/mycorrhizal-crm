package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Issue #412: the OIDC auth-flow attack matrix. This file adds the rows the
// existing OIDC test suite (oidc_service_test.go, oidc_claims_test.go,
// oidc_userinfo_test.go, controllers/oidc_controller_test.go) did not yet
// pin: PKCE code-verifier binding, authorization-code replay, issuer
// validation, and the OIDC_TRUST_EMAIL / OIDC_AUTO_PROVISION account-mix-up
// matrix. Everything else in the issue's checklist (state validation, nonce
// validation, azp/audience validation, login CSRF via the state cookie,
// post-login/post-logout redirect targets) already had real, non-vacuous
// coverage before this file -- see those files for it.

// --- PKCE binding + authorization-code replay ---
//
// fakeIDP (oidc_service_test.go) accepts any code and any PKCE verifier, so a
// "wrong verifier" or "replayed code" test against it would pass whether or
// not ExchangeAndVerify actually sends the right values -- it proves nothing.
// strictIDP is the real oracle: it enforces the two protections a
// spec-compliant IdP is responsible for (RFC 7636 code_verifier binding, and
// single-use authorization codes) so these tests fail if our client ever
// stops sending them correctly.
type strictIDP struct {
	Server   *httptest.Server
	key      *rsa.PrivateKey
	kid      string
	clientID string

	// ExpectedVerifier is the PKCE verifier the (simulated) /auth step bound
	// its code_challenge to. /token rejects any code_verifier that doesn't
	// match it, exactly as a real IdP compares SHA256(code_verifier) against
	// the stored challenge.
	ExpectedVerifier string

	IDTokenClaims jwt.MapClaims

	usedCodes map[string]bool
}

func newStrictIDP(t *testing.T, clientID string) *strictIDP {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	idp := &strictIDP{
		key:       key,
		kid:       "strict-kid",
		clientID:  clientID,
		usedCodes: map[string]bool{},
		IDTokenClaims: jwt.MapClaims{
			"sub":            "test-subject",
			"email":          "test-user@example.com",
			"email_verified": true,
			"name":           "Test User",
		},
	}
	idp.Server = httptest.NewServer(http.HandlerFunc(idp.handle))
	t.Cleanup(idp.Server.Close)
	return idp
}

func (idp *strictIDP) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.URL.Path {
	case "/.well-known/openid-configuration":
		fmt.Fprintf(w, `{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"jwks_uri":%q}`,
			idp.Server.URL, idp.Server.URL+"/auth", idp.Server.URL+"/token", idp.Server.URL+"/keys")
	case "/keys":
		n := base64.RawURLEncoding.EncodeToString(idp.key.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(idp.key.PublicKey.E)).Bytes())
		fmt.Fprintf(w, `{"keys":[{"kty":"RSA","kid":%q,"use":"sig","alg":"RS256","n":%q,"e":%q}]}`, idp.kid, n, e)
	case "/token":
		idp.handleToken(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (idp *strictIDP) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, `{"error":"invalid_request"}`, http.StatusBadRequest)
		return
	}

	code := r.Form.Get("code")
	if idp.usedCodes[code] {
		// RFC 6749 4.1.2: an authorization code MUST NOT be used more than
		// once. A real IdP revokes any tokens issued from prior uses too;
		// this fake only needs to reject the replay for the matrix's
		// purposes.
		http.Error(w, `{"error":"invalid_grant","error_description":"code already redeemed"}`, http.StatusBadRequest)
		return
	}

	verifier := r.Form.Get("code_verifier")
	if verifier != idp.ExpectedVerifier {
		http.Error(w, `{"error":"invalid_grant","error_description":"code_verifier does not match code_challenge"}`, http.StatusBadRequest)
		return
	}

	idp.usedCodes[code] = true

	signed, err := idp.signIDToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, `{"access_token":"test-access-token","token_type":"Bearer","id_token":%q}`, signed)
}

func (idp *strictIDP) signIDToken() (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{}
	for k, v := range idp.IDTokenClaims {
		claims[k] = v
	}
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = idp.Server.URL
	}
	if _, ok := claims["aud"]; !ok {
		claims["aud"] = idp.clientID
	}
	claims["exp"] = now.Add(time.Hour).Unix()
	claims["iat"] = now.Unix()

	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = idp.kid
	return tok.SignedString(idp.key)
}

func TestExchangeAndVerify_PKCEWrongVerifierRejected(t *testing.T) {
	idp := newStrictIDP(t, "test-client")
	idp.ExpectedVerifier = "the-verifier-the-authorize-request-was-actually-bound-to"

	provider, err := InitOIDCProvider(context.Background(), testOIDCConfig(idp.Server.URL, "test-client"))
	require.NoError(t, err)

	// A substituted/guessed verifier -- what an attacker sends when trying to
	// redeem a stolen authorization code under their own PKCE flow -- must be
	// rejected by the token endpoint, and ExchangeAndVerify must surface that
	// as an error rather than proceeding as if it succeeded.
	idToken, token, rawIDToken, err := provider.ExchangeAndVerify(context.Background(), "auth-code", "attacker-supplied-verifier")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to exchange code")
	assert.Nil(t, idToken)
	assert.Nil(t, token)
	assert.Empty(t, rawIDToken)
}

func TestExchangeAndVerify_PKCECorrectVerifierSucceeds(t *testing.T) {
	idp := newStrictIDP(t, "test-client")
	idp.ExpectedVerifier = "the-real-verifier-the-authorize-request-was-bound-to"

	provider, err := InitOIDCProvider(context.Background(), testOIDCConfig(idp.Server.URL, "test-client"))
	require.NoError(t, err)

	// Sanity companion to the "wrong verifier" test above: strictIDP isn't
	// just rejecting everything.
	idToken, token, _, err := provider.ExchangeAndVerify(context.Background(), "auth-code", idp.ExpectedVerifier)
	require.NoError(t, err)
	assert.NotNil(t, idToken)
	assert.NotNil(t, token)
}

func TestExchangeAndVerify_RejectsAuthorizationCodeReplay(t *testing.T) {
	idp := newStrictIDP(t, "test-client")
	idp.ExpectedVerifier = "single-use-verifier"

	provider, err := InitOIDCProvider(context.Background(), testOIDCConfig(idp.Server.URL, "test-client"))
	require.NoError(t, err)

	_, _, _, err = provider.ExchangeAndVerify(context.Background(), "one-time-code", idp.ExpectedVerifier)
	require.NoError(t, err, "the first redemption of a fresh code must succeed")

	// Replaying the exact same code -- e.g. an attacker who intercepted the
	// callback URL and races the real user to /token -- must be rejected on
	// the second attempt.
	idToken, token, rawIDToken, err := provider.ExchangeAndVerify(context.Background(), "one-time-code", idp.ExpectedVerifier)
	require.Error(t, err, "redeeming the same authorization code twice must be rejected")
	assert.Contains(t, err.Error(), "failed to exchange code")
	assert.Nil(t, idToken)
	assert.Nil(t, token)
	assert.Empty(t, rawIDToken)
}

// --- Issuer validation ---

// go-oidc's verifier checks the ID token's `iss` against the issuer pinned at
// InitOIDCProvider (from discovery), but nothing in this codebase's own test
// suite exercised the rejection path -- every existing fakeIDP-based test
// lets signIDToken default `iss` to the real server URL.
func TestExchangeAndVerify_RejectsMismatchedIssuer(t *testing.T) {
	idp := newFakeIDP(t, "test-client")
	defer idp.Close()

	// A token claiming to be issued by a different party than the one this
	// verifier was initialized against. Even though it's signed by a key
	// this IdP's own JWKS publishes, a mismatched issuer must still be
	// rejected -- otherwise a compromised or malicious secondary issuer
	// sharing infrastructure with the real one could mint tokens for it.
	idp.IDTokenClaims["iss"] = "https://not-the-configured-issuer.example.com"

	provider, err := InitOIDCProvider(context.Background(), testOIDCConfig(idp.Server.URL, "test-client"))
	require.NoError(t, err)

	idToken, token, rawIDToken, err := provider.ExchangeAndVerify(context.Background(), "test-code", "test-pkce-verifier")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to verify id_token")
	assert.Nil(t, idToken)
	assert.Nil(t, token)
	assert.Empty(t, rawIDToken)
}

// --- Account mix-up / identity linking: OIDC_TRUST_EMAIL matrix ---
//
// These use the real migrated schema (database.InitDB), not AutoMigrate --
// the invariant under test depends on users.email genuinely being UNIQUE NOT
// NULL exactly as migration 000001 defines it, per CLAUDE.md's backend
// testing guidance.

func realUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := dbtest.New(t)
	return db
}

// The core invariant from #412: an OIDC identity must never authenticate as
// another local account merely because an attacker controls/knows an email
// address. With OIDC_TRUST_EMAIL off (the default), only a *verified* email
// claim may link to an existing local account.
func TestFindOrProvisionUser_TrustEmailOff_UnverifiedEmailDoesNotLinkExistingAccount(t *testing.T) {
	db := realUserDB(t)
	cfg := &config.Config{OIDC: config.OIDCConfig{TrustEmail: false, AllowAutoProvision: false}}

	existing := models.User{Username: "victim", Password: "irrelevant-hash", Email: "victim@example.com"}
	require.NoError(t, db.Create(&existing).Error)

	claims := &OIDCClaims{
		Subject:       "attacker-subject",
		Provider:      "https://idp.example.com",
		Email:         "victim@example.com",
		EmailVerified: false, // the attacker's IdP did not verify this address
		Name:          "Attacker",
	}
	user, err := FindOrProvisionUser(db, claims, cfg)

	require.ErrorIs(t, err, ErrOIDCUserNotFound, "an unverified email claim must not authenticate as the matching local account")
	assert.Nil(t, user)

	var reloaded models.User
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	assert.Nil(t, reloaded.OIDCSubject, "the victim's account must remain unlinked")
}

// Same scenario with auto-provisioning also enabled: FindOrProvisionUser must
// neither silently link to the existing account nor create a colliding
// shadow account (blocked by the UNIQUE email constraint) -- it must fail
// cleanly either way.
func TestFindOrProvisionUser_TrustEmailOff_UnverifiedEmailWithAutoProvisionDoesNotHijackOrDuplicate(t *testing.T) {
	db := realUserDB(t)
	cfg := &config.Config{OIDC: config.OIDCConfig{TrustEmail: false, AllowAutoProvision: true}}

	existing := models.User{Username: "victim", Password: "irrelevant-hash", Email: "victim@example.com"}
	require.NoError(t, db.Create(&existing).Error)

	claims := &OIDCClaims{
		Subject:       "attacker-subject",
		Provider:      "https://idp.example.com",
		Email:         "victim@example.com",
		EmailVerified: false,
		Name:          "Attacker",
	}
	user, err := FindOrProvisionUser(db, claims, cfg)

	require.Error(t, err, "must not silently provision a colliding account or link to the existing one")
	assert.Nil(t, user)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Where("email = ?", "victim@example.com").Count(&count).Error)
	assert.Equal(t, int64(1), count, "no duplicate/shadow account may be created")

	var reloaded models.User
	require.NoError(t, db.First(&reloaded, existing.ID).Error)
	assert.Nil(t, reloaded.OIDCSubject, "the victim's account must remain unlinked")
}

// The opposite configuration: OIDC_TRUST_EMAIL=true (documented as safe only
// for a trusted self-hosted provider) does get the linking behavior --
// pinning that the toggle actually changes something, not just that the safe
// default is safe.
func TestFindOrProvisionUser_TrustEmailOn_UnverifiedEmailLinksExistingAccount(t *testing.T) {
	db := realUserDB(t)
	cfg := &config.Config{OIDC: config.OIDCConfig{TrustEmail: true, AllowAutoProvision: false}}

	existing := models.User{Username: "person", Password: "irrelevant-hash", Email: "person@example.com"}
	require.NoError(t, db.Create(&existing).Error)

	claims := &OIDCClaims{
		Subject:       "new-subject",
		Provider:      "https://idp.example.com",
		Email:         "person@example.com",
		EmailVerified: false,
		Name:          "Person",
	}
	user, err := FindOrProvisionUser(db, claims, cfg)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, existing.ID, user.ID)
	require.NotNil(t, user.OIDCSubject)
	assert.Equal(t, "new-subject", *user.OIDCSubject)
}

// OIDC_AUTO_PROVISION off, matrix row 6: an entirely unknown identity (no
// subject match, no email match at all) creates no account.
func TestFindOrProvisionUser_AutoProvisionOff_UnknownIdentityCreatesNoAccount(t *testing.T) {
	db := realUserDB(t)
	cfg := &config.Config{OIDC: config.OIDCConfig{AllowAutoProvision: false}}

	claims := &OIDCClaims{
		Subject:       "nobody-subject",
		Provider:      "https://idp.example.com",
		Email:         "nobody@example.com",
		EmailVerified: true,
	}
	user, err := FindOrProvisionUser(db, claims, cfg)

	require.ErrorIs(t, err, ErrOIDCUserNotFound)
	assert.Nil(t, user)

	var count int64
	require.NoError(t, db.Model(&models.User{}).Count(&count).Error)
	assert.Zero(t, count)
}

// Matrix row 8: DeleteUser hard-deletes (Unscoped) specifically because
// users.email and the (oidc_subject, oidc_provider) index are genuinely
// UNIQUE, not partial-unique -- a soft-deleted row would block this forever.
// A later OIDC login for the same subject/email must re-provision cleanly
// rather than dying on a stale unique constraint, and must land on a fresh
// account rather than resurrecting the deleted one's row/data.
func TestFindOrProvisionUser_ReprovisionsAfterAccountHardDeleted(t *testing.T) {
	db := realUserDB(t)
	cfg := &config.Config{OIDC: config.OIDCConfig{AllowAutoProvision: true}}

	subject := "returning-subject"
	providerURL := "https://idp.example.com"
	original := models.User{
		Username:     "returning",
		Email:        "returning@example.com",
		OIDCSubject:  &subject,
		OIDCProvider: &providerURL,
	}
	require.NoError(t, db.Create(&original).Error)
	require.NoError(t, db.Unscoped().Delete(&original).Error)

	claims := &OIDCClaims{
		Subject:       subject,
		Provider:      providerURL,
		Email:         "returning@example.com",
		EmailVerified: true,
		Name:          "Returning Person",
	}
	user, err := FindOrProvisionUser(db, claims, cfg)

	require.NoError(t, err)
	require.NotNil(t, user)
	assert.NotEqual(t, original.ID, user.ID, "a genuinely new account, not a resurrection of the deleted row")
	require.NotNil(t, user.OIDCSubject)
	assert.Equal(t, subject, *user.OIDCSubject)
}
