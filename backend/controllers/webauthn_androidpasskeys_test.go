package controllers

import (
	"net/http"
	"testing"

	"mycorrhizal/androidpasskey"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Native Android passkeys (ADR 0034, issue #1293): a Credential Manager
// ceremony reports clientDataJSON.origin = android:apk-key-hash:<b64url of the
// signing-cert SHA-256> while the RP ID stays the FRONTEND_URL host. These
// tests drive the real begin/finish endpoints with the virtual authenticator
// signing exactly that origin.

const (
	// Independently computed: python3 hashlib.sha256(b"mycorrhizal-test-cert").
	androidTestCertColon  = "A3:65:E6:D6:28:35:03:7A:56:FD:DC:C8:88:80:F5:5F:EC:99:F4:FD:02:1B:A4:C0:AF:50:91:72:34:2E:6E:C0"
	androidTestCertOrigin = "android:apk-key-hash:o2Xm1ig1A3pW_dzIiID1X-yZ9P0CG6TAr1CRcjQubsA"
	// The built-in obtainium channel (issue #1334), always trusted when on.
	androidObtainiumColon  = "24:CF:16:6F:59:36:A6:AD:05:B4:4B:6B:56:9D:71:17:7A:03:8D:4A:06:0F:A4:85:E1:50:2E:73:29:EF:06:5E"
	androidObtainiumOrigin = "android:apk-key-hash:JM8Wb1k2pq0FtEtrVp1xF3oDjUoGD6SF4VAucynvBl4"
	// A different, equally well-formed certificate (bytes 0x00..0x1f).
	androidOtherCertOrigin = "android:apk-key-hash:AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8"
)

func (e *waEnv) enableAndroid() {
	e.cfg.WebAuthnAndroidEnabled = true
	e.cfg.WebAuthnAndroidCertSHA256 = []string{androidTestCertColon}
}

func (e *waEnv) androidAuth(origin string) *virtualAuthenticator {
	return newVirtualAuthenticator(e.t, origin, waTestRPID)
}

func TestWebAuthnAndroid_OpaqueOriginAcceptedForRegisterAndLogin(t *testing.T) {
	e := newWAEnv(t)
	e.enableAndroid()
	va := e.androidAuth(androidTestCertOrigin)

	_, _, _ = e.enroll(va, "Pixel", e.session())
	require.Equal(t, int64(1), e.credentialCount())

	pending, methods := e.passwordStep()
	assert.Equal(t, []string{"webauthn"}, methods)
	w, cookies := e.passkeyLogin(va, pending)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NotNil(t, cookies["auth_token"])
}

func TestWebAuthnAndroid_WrongCertHashRejected(t *testing.T) {
	e := newWAEnv(t)
	e.enableAndroid()

	// Registration with an unlisted certificate hash is refused; nothing stored.
	bad := e.androidAuth(androidOtherCertOrigin)
	w, _ := e.register(bad, "Rogue", e.session())
	assert.NotEqual(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(0), e.credentialCount())

	// Login: enroll with the listed hash, then a different-hash client (same
	// key, so only the origin differs) is refused.
	good := e.androidAuth(androidTestCertOrigin)
	e.enroll(good, "Pixel", e.session())
	imposter := e.androidAuth(androidOtherCertOrigin)
	imposter.key, imposter.credentialID = good.key, good.credentialID
	pending, _ := e.passwordStep()
	w, cookies := e.passkeyLogin(imposter, pending)
	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

func TestWebAuthnAndroid_OpaqueOriginRejectedWhenFeatureOff(t *testing.T) {
	// Switch off, but the fingerprint is even configured: still nothing accepted.
	e := newWAEnv(t)
	e.cfg.WebAuthnAndroidCertSHA256 = []string{androidTestCertColon}
	va := e.androidAuth(androidTestCertOrigin)
	w, _ := e.register(va, "Pixel", e.session())
	assert.NotEqual(t, http.StatusCreated, w.Code, w.Body.String())
	assert.Equal(t, int64(0), e.credentialCount())

	// Enrolled while on, then switched off (restart): the Android ceremony no
	// longer verifies.
	e.enableAndroid()
	e.enroll(va, "Pixel", e.session())
	e.cfg.WebAuthnAndroidEnabled = false
	pending, _ := e.passwordStep()
	w, cookies := e.passkeyLogin(va, pending)
	assert.NotEqual(t, http.StatusOK, w.Code)
	assert.Nil(t, cookies["auth_token"])
}

func TestWebAuthnAndroid_WebCeremonyUnchangedWhenEnabled(t *testing.T) {
	e := newWAEnv(t)
	e.enableAndroid()
	web := e.auth() // https FRONTEND_URL origin
	e.enroll(web, "Laptop", e.session())
	pending, _ := e.passwordStep()
	w, _ := e.passkeyLogin(web, pending)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
}

func TestNewWebAuthn_AndroidOriginsOnlyWhenEffective(t *testing.T) {
	e := newWAEnv(t)

	off, err := services.NewWebAuthn(e.cfg)
	require.NoError(t, err)
	assert.Empty(t, off.Config.RPOpaqueOrigins)

	e.enableAndroid()
	on, err := services.NewWebAuthn(e.cfg)
	require.NoError(t, err)
	assert.Equal(t, []string{androidObtainiumOrigin, androidTestCertOrigin}, on.Config.RPOpaqueOrigins)

	// The https origin set and RP ID are byte-identical either way.
	assert.Equal(t, off.Config.RPOrigins, on.Config.RPOrigins)
	assert.Equal(t, []string{waTestOrigin}, on.Config.RPOrigins)
	assert.Equal(t, off.Config.RPID, on.Config.RPID)

	// Switch on but the URL fails the public-name check (.lan): stays off.
	e.cfg.FrontendURL = "https://crm.lan"
	inv, err := services.NewWebAuthn(e.cfg)
	require.NoError(t, err)
	assert.Empty(t, inv.Config.RPOpaqueOrigins)

	// The golden-vector origin matches the package's own derivation.
	fp, err := androidpasskey.ParseFingerprint(androidTestCertColon)
	require.NoError(t, err)
	assert.Equal(t, androidTestCertOrigin, fp.Origin())
}
