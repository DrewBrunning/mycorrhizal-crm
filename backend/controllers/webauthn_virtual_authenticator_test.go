package controllers

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

// virtualAuthenticator is a minimal software WebAuthn authenticator (ES256,
// "none" attestation) used to drive the real ceremony endpoints end to end —
// the go-webauthn library ships no virtual authenticator. It signs exactly what
// a browser + platform authenticator would, so the server verifies real
// signatures, real rpIdHash and real client data rather than mocks.
type virtualAuthenticator struct {
	t            *testing.T
	origin       string
	rpID         string
	key          *ecdsa.PrivateKey
	credentialID []byte
	signCount    uint32
	backupFlags  byte // 0x08 BE, 0x10 BS
}

func newVirtualAuthenticator(t *testing.T, origin, rpID string) *virtualAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	id := make([]byte, 32)
	_, err = rand.Read(id)
	require.NoError(t, err)
	return &virtualAuthenticator{t: t, origin: origin, rpID: rpID, key: key, credentialID: id}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (v *virtualAuthenticator) clientData(typ, challenge string) []byte {
	b, err := json.Marshal(map[string]any{
		"type":        typ,
		"challenge":   challenge,
		"origin":      v.origin,
		"crossOrigin": false,
	})
	require.NoError(v.t, err)
	return b
}

// challengeFrom extracts publicKey.challenge from a begin response body.
func challengeFrom(t *testing.T, body []byte) string {
	t.Helper()
	var opts struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	require.NoError(t, json.Unmarshal(body, &opts))
	require.NotEmpty(t, opts.PublicKey.Challenge)
	return opts.PublicKey.Challenge
}

func (v *virtualAuthenticator) rpIDHash() []byte {
	h := sha256.Sum256([]byte(v.rpID))
	return h[:]
}

// attestation answers navigator.credentials.create for the given challenge.
func (v *virtualAuthenticator) attestation(challenge string) map[string]any {
	xb := v.key.PublicKey.X.FillBytes(make([]byte, 32))
	yb := v.key.PublicKey.Y.FillBytes(make([]byte, 32))
	coseKey, err := cbor.Marshal(map[int]any{1: 2, 3: -7, -1: 1, -2: xb, -3: yb})
	require.NoError(v.t, err)

	var auth bytes.Buffer
	auth.Write(v.rpIDHash())
	auth.WriteByte(0x01 | 0x04 | 0x40 | v.backupFlags) // UP | UV | AT | BE/BS
	_ = binary.Write(&auth, binary.BigEndian, v.signCount)
	auth.Write(make([]byte, 16)) // AAGUID
	_ = binary.Write(&auth, binary.BigEndian, uint16(len(v.credentialID)))
	auth.Write(v.credentialID)
	auth.Write(coseKey)

	attObj, err := cbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": auth.Bytes()})
	require.NoError(v.t, err)

	return map[string]any{
		"id":    b64(v.credentialID),
		"rawId": b64(v.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"attestationObject": b64(attObj),
			"clientDataJSON":    b64(v.clientData("webauthn.create", challenge)),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": map[string]any{},
	}
}

// assertion answers navigator.credentials.get for the given challenge, bumping
// the signature counter first (as a real authenticator does).
func (v *virtualAuthenticator) assertion(challenge string) map[string]any {
	v.signCount++
	return v.assertionWithCount(challenge, v.signCount)
}

func (v *virtualAuthenticator) assertionWithCount(challenge string, count uint32) map[string]any {
	var auth bytes.Buffer
	auth.Write(v.rpIDHash())
	auth.WriteByte(0x01 | 0x04 | v.backupFlags) // UP | UV
	_ = binary.Write(&auth, binary.BigEndian, count)

	cd := v.clientData("webauthn.get", challenge)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, auth.Bytes()...), cdHash[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, v.key, digest[:])
	require.NoError(v.t, err)

	return map[string]any{
		"id":    b64(v.credentialID),
		"rawId": b64(v.credentialID),
		"type":  "public-key",
		"response": map[string]any{
			"authenticatorData": b64(auth.Bytes()),
			"clientDataJSON":    b64(cd),
			"signature":         b64(sig),
		},
		"clientExtensionResults": map[string]any{},
	}
}
