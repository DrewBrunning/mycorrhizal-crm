// Package atrest implements field-level encryption of sensitive columns at
// rest (issue #380, ASVS V6.4/V8.3): AES-256-GCM (AEAD) over the genuinely
// sensitive, non-searchable columns in the schema, with a wrapped-DEK
// envelope so master-key rotation never requires re-encrypting the payloads.
//
// Threat model (self-hosted; the operator owns the disk): the encryption
// closes the "plaintext PII in a DB dump" gap for columns that are NOT
// searchable. The FTS-indexed columns (notes.content, activities.*, and the
// flat contact search fields) are deliberately left plaintext and documented
// as such in docs/security/asvs-l2.md — encrypting them would break the FTS5
// triggers that index them directly (000007/000010/000020). Everything the
// issue's "How to verify" step demands holds here:
//
//   - a dump of the .db shows no plaintext PII in the sensitive columns;
//   - key rotation re-encrypts without data loss (rotation only rewraps the
//     single wrapped DEK — the payload bytes are untouched);
//   - restore-from-backup works with the key; a wrong key fails closed
//     (GCM authentication rejects it at boot, before any data is served);
//   - existing rows are backfilled, not dropped (atrest.Backfill, asserted
//     row-count-preserving by its test);
//   - search/exports still behave (the serializers decrypt transparently on
//     every GORM read, including db.Raw().Scan; the FTS columns are the
//     documented plaintext exception).
//
// "Lost key = lost data, by design": the DEK is stored only wrapped by the
// master key. If the master key is lost, the DEK cannot be unwrapped and
// every encrypted column becomes undecryptable. There is deliberately no
// escrow/backdoor.
//
// Key layering
//
//	DATA_ENCRYPTION_KEY  (base64, 32 bytes)        ─┐
//	DATA_ENCRYPTION_KEY_FILE (path to such a key)  ─┼─► KEK (master key)
//	HKDF-SHA256(JWT_SECRET_KEY) fallback           ─┘     │
//	                                                       ▼
//	random 32-byte DEK  ──AES-256-GCM(KEK)──►  data_encryption_keys.wrapped_dek
//	                                            (key_id 'main')
//	                                                       │
//	                                                       ▼
//	field ciphertext = "encv1:main:" + base64url(nonce ‖ ct ‖ tag)
//
// The single wrapped DEK is a deliberate simplification of the issue's
// "per-row data keys": with one deployment-level DEK, rotating the master
// key is a one-row UPDATE (unwrap, rewrap) and no payload is ever touched —
// strictly cheaper than per-row keys and with the same rotation property,
// since a KEK compromise unwraps every row's DEK regardless. Documented in
// the V6.4.1 ASVS row.
package atrest

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
	"gorm.io/gorm"
)

// ciphertextPrefix marks a value as at-rest-encrypted and carries the format
// version. Any value without this prefix is treated as legacy plaintext
// (pre-backfill rows, or tests that never initialized the layer) and passed
// through unchanged — that pass-through is what makes Backfill idempotent
// and the migration window safe.
const ciphertextPrefix = "encv1:"

// keyID identifies the wrapped DEK row. Fixed per deployment today; kept in
// the ciphertext so a future key-rotation that adds a second DEK can still
// decrypt rows written under the old one.
const keyID = "main"

// maxKeySize / keySize are the AES-256 key length in bytes.
const keySize = 32

// atrestKEKInfo is the HKDF domain-separation string for deriving the KEK
// from JWT_SECRET_KEY when no dedicated DATA_ENCRYPTION_KEY is configured.
const atrestKEKInfo = "mycorrhizal-at-rest-master-key-v1"

// Engine holds the unwrapped DEK plus the master key that wrapped it (kept
// for rotation). Loaded once at startup by Initialize.
type Engine struct {
	mu  sync.RWMutex
	dek []byte
	kek []byte
	on  bool
}

var engine Engine

// DecodeMasterKey parses a base64 string into a 32-byte master key,
// rejecting anything that does not decode to exactly keySize bytes.
func DecodeMasterKey(raw string) ([]byte, error) {
	kek, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return nil, fmt.Errorf("not valid base64: %w", err)
	}
	if len(kek) != keySize {
		return nil, fmt.Errorf("must decode to %d bytes, got %d", keySize, len(kek))
	}
	return kek, nil
}

// ResolveMasterKey implements the resolution order for the at-rest master
// key (KEK), parameterized on explicit inputs rather than reading the
// environment itself:
//
//  1. dataKey (base64, 32 bytes), else
//  2. dataKeyFile (path whose trimmed contents are base64, 32 bytes), else
//  3. HKDF-SHA256 over jwtSecret (the zero-config fallback, mirroring
//     services/credential_crypto.go's coupling so existing deployments keep
//     working without a new required variable).
//
// Returns (nil, nil) when none of the three inputs yields a key — callers
// treat that as "encryption not configured" rather than an error; production
// always has JWT_SECRET_KEY (config validation requires it), so production
// always encrypts.
//
// In-process callers that already hold a validated config.Config must pass
// its DataEncryptionKey/DataEncryptionKeyFile/JWTSecretKey fields here
// instead of calling EncryptionKey (which re-reads the environment
// independently) — otherwise config.Validate and the value actually used to
// arm encryption are two separate reads of the same variables that could, in
// principle, disagree (issue #938).
func ResolveMasterKey(dataKey, dataKeyFile, jwtSecret string) ([]byte, error) {
	if dataKey != "" {
		kek, err := DecodeMasterKey(dataKey)
		if err != nil {
			return nil, fmt.Errorf("DATA_ENCRYPTION_KEY %w", err)
		}
		return kek, nil
	}

	if dataKeyFile != "" {
		raw, err := os.ReadFile(dataKeyFile) // #nosec G304 G703 -- path is an operator-supplied config path (DATA_ENCRYPTION_KEY_FILE), not request input
		if err != nil {
			return nil, fmt.Errorf("DATA_ENCRYPTION_KEY_FILE: %w", err)
		}
		kek, err := DecodeMasterKey(string(raw))
		if err != nil {
			return nil, fmt.Errorf("DATA_ENCRYPTION_KEY_FILE %w", err)
		}
		return kek, nil
	}

	if jwtSecret != "" {
		return deriveKEKFromJWT(jwtSecret), nil
	}

	return nil, nil
}

// EncryptionKey resolves the master key (KEK) directly from the process
// environment (DATA_ENCRYPTION_KEY, DATA_ENCRYPTION_KEY_FILE,
// JWT_SECRET_KEY) via ResolveMasterKey. It exists for the standalone CLI
// tools (cmd/backup, cmd/backupverify, cmd/rotate-at-rest-key,
// cmd/backfill-at-rest, cmd/backfill-unicode-nfc) that operate without
// building a full config.Config. Server code that already holds a validated
// config.Config (main.go, services/restore_drill_service.go) must call
// ResolveMasterKey with its fields instead, so the validated value is the
// used value (issue #938).
func EncryptionKey() ([]byte, error) {
	return ResolveMasterKey(
		os.Getenv("DATA_ENCRYPTION_KEY"),
		os.Getenv("DATA_ENCRYPTION_KEY_FILE"),
		os.Getenv("JWT_SECRET_KEY"),
	)
}

// deriveKEKFromJWT derives the 32-byte master key from the JWT secret via
// HKDF-SHA256 with a dedicated info string (domain separation from
// credential_crypto's own derivation, so the two never share key material).
func deriveKEKFromJWT(jwt string) []byte {
	kek := make([]byte, keySize)
	_, _ = io.ReadFull(hkdf.New(sha256.New, []byte(jwt), nil, []byte(atrestKEKInfo)), kek)
	return kek
}

// backupSigningKeyInfo is the HKDF domain-separation string for the detached
// snapshot-signing key (issue #943). The signing key is derived from the same
// at-rest master key as the KEK, but with its own info string so the same
// secret bytes are never used for both AES-GCM key wrapping and HMAC snapshot
// signatures.
const backupSigningKeyInfo = "mycorrhizal-backup-signing-key-v1"

// DeriveBackupSigningKey derives the HMAC-SHA256 key used to sign backup
// snapshot manifests (issue #943) from the at-rest master key. It is
// domain-separated from the KEK and from credential_crypto's derivations.
// Returns nil for a nil master key, matching EncryptionKey's "no key
// configured" signal.
func DeriveBackupSigningKey(master []byte) []byte {
	if master == nil {
		return nil
	}
	key := make([]byte, keySize)
	_, _ = io.ReadFull(hkdf.New(sha256.New, master, nil, []byte(backupSigningKeyInfo)), key)
	return key
}

// BackupSigningKey resolves the at-rest master key from the environment (via
// EncryptionKey) and derives the snapshot-signing key from it. It returns
// (nil, nil) when no master key is configured — the caller decides whether an
// unsigned backup is acceptable; `make backup` treats it as a hard error
// because an unsigned snapshot is exactly the authenticity gap issue #943
// closes. Deriving from the at-rest key means a deployment that already
// manages DATA_ENCRYPTION_KEY (or the JWT_SECRET_KEY fallback) needs no new
// secret to sign its backups.
func BackupSigningKey() ([]byte, error) {
	master, err := EncryptionKey()
	if err != nil || master == nil {
		return nil, err
	}
	return DeriveBackupSigningKey(master), nil
}

// Initialize loads (or lazily creates) the wrapped DEK under the given
// master key and arms the serializer layer. Calling it again replaces the
// in-memory key material. A wrong KEK fails closed: unwrapping the stored DEK
// is an AES-GCM authentication failure and returns an error — the caller
// should refuse to boot, which is exactly the "wrong key fails closed"
// behavior the issue's verify section requires.
//
// db is used to read/seed the data_encryption_keys table. It may be nil only
// when encryption is not configured (kek == nil), in which case Initialize is
// a no-op and serializers stay pass-through.
func Initialize(db *gorm.DB, kek []byte) error {
	if kek == nil {
		engine.mu.Lock()
		engine.on = false
		engine.dek = nil
		engine.kek = nil
		engine.mu.Unlock()
		return nil
	}
	if db == nil {
		return errors.New("atrest: db is required when a master key is configured")
	}

	dek, err := loadOrCreateDEK(db, kek)
	if err != nil {
		return err
	}

	engine.mu.Lock()
	engine.dek = dek
	engine.kek = append([]byte(nil), kek...)
	engine.on = true
	engine.mu.Unlock()
	return nil
}

// armForTest is the test-only path that arms the engine with a DEK directly,
// bypassing the data_encryption_keys table (which needs a real DB). It keeps
// the pure-crypto tests independent of database.InitDB. The full
// Initialize→persist→load path is exercised by the backfill/serializer tests
// against a real migrated DB.
func armForTest(dek, kek []byte) {
	engine.mu.Lock()
	engine.dek = append([]byte(nil), dek...)
	engine.kek = append([]byte(nil), kek...)
	engine.on = true
	engine.mu.Unlock()
}

// ResetForTest disarms the engine (test cleanup).
func ResetForTest() {
	engine.mu.Lock()
	engine.on = false
	engine.dek = nil
	engine.kek = nil
	engine.mu.Unlock()
}

// loadOrCreateDEK reads the deployment's wrapped DEK and unwraps it with the
// master key; if no row exists yet it generates a fresh 32-byte DEK, wraps it
// under the master key, and persists it. Row-count preservation is the
// backfill test's concern; here we only ever add the single key row.
func loadOrCreateDEK(db *gorm.DB, kek []byte) ([]byte, error) {
	type keyRow struct {
		WrappedDEK []byte `gorm:"column:wrapped_dek"`
	}
	var row keyRow
	// Scan (not First): a missing row is the normal first-boot case and must
	// not log a "record not found" error through GORM's logger.
	err := db.Table("data_encryption_keys").Select("wrapped_dek").Where("key_id = ?", keyID).Limit(1).Scan(&row).Error
	switch {
	case err == nil && len(row.WrappedDEK) > 0:
		dek, uerr := unwrap(kek, row.WrappedDEK)
		if uerr != nil {
			return nil, fmt.Errorf("atrest: failed to unwrap data-encryption key (wrong DATA_ENCRYPTION_KEY? lost key = lost data): %w", uerr)
		}
		return dek, nil
	case err == nil:
		dek := make([]byte, keySize)
		if _, rerr := io.ReadFull(rand.Reader, dek); rerr != nil {
			return nil, fmt.Errorf("atrest: generate DEK: %w", rerr)
		}
		wrapped, werr := wrap(kek, dek)
		if werr != nil {
			return nil, fmt.Errorf("atrest: wrap DEK: %w", werr)
		}
		ins := db.Table("data_encryption_keys").Create(map[string]interface{}{
			"key_id":      keyID,
			"wrapped_dek": wrapped,
			"created_at":  time.Now().UTC(),
		})
		if ins.Error != nil {
			return nil, fmt.Errorf("atrest: persist wrapped DEK: %w", ins.Error)
		}
		return dek, nil
	default:
		return nil, fmt.Errorf("atrest: read data_encryption_keys: %w", err)
	}
}

// KeyMaterialTimes is the age metadata of the persisted wrapped-DEK row: when
// the DEK was first created and when the master key wrapping it was last
// rotated (zero when it never has been). It is read-only metadata for the
// operator-facing key-age check (issue #955); no key material is exposed.
type KeyMaterialTimes struct {
	CreatedAt time.Time
	RotatedAt time.Time
}

// LastChanged returns the later of CreatedAt and RotatedAt — the effective age
// of the key material, since rotating the master key replaces the KEK without
// touching the DEK.
func (k KeyMaterialTimes) LastChanged() time.Time {
	if k.RotatedAt.After(k.CreatedAt) {
		return k.RotatedAt
	}
	return k.CreatedAt
}

// ReadKeyMaterialTimes reads the wrapped-DEK row's timestamps. db must be
// non-nil. ok is false when no wrapped DEK row exists yet (encryption has never
// been armed on this database), which is not an error. A row whose created_at
// is NULL is treated as absent (the migration declares it NOT NULL, so this
// only guards a hand-edited database).
func ReadKeyMaterialTimes(db *gorm.DB) (KeyMaterialTimes, bool, error) {
	if db == nil {
		return KeyMaterialTimes{}, false, errors.New("atrest: key material times require a db handle")
	}
	type row struct {
		CreatedAt *time.Time `gorm:"column:created_at"`
		RotatedAt *time.Time `gorm:"column:rotated_at"`
	}
	var r row
	if err := db.Table("data_encryption_keys").
		Select("created_at, rotated_at").
		Where("key_id = ?", keyID).Limit(1).Scan(&r).Error; err != nil {
		return KeyMaterialTimes{}, false, fmt.Errorf("atrest: read data_encryption_keys timestamps: %w", err)
	}
	if r.CreatedAt == nil {
		return KeyMaterialTimes{}, false, nil
	}
	out := KeyMaterialTimes{CreatedAt: r.CreatedAt.UTC()}
	if r.RotatedAt != nil {
		out.RotatedAt = r.RotatedAt.UTC()
	}
	return out, true, nil
}

// VerifyBackupDecryptable reports whether a (restored) database's wrapped DEK
// unwraps under the given master key, without arming the global engine or
// creating anything. It is the restore-side counterpart to Initialize: a
// restore succeeds only if the snapshot's encrypted columns are actually
// readable under the current key, and a wrong/lost key must fail a restore as
// loudly as it would fail a real boot — not silently pass a row-count
// comparison (issue #420, wired into the restore drill in
// services/restore_drill_service.go).
//
// Semantics mirror Initialize's boot-time behavior:
//
//   - kek == nil (encryption not configured): pass — a pass-through
//     deployment has nothing to verify;
//   - no data_encryption_keys row: pass — a legacy/pre-encryption database has
//     no wrapped DEK, and a real boot with a configured key would create one
//     on first Initialize;
//   - row present: unwrap it under kek; a failure is an error (the master key
//     was rotated or lost since the snapshot was taken).
func VerifyBackupDecryptable(db *gorm.DB, kek []byte) error {
	if kek == nil {
		return nil
	}
	if db == nil {
		return errors.New("atrest: verify requires a db handle")
	}
	type keyRow struct {
		WrappedDEK []byte `gorm:"column:wrapped_dek"`
	}
	var row keyRow
	if err := db.Table("data_encryption_keys").Select("wrapped_dek").Where("key_id = ?", keyID).Limit(1).Scan(&row).Error; err != nil {
		return fmt.Errorf("atrest: read data_encryption_keys for restore verification: %w", err)
	}
	if len(row.WrappedDEK) == 0 {
		return nil
	}
	if _, err := unwrap(kek, row.WrappedDEK); err != nil {
		return fmt.Errorf("atrest: restored database's data-encryption key does not unwrap under the current master key (key rotated or lost since the backup? a real restore would fail closed at boot): %w", err)
	}
	return nil
}

// RotateMasterKey swaps the master key that wraps the deployment DEK:
// unwrap with the old key, rewrap with the new, update the single row. No
// payload bytes are read or rewritten — rotation never requires re-encrypting
// the database, which is the property the issue's envelope recommendation
// exists to provide.
func RotateMasterKey(db *gorm.DB, oldKEK, newKEK []byte) error {
	if db == nil || oldKEK == nil || newKEK == nil {
		return errors.New("atrest: rotate requires db, old and new master key")
	}
	type keyRow struct {
		WrappedDEK []byte `gorm:"column:wrapped_dek"`
	}
	var row keyRow
	if err := db.Table("data_encryption_keys").Select("wrapped_dek").Where("key_id = ?", keyID).Limit(1).Scan(&row).Error; err != nil {
		return fmt.Errorf("atrest: rotate read DEK: %w", err)
	}
	if len(row.WrappedDEK) == 0 {
		return errors.New("atrest: rotate: no wrapped DEK found (has the server ever booted with a key?)")
	}
	dek, err := unwrap(oldKEK, row.WrappedDEK)
	if err != nil {
		return fmt.Errorf("atrest: rotate unwrap with old key failed: %w", err)
	}
	wrapped, err := wrap(newKEK, dek)
	if err != nil {
		return fmt.Errorf("atrest: rotate rewrap: %w", err)
	}
	if err := db.Table("data_encryption_keys").Where("key_id = ?", keyID).
		Updates(map[string]interface{}{
			"wrapped_dek": wrapped,
			"rotated_at":  time.Now().UTC(),
		}).Error; err != nil {
		return fmt.Errorf("atrest: rotate persist: %w", err)
	}
	// Update the in-memory key material so a running server keeps working.
	engine.mu.Lock()
	engine.dek = dek
	engine.kek = append([]byte(nil), newKEK...)
	engine.mu.Unlock()
	return nil
}

// Encrypt seals plaintext under the DEK. The empty string stays empty (so
// NOT NULL columns and empty-string semantics are unchanged). Returns an
// error only when the layer is armed and sealing fails (can't happen for
// GCM with valid key material).
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if !engine.on {
		return plaintext, nil
	}
	ct, err := seal(engine.dek, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return ciphertextPrefix + keyID + ":" + base64.RawURLEncoding.EncodeToString(ct), nil
}

// Decrypt opens a stored value. Values without the ciphertext prefix are
// returned unchanged (legacy plaintext: rows written before encryption, or
// rows in a deployment that never configured a key). Values with the prefix
// are decrypted, and a wrong key / corrupted ciphertext fails closed with an
// error rather than returning garbage.
func Decrypt(stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !strings.HasPrefix(stored, ciphertextPrefix) {
		return stored, nil
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if !engine.on {
		// Encrypted data present but the layer is not armed: fail closed
		// rather than return ciphertext as if it were plaintext.
		return "", errors.New("atrest: encrypted value read while at-rest encryption is not initialized")
	}
	rest := strings.TrimPrefix(stored, ciphertextPrefix)
	kid, payload, ok := strings.Cut(rest, ":")
	if !ok || kid != keyID {
		return "", errors.New("atrest: malformed encrypted value")
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return "", errors.New("atrest: malformed encrypted value")
	}
	pt, err := open(engine.dek, raw)
	if err != nil {
		return "", fmt.Errorf("atrest: decryption failed (wrong key or corrupted data): %w", err)
	}
	return string(pt), nil
}

// seal is AES-256-GCM: fresh random nonce, ciphertext+tag appended.
func seal(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// open reverses seal (nonce-prefixed ciphertext+tag).
func open(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], nil)
}

// wrap seals a DEK under the master key.
func wrap(kek, dek []byte) ([]byte, error) { return seal(kek, dek) }

// unwrap opens a wrapped DEK under the master key.
func unwrap(kek, wrapped []byte) ([]byte, error) { return open(kek, wrapped) }
