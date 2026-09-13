package database

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// Detached snapshot authentication (issue #943).
//
// A database snapshot is verified with PRAGMA integrity_check when it is
// written, but that says only that the bytes are a structurally valid SQLite
// database — not that they are *our* database, unmodified since we wrote them.
// An attacker with write access to the backup store (a distinct trust boundary
// from the host, docs/security/threat-model.md) can otherwise substitute a
// tampered or older snapshot and every consumer trusts it.
//
// `make backup` therefore writes a detached manifest beside each snapshot:
//
//	mycorrhizal-20260809-120000.db
//	mycorrhizal-20260809-120000.db.manifest.json
//
// The manifest records the snapshot's size and SHA-256, and an HMAC-SHA256 over
// those fields keyed by the at-rest master key derived with domain separation
// (atrest.BackupSigningKey). Verifying requires the key, which never travels in
// the backup, so a store-only attacker cannot forge a manifest for a
// substituted file or silently strip the manifest — a missing manifest is
// treated as a failure by `make backup-verify` unless the operator explicitly
// opts into reconciling a legacy unsigned set.
//
// Scope and limits, stated plainly:
//
//   - It authenticates the *database* piece only. The photo/attachment
//     directories are outside the tool's boundary (BACKUP-02, issue #454) and
//     remain unsigned; their integrity is operator-owned, like their retention.
//   - It does not stop replay of an older snapshot that is itself validly
//     signed (a rollback to a previously-good backup). The freshness alert
//     (services.backupStaleCondition) is the signal that new backups have
//     stopped, and the operator's off-host/immutable storage (issue #505) is
//     the control for which snapshots exist to replay.
//   - It does not encrypt anything; confidentiality is inherited from the
//     database's field-level at-rest encryption (issue #420).

// backupManifestSuffix is appended to the snapshot path. It is deliberately a
// distinct file rather than a header inside the snapshot: mutating the SQLite
// file to add one would break VACUUM INTO's output and the integrity guarantee.
const backupManifestSuffix = ".manifest.json"

// Manifest format constants. Bump the version on any change to the signed
// fields or the signing input; verification rejects an unknown version rather
// than guessing.
const (
	backupManifestVersion    = 1
	backupSignatureAlgorithm = "hmac-sha256"
	// backupSigningInputPrefix domain-separates the HMAC from any other use of
	// the signing key.
	backupSigningInputPrefix = "mycorrhizal-backup-signature-v1"
)

// BackupManifest is the detached, signed description of one snapshot.
type BackupManifest struct {
	Version   int    `json:"version"`
	Algorithm string `json:"algorithm"`
	CreatedAt string `json:"created_at"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	HMAC      string `json:"hmac"`
}

// signingInput is the exact byte string the HMAC covers. Every field except
// the HMAC itself is included, so tampering with any of them invalidates the
// signature.
func (m BackupManifest) signingInput() []byte {
	return []byte(fmt.Sprintf("%s\n%d\n%s\n%s\n%d\n%s\n",
		backupSigningInputPrefix, m.Version, m.Algorithm, m.CreatedAt, m.SizeBytes, m.SHA256))
}

// ManifestPath returns the path of the manifest that authenticates the
// snapshot at snapshotPath.
func ManifestPath(snapshotPath string) string {
	return snapshotPath + backupManifestSuffix
}

// SignBackup computes and writes the detached manifest for an existing
// snapshot. It is write-new-only, mirroring database.BackupSnapshot: an
// existing manifest is refused, never overwritten. The signing key comes from
// atrest.BackupSigningKey (derived from the at-rest master key) and is never
// stored in the backup.
//
// On a write error a partial manifest may remain; VerifyBackupSignature rejects
// it (the HMAC or digest will not match), and the operator picks a fresh output
// path. The snapshot itself is never modified.
func SignBackup(snapshotPath string, signingKey []byte) error {
	if len(signingKey) == 0 {
		return errors.New("sign backup: signing key is empty")
	}
	fi, err := os.Stat(snapshotPath)
	if err != nil {
		return fmt.Errorf("sign backup: stat snapshot %q: %w", snapshotPath, err)
	}
	digest, err := sha256OfFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("sign backup: %w", err)
	}

	m := BackupManifest{
		Version:   backupManifestVersion,
		Algorithm: backupSignatureAlgorithm,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		SizeBytes: fi.Size(),
		SHA256:    digest,
	}
	m.HMAC = base64.RawURLEncoding.EncodeToString(signManifest(signingKey, m))

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil { // # pragma: no cover -- a struct of scalars cannot fail to marshal
		return fmt.Errorf("sign backup: encode manifest: %w", err)
	}
	data = append(data, '\n')

	manifestPath := ManifestPath(snapshotPath)
	f, err := os.OpenFile(manifestPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("sign backup: manifest %q already exists; refusing to overwrite", manifestPath)
		}
		return fmt.Errorf("sign backup: create manifest %q: %w", manifestPath, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("sign backup: write manifest %q: %w", manifestPath, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("sign backup: close manifest %q: %w", manifestPath, err)
	}
	return nil
}

// ReadBackupManifest loads and parses the manifest beside a snapshot. A missing
// manifest is a distinct, assertable error so callers can tell "unsigned" from
// "tampered".
func ReadBackupManifest(snapshotPath string) (BackupManifest, error) {
	var m BackupManifest
	data, err := os.ReadFile(ManifestPath(snapshotPath)) // #nosec G304 -- path is derived from an operator-supplied snapshot path, not request input
	if err != nil {
		if os.IsNotExist(err) {
			return m, fmt.Errorf("no manifest at %q: the snapshot is unsigned", ManifestPath(snapshotPath))
		}
		return m, fmt.Errorf("read manifest %q: %w", ManifestPath(snapshotPath), err)
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return m, fmt.Errorf("parse manifest %q: %w", ManifestPath(snapshotPath), err)
	}
	return m, nil
}

// VerifyBackupSignature authenticates a snapshot against its manifest with the
// given signing key. It verifies the manifest's HMAC first (so its fields are
// trusted before use), then checks the snapshot's size and SHA-256 against the
// authenticated values.
func VerifyBackupSignature(snapshotPath string, signingKey []byte) error {
	if len(signingKey) == 0 {
		return errors.New("verify backup signature: signing key is empty")
	}
	m, err := ReadBackupManifest(snapshotPath)
	if err != nil {
		return err
	}
	if m.Version != backupManifestVersion {
		return fmt.Errorf("verify backup signature: unsupported manifest version %d (this build understands %d)", m.Version, backupManifestVersion)
	}
	if m.Algorithm != backupSignatureAlgorithm {
		return fmt.Errorf("verify backup signature: unsupported algorithm %q (this build understands %q)", m.Algorithm, backupSignatureAlgorithm)
	}

	expectedMAC := signManifest(signingKey, m)
	gotMAC, err := base64.RawURLEncoding.DecodeString(m.HMAC)
	if err != nil {
		return fmt.Errorf("verify backup signature: malformed signature in manifest %q", ManifestPath(snapshotPath))
	}
	if !hmac.Equal(expectedMAC, gotMAC) {
		return fmt.Errorf("verify backup signature: manifest %q is not authentic (wrong key, or tampered manifest)", ManifestPath(snapshotPath))
	}

	fi, err := os.Stat(snapshotPath)
	if err != nil {
		return fmt.Errorf("verify backup signature: stat snapshot %q: %w", snapshotPath, err)
	}
	if fi.Size() != m.SizeBytes {
		return fmt.Errorf("verify backup signature: snapshot %q is %d bytes, manifest says %d (tampered or substituted)", snapshotPath, fi.Size(), m.SizeBytes)
	}
	digest, err := sha256OfFile(snapshotPath)
	if err != nil {
		return fmt.Errorf("verify backup signature: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(digest), []byte(m.SHA256)) != 1 {
		return fmt.Errorf("verify backup signature: snapshot %q does not match the digest in its manifest (tampered or substituted)", snapshotPath)
	}
	return nil
}

// signManifest computes the raw HMAC-SHA256 over the manifest's signed fields.
func signManifest(signingKey []byte, m BackupManifest) []byte {
	mac := hmac.New(sha256.New, signingKey)
	mac.Write(m.signingInput())
	return mac.Sum(nil)
}

// sha256OfFile returns the lowercase hex SHA-256 of a file's contents.
func sha256OfFile(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path is an operator-supplied snapshot path, not request input
	if err != nil {
		return "", fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %q: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
