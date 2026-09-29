package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"mycorrhizal/config"
	"mycorrhizal/models"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ---------------------------------------------------------------------------
// WebAuthn / passkeys (issue #593, backend half of #560).
//
// A passkey is an ALTERNATIVE second factor to TOTP: login demands either.
// This file holds the relying-party construction, the user adapter the
// go-webauthn library needs, credential (de)serialization, the factor
// bookkeeping shared with the TOTP paths, and the in-memory ceremony store.
// ---------------------------------------------------------------------------

// ErrWebAuthnNotConfigured is returned when the relying party cannot be built
// because FRONTEND_URL is not a concrete origin (e.g. the dev wildcard "*").
// A browser refuses a ceremony against a mismatched RPID, so failing here with
// a clear message beats a confusing browser-side failure.
var ErrWebAuthnNotConfigured = errors.New("passkeys require FRONTEND_URL to be set to the concrete origin users load the app from (not '*')")

// Second-factor method tokens as reported by POST /login's `methods` field.
// Mirrored by hand in frontend/Android clients.
const (
	SecondFactorTOTP     = "totp"
	SecondFactorWebAuthn = "webauthn"
)

// webAuthnCeremonyTTL bounds how long a begin→finish ceremony may stay open.
const webAuthnCeremonyTTL = 5 * time.Minute

// NewWebAuthn builds the relying party from cfg.FrontendURL: RPID is the bare
// hostname, the sole allowed origin is FrontendURL itself.
func NewWebAuthn(cfg *config.Config) (*webauthn.WebAuthn, error) {
	origin := strings.TrimRight(strings.TrimSpace(cfg.FrontendURL), "/")
	if origin == "" || origin == "*" {
		return nil, ErrWebAuthnNotConfigured
	}
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrWebAuthnNotConfigured
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "Mycorrhizal",
		RPID:          u.Hostname(),
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWebAuthnNotConfigured, err)
	}
	return wa, nil
}

// WebAuthnUser adapts a models.User plus its stored credentials to the
// library's webauthn.User interface.
type WebAuthnUser struct {
	User        models.User
	Credentials []webauthn.Credential
}

// WebAuthnID is the opaque user handle (a per-RP correlator, never displayed).
func (u *WebAuthnUser) WebAuthnID() []byte { return []byte(fmt.Sprintf("user-%d", u.User.ID)) }

func (u *WebAuthnUser) WebAuthnName() string { return u.User.Username }

func (u *WebAuthnUser) WebAuthnDisplayName() string { return u.User.Username }

func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// LoadWebAuthnUser loads the user's stored credentials into a library adapter.
func LoadWebAuthnUser(db *gorm.DB, user models.User) (*WebAuthnUser, []models.WebAuthnCredential, error) {
	var rows []models.WebAuthnCredential
	if err := db.Where("user_id = ?", user.ID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	creds := make([]webauthn.Credential, 0, len(rows))
	for _, r := range rows {
		creds = append(creds, credentialFromRow(r))
	}
	return &WebAuthnUser{User: user, Credentials: creds}, rows, nil
}

func credentialFromRow(r models.WebAuthnCredential) webauthn.Credential {
	var transports []protocol.AuthenticatorTransport
	for _, t := range strings.Split(r.Transports, ",") {
		if t = strings.TrimSpace(t); t != "" {
			transports = append(transports, protocol.AuthenticatorTransport(t))
		}
	}
	return webauthn.Credential{
		ID:              r.CredentialID,
		PublicKey:       r.PublicKey,
		AttestationType: r.AttestationType,
		Transport:       transports,
		Flags: webauthn.CredentialFlags{
			UserPresent:    true,
			BackupEligible: r.BackupEligible,
			BackupState:    r.BackupState,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    r.AAGUID,
			SignCount: r.SignCount,
		},
	}
}

// RowFromCredential converts a freshly registered library credential into a
// storable row for userID.
func RowFromCredential(userID uint, name string, c *webauthn.Credential) models.WebAuthnCredential {
	transports := make([]string, 0, len(c.Transport))
	for _, t := range c.Transport {
		transports = append(transports, string(t))
	}
	return models.WebAuthnCredential{
		UserID:          userID,
		CredentialID:    c.ID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		AAGUID:          c.Authenticator.AAGUID,
		SignCount:       c.Authenticator.SignCount,
		Transports:      strings.Join(transports, ","),
		BackupEligible:  c.Flags.BackupEligible,
		BackupState:     c.Flags.BackupState,
		Name:            name,
	}
}

// RecordWebAuthnUse persists the post-assertion counter / backup state and
// stamps last_used_at for the credential that just authenticated.
func RecordWebAuthnUse(db *gorm.DB, userID uint, c *webauthn.Credential) error {
	return db.Model(&models.WebAuthnCredential{}).
		Where("user_id = ? AND credential_id = ?", userID, c.ID).
		Updates(map[string]any{
			"sign_count":   c.Authenticator.SignCount,
			"backup_state": c.Flags.BackupState,
			"last_used_at": time.Now(),
		}).Error
}

// CountWebAuthnCredentials returns how many passkeys userID has enrolled.
func CountWebAuthnCredentials(db *gorm.DB, userID uint) (int64, error) {
	var n int64
	err := db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&n).Error
	return n, err
}

// SecondFactorMethods lists the enrolled second factors for user, in the
// stable order ["totp", "webauthn"]. Empty means the account has no second
// factor and password login alone mints a session.
func SecondFactorMethods(db *gorm.DB, user models.User) ([]string, error) {
	methods := []string{}
	if user.TOTPEnabled {
		methods = append(methods, SecondFactorTOTP)
	}
	n, err := CountWebAuthnCredentials(db, user.ID)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		methods = append(methods, SecondFactorWebAuthn)
	}
	return methods, nil
}

// ---------------------------------------------------------------------------
// Ceremony store. Session data (challenge etc.) must survive between the begin
// and finish requests. This is a single-process app, so an in-memory,
// single-use, short-TTL store keyed by (purpose, user) is enough and needs no
// cookie: begin overwrites any earlier in-flight ceremony for the same key,
// and finish consumes it so a captured response can never be replayed.
// ---------------------------------------------------------------------------

// Ceremony purposes.
const (
	CeremonyRegister = "register"
	CeremonyLogin    = "login"
	CeremonyProof    = "proof" // re-authentication for a sensitive operation
)

type ceremonyEntry struct {
	session webauthn.SessionData
	name    string
	expires time.Time
}

// CeremonyStore is a concurrency-safe single-use store of in-flight ceremonies.
type CeremonyStore struct {
	mu      sync.Mutex
	entries map[string]ceremonyEntry
	now     func() time.Time
}

// NewCeremonyStore returns an empty store.
func NewCeremonyStore() *CeremonyStore {
	return &CeremonyStore{entries: map[string]ceremonyEntry{}, now: time.Now}
}

// DefaultCeremonies is the process-wide store used by the controllers.
var DefaultCeremonies = NewCeremonyStore()

func ceremonyKey(purpose string, userID uint) string { return fmt.Sprintf("%s:%d", purpose, userID) }

// Put stores (replacing any prior) the ceremony for (purpose, userID).
func (s *CeremonyStore) Put(purpose string, userID uint, session webauthn.SessionData, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for k, e := range s.entries { // opportunistic sweep; the map stays tiny
		if now.After(e.expires) {
			delete(s.entries, k)
		}
	}
	s.entries[ceremonyKey(purpose, userID)] = ceremonyEntry{session: session, name: name, expires: now.Add(webAuthnCeremonyTTL)}
}

// Take removes and returns the ceremony for (purpose, userID); ok is false if
// there is none or it expired.
func (s *CeremonyStore) Take(purpose string, userID uint) (session webauthn.SessionData, name string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := ceremonyKey(purpose, userID)
	e, found := s.entries[k]
	if !found {
		return webauthn.SessionData{}, "", false
	}
	delete(s.entries, k)
	if s.now().After(e.expires) {
		return webauthn.SessionData{}, "", false
	}
	return e.session, e.name, true
}

// RandomCredentialLabel is a fallback label when the client supplies none.
func RandomCredentialLabel() string {
	return "Passkey " + strings.ReplaceAll(uuid.NewString(), "-", "")[:4]
}
