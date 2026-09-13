package database_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"mycorrhizal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSigningKey is a fixed 32-byte HMAC key so the sign/verify tests do not
// depend on the environment.
var testSigningKey = []byte("0123456789abcdef0123456789abcdef")

func writeStandaloneFile(t *testing.T, name string, content []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, content, 0o640))
	return p
}

func readManifestJSON(t *testing.T, snapshotPath string) database.BackupManifest {
	t.Helper()
	m, err := database.ReadBackupManifest(snapshotPath)
	require.NoError(t, err)
	return m
}

func TestSignBackupThenVerify(t *testing.T) {
	t.Parallel()
	content := []byte("pretend-this-is-a-sqlite-snapshot")
	p := writeStandaloneFile(t, "snap.db", content)

	require.NoError(t, database.SignBackup(p, testSigningKey))
	require.NoError(t, database.VerifyBackupSignature(p, testSigningKey))

	assert.Equal(t, p+".manifest.json", database.ManifestPath(p))
	m := readManifestJSON(t, p)
	assert.Equal(t, 1, m.Version)
	assert.Equal(t, "hmac-sha256", m.Algorithm)
	assert.Equal(t, int64(len(content)), m.SizeBytes)
	assert.Len(t, m.SHA256, 64)
	assert.NotEmpty(t, m.HMAC)
	assert.NotEmpty(t, m.CreatedAt)

	fi, err := os.Stat(database.ManifestPath(p))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm(), "the manifest must not be world-readable")
}

// TestSignBackupLeavesSnapshotByteIdentical pins that signing is a detached,
// write-new-only operation: the authenticated snapshot itself is untouched.
func TestSignBackupLeavesSnapshotByteIdentical(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("immutable snapshot bytes"))

	before := sha256.Sum256(mustRead(t, p))
	require.NoError(t, database.SignBackup(p, testSigningKey))
	after := sha256.Sum256(mustRead(t, p))

	assert.Equal(t, hex.EncodeToString(before[:]), hex.EncodeToString(after[:]))
}

func TestSignBackupRefusesToOverwriteManifest(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))
	require.NoError(t, database.SignBackup(p, testSigningKey))
	first := mustRead(t, database.ManifestPath(p))

	err := database.SignBackup(p, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to overwrite")
	assert.Equal(t, first, mustRead(t, database.ManifestPath(p)), "a refused sign must not alter the manifest")
}

func TestVerifyBackupSignatureRejectsTamperedSnapshot(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))
	require.NoError(t, database.SignBackup(p, testSigningKey))

	require.NoError(t, os.WriteFile(p, []byte("snapshot-plus-one-tampered-byte"), 0o640))
	err := database.VerifyBackupSignature(p, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tampered or substituted")
}

func TestVerifyBackupSignatureRejectsTamperedManifestField(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))
	require.NoError(t, database.SignBackup(p, testSigningKey))

	m := readManifestJSON(t, p)
	m.SizeBytes++ // forge a plausible-looking manifest
	writeManifest(t, p, m)

	err := database.VerifyBackupSignature(p, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authentic")
}

// TestVerifyBackupSignatureRejectsSwappedManifest covers substitution: a
// manifest that is itself validly signed under the key, but for a different
// snapshot, must not authenticate this one.
func TestVerifyBackupSignatureRejectsSwappedManifest(t *testing.T) {
	t.Parallel()
	a := writeStandaloneFile(t, "a.db", []byte("backup A"))
	b := writeStandaloneFile(t, "b.db", []byte("backup B"))
	require.NoError(t, database.SignBackup(a, testSigningKey))
	require.NoError(t, database.SignBackup(b, testSigningKey))

	// An attacker with store write access swaps A's snapshot for B's, leaving
	// A's manifest in place.
	require.NoError(t, os.WriteFile(a, mustRead(t, b), 0o640))

	err := database.VerifyBackupSignature(a, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tampered or substituted")
}

func TestVerifyBackupSignatureRejectsWrongKey(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))
	require.NoError(t, database.SignBackup(p, testSigningKey))

	err := database.VerifyBackupSignature(p, []byte("a-completely-different-key-000000"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authentic")
}

func TestVerifyBackupSignatureRequiresManifest(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))

	_, err := database.ReadBackupManifest(p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsigned")

	err = database.VerifyBackupSignature(p, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsigned")
}

func TestVerifyBackupSignatureRejectsMalformedManifest(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))
	require.NoError(t, os.WriteFile(database.ManifestPath(p), []byte("{not json"), 0o600))

	err := database.VerifyBackupSignature(p, testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse manifest")
}

func TestVerifyBackupSignatureRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))

	for _, tc := range []struct {
		name string
		m    database.BackupManifest
		want string
	}{
		{"version", database.BackupManifest{Version: 99, Algorithm: "hmac-sha256"}, "unsupported manifest version"},
		{"algorithm", database.BackupManifest{Version: 1, Algorithm: "md5"}, "unsupported algorithm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeManifest(t, p, tc.m)
			err := database.VerifyBackupSignature(p, testSigningKey)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestSignAndVerifyBackupSignatureEmptyKey(t *testing.T) {
	t.Parallel()
	p := writeStandaloneFile(t, "snap.db", []byte("snapshot"))

	require.Error(t, database.SignBackup(p, nil))
	require.Error(t, database.SignBackup(p, []byte{}))
	require.Error(t, database.VerifyBackupSignature(p, nil))
}

func TestSignBackupMissingSnapshotErrors(t *testing.T) {
	t.Parallel()
	err := database.SignBackup(filepath.Join(t.TempDir(), "nope.db"), testSigningKey)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat snapshot")
}

// writeManifest rewrites the manifest beside p, bypassing SignBackup so the
// tests can forge fields.
func writeManifest(t *testing.T, snapshotPath string, m database.BackupManifest) {
	t.Helper()
	data, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(database.ManifestPath(snapshotPath), data, 0o600))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return data
}
