package services

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/internal/dbtest"
	"mycorrhizal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewWebAuthn_DerivesRPIDFromFrontendURL(t *testing.T) {
	cases := map[string]string{
		"https://crm.example.com":      "crm.example.com",
		"https://crm.example.com/":     "crm.example.com",
		"https://crm.example.com:8443": "crm.example.com",
		"http://localhost:3000":        "localhost",
	}
	for origin, wantRPID := range cases {
		wa, err := NewWebAuthn(&config.Config{FrontendURL: origin})
		require.NoError(t, err, origin)
		assert.Equal(t, wantRPID, wa.Config.RPID, origin)
		assert.Equal(t, []string{strings.TrimRight(origin, "/")}, wa.Config.RPOrigins, origin)
	}
}

func TestNewWebAuthn_RejectsNonConcreteOrigins(t *testing.T) {
	for _, origin := range []string{"", "  ", "*", "not a url", "ftp://crm.example.com", "https://", "crm.example.com"} {
		_, err := NewWebAuthn(&config.Config{FrontendURL: origin})
		assert.ErrorIs(t, err, ErrWebAuthnNotConfigured, origin)
	}
}

func TestSecondFactorMethods(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "sfm", Email: "sfm@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	m, err := SecondFactorMethods(db, user)
	require.NoError(t, err)
	assert.Equal(t, []string{}, m, "no factor is an empty non-nil list")

	user.TOTPEnabled = true
	m, err = SecondFactorMethods(db, user)
	require.NoError(t, err)
	assert.Equal(t, []string{"totp"}, m)

	require.NoError(t, db.Create(&models.WebAuthnCredential{UserID: user.ID, CredentialID: []byte("c"), PublicKey: []byte("p")}).Error)
	m, err = SecondFactorMethods(db, user)
	require.NoError(t, err)
	assert.Equal(t, []string{"totp", "webauthn"}, m)

	user.TOTPEnabled = false
	m, err = SecondFactorMethods(db, user)
	require.NoError(t, err)
	assert.Equal(t, []string{"webauthn"}, m)
}

func TestCredentialRowRoundTrip(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "rt", Email: "rt@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)

	in := &webauthn.Credential{
		ID:              []byte("cred-id"),
		PublicKey:       []byte("pub"),
		AttestationType: "none",
		Transport:       []protocol.AuthenticatorTransport{protocol.USB, protocol.NFC},
		Flags:           webauthn.CredentialFlags{BackupEligible: true, BackupState: true},
		Authenticator:   webauthn.Authenticator{AAGUID: []byte("0123456789abcdef"), SignCount: 7},
	}
	row := RowFromCredential(user.ID, "Key", in)
	require.NoError(t, db.Create(&row).Error)
	assert.NotEmpty(t, row.ID, "BeforeCreate must mint the UUID")

	u, rows, err := LoadWebAuthnUser(db, user)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Len(t, u.Credentials, 1)
	got := u.Credentials[0]
	assert.Equal(t, in.ID, got.ID)
	assert.Equal(t, in.PublicKey, got.PublicKey)
	assert.Equal(t, []protocol.AuthenticatorTransport{protocol.USB, protocol.NFC}, got.Transport)
	assert.True(t, got.Flags.BackupEligible)
	assert.True(t, got.Flags.BackupState)
	assert.Equal(t, uint32(7), got.Authenticator.SignCount)
	assert.Equal(t, in.Authenticator.AAGUID, got.Authenticator.AAGUID)

	// Adapter identity.
	assert.Equal(t, []byte(fmt.Sprintf("user-%d", user.ID)), u.WebAuthnID())
	assert.Equal(t, "rt", u.WebAuthnName())
	assert.Equal(t, "rt", u.WebAuthnDisplayName())

	// RecordWebAuthnUse persists counter/backup state and stamps last_used_at.
	got.Authenticator.SignCount = 9
	got.Flags.BackupState = false
	require.NoError(t, RecordWebAuthnUse(db, user.ID, &got))
	var reloaded models.WebAuthnCredential
	require.NoError(t, db.First(&reloaded, "id = ?", row.ID).Error)
	assert.Equal(t, uint32(9), reloaded.SignCount)
	assert.False(t, reloaded.BackupState)
	require.NotNil(t, reloaded.LastUsedAt)

	n, err := CountWebAuthnCredentials(db, user.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)
}

func TestUniqueNaturalKeyIsEnforced(t *testing.T) {
	db := dbtest.New(t)
	user := models.User{Username: "nk", Email: "nk@example.com", Password: "x"}
	require.NoError(t, db.Create(&user).Error)
	mk := func() error {
		return db.Create(&models.WebAuthnCredential{UserID: user.ID, CredentialID: []byte("same"), PublicKey: []byte("p")}).Error
	}
	require.NoError(t, mk())
	require.ErrorContains(t, mk(), "UNIQUE constraint failed: webauthn_credentials",
		"(user_id, credential_id) is a plain unique index")
}

func TestCeremonyStore_SingleUseAndExpiry(t *testing.T) {
	s := NewCeremonyStore()
	now := time.Now()
	s.now = func() time.Time { return now }

	s.Put(CeremonyLogin, 1, webauthn.SessionData{Challenge: "a"}, "n")
	sess, name, ok := s.Take(CeremonyLogin, 1)
	require.True(t, ok)
	assert.Equal(t, "a", sess.Challenge)
	assert.Equal(t, "n", name)
	_, _, ok = s.Take(CeremonyLogin, 1)
	assert.False(t, ok, "a ceremony is single-use")

	// Purpose and user scope the key.
	s.Put(CeremonyLogin, 1, webauthn.SessionData{Challenge: "a"}, "")
	_, _, ok = s.Take(CeremonyRegister, 1)
	assert.False(t, ok)
	_, _, ok = s.Take(CeremonyLogin, 2)
	assert.False(t, ok)

	// Put overwrites an in-flight ceremony.
	s.Put(CeremonyLogin, 1, webauthn.SessionData{Challenge: "b"}, "")
	sess, _, ok = s.Take(CeremonyLogin, 1)
	require.True(t, ok)
	assert.Equal(t, "b", sess.Challenge)

	// Expiry: Take refuses a stale entry, Put sweeps stale ones.
	s.Put(CeremonyProof, 5, webauthn.SessionData{}, "")
	now = now.Add(webAuthnCeremonyTTL + time.Second)
	_, _, ok = s.Take(CeremonyProof, 5)
	assert.False(t, ok, "expired ceremony must be refused")

	s.Put(CeremonyProof, 6, webauthn.SessionData{}, "")
	now = now.Add(webAuthnCeremonyTTL + time.Second)
	s.Put(CeremonyProof, 7, webauthn.SessionData{}, "")
	assert.Len(t, s.entries, 1, "stale entries are swept on Put")
}

func TestRandomCredentialLabel(t *testing.T) {
	a, b := RandomCredentialLabel(), RandomCredentialLabel()
	assert.Regexp(t, `^Passkey [0-9a-f]{4}$`, a)
	assert.Regexp(t, `^Passkey [0-9a-f]{4}$`, b)
}

// Issue #1793: an unnamed passkey should be recognisable, not "Passkey a419".
func TestDefaultCredentialLabel(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{
			name: "headless chrome on linux",
			ua:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/153.0.0.0 Safari/537.36",
			want: "Chrome 153 on Linux",
		},
		{
			name: "firefox on linux",
			ua:   "Mozilla/5.0 (X11; Linux x86_64; rv:120.0) Gecko/20100101 Firefox/120.0",
			want: "Firefox 120 on Linux",
		},
		{
			name: "edge on windows",
			ua:   "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0",
			want: "Edge 120 on Windows",
		},
		{
			name: "safari on macOS",
			ua:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Safari/605.1.15",
			want: "Safari 17 on macOS",
		},
		{
			name: "chrome on android",
			ua:   "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36",
			want: "Chrome 120 on Android",
		},
		{
			name: "safari on iphone",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			want: "Safari 17 on iPhone",
		},
		{
			name: "chrome on iphone (CriOS, not Safari)",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/120.0.6099.119 Mobile/15E148 Safari/604.1",
			want: "Chrome 120 on iPhone",
		},
		{
			name: "firefox on iphone (FxiOS, not Safari)",
			ua:   "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/121.0 Mobile/15E148 Safari/605.1.15",
			want: "Firefox 121 on iPhone",
		},
		{
			name: "edge on ipad (EdgiOS, not Safari)",
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) EdgiOS/120.0.2210.126 Version/17.0 Mobile/15E148 Safari/604.1",
			want: "Edge 120 on iPad",
		},
		{
			name: "safari on ipad",
			ua:   "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
			want: "Safari 17 on iPad",
		},
		{
			name: "opera on linux",
			ua:   "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/105.0.0.0 Safari/537.36 OPR/105.0.0.0",
			want: "Opera 105 on Linux",
		},
		{
			name: "samsung internet on android",
			ua:   "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/23.0 Chrome/115.0.0.0 Mobile Safari/537.36",
			want: "Samsung Internet 23 on Android",
		},
		{
			name: "chrome on chrome os",
			ua:   "Mozilla/5.0 (X11; CrOS x86_64 14541.0.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			want: "Chrome 120 on Chrome OS",
		},
		{
			name: "browser only",
			ua:   "Firefox/120.0",
			want: "Firefox 120",
		},
		{
			name: "platform only",
			ua:   "Mozilla/5.0 (X11; Linux x86_64) SomeEngine/1.0",
			want: "Linux",
		},
		{
			name: "unknown falls back to a random label",
			ua:   "Some Completely Unknown Agent/1.0",
			want: "^Passkey [0-9a-f]{4}$",
		},
		{
			name: "empty falls back to a random label",
			ua:   "",
			want: "^Passkey [0-9a-f]{4}$",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultCredentialLabel(tt.ua)
			if strings.HasPrefix(tt.want, "^") {
				assert.Regexp(t, tt.want, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMajorVersionAfter(t *testing.T) {
	assert.Equal(t, "153", majorVersionAfter("HeadlessChrome/153.0.0.0", "Chrome/"))
	assert.Equal(t, "", majorVersionAfter("Firefox/", "Firefox/"))
	assert.Equal(t, "", majorVersionAfter("Chrome/120", "Firefox/"))
}
