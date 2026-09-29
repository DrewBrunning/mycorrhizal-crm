// Package androidpasskey holds the server side of native Android passkeys
// (ADR 0034, issue #1293): the effective-state decision, the fingerprint
// registry, the Digital Asset Links document served at
// /.well-known/assetlinks.json, and the derivation of the
// android:apk-key-hash:... ceremony origins go-webauthn accepts as
// RPOpaqueOrigins.
//
// It deliberately imports nothing from this module so config (which reports
// the /health capability token) and services (which builds the relying party)
// can both depend on it without a cycle.
package androidpasskey

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
)

// PackageName is the Android applicationId the assetlinks statement targets.
// Every release flavor (obtainium, play, foss) and the debug/benchmark build
// types keep the same id: android/app/build.gradle.kts sets no applicationId
// and no applicationIdSuffix, so it defaults to the namespace
// (docs/adrs/0022-distribution-variants.md: all flavors deliberately keep
// applicationId = com.mycorrhizal.crm).
const PackageName = "com.mycorrhizal.crm"

// Relation is the Digital Asset Links relation Credential Manager checks for
// passkeys shared between an app and its domain.
const Relation = "delegate_permission/common.get_login_creds"

// OriginPrefix precedes the base64url signing-certificate hash in the
// clientDataJSON origin a native Credential Manager ceremony reports.
const OriginPrefix = "android:apk-key-hash:"

// Fingerprint is a raw SHA-256 signing-certificate digest.
type Fingerprint [32]byte

// Colon renders the fingerprint the way assetlinks.json expects it:
// uppercase hex pairs separated by colons.
func (f Fingerprint) Colon() string {
	h := strings.ToUpper(hex.EncodeToString(f[:]))
	parts := make([]string, 0, 32)
	for i := 0; i < len(h); i += 2 {
		parts = append(parts, h[i:i+2])
	}
	return strings.Join(parts, ":")
}

// Origin is the ceremony origin for this signing certificate:
// android:apk-key-hash: + base64url (no padding) of the raw digest bytes.
func (f Fingerprint) Origin() string {
	return OriginPrefix + base64.RawURLEncoding.EncodeToString(f[:])
}

// ParseFingerprint accepts a SHA-256 fingerprint as 64 hex characters or 32
// colon-separated hex pairs, in either case, and normalises it.
func ParseFingerprint(s string) (Fingerprint, error) {
	var f Fingerprint
	raw := strings.TrimSpace(s)
	if raw == "" {
		return f, errors.New("empty fingerprint")
	}
	compact := raw
	if strings.Contains(raw, ":") {
		parts := strings.Split(raw, ":")
		if len(parts) != 32 {
			return f, fmt.Errorf("fingerprint %q: want 32 colon-separated hex pairs, got %d", s, len(parts))
		}
		for _, p := range parts {
			if len(p) != 2 {
				return f, fmt.Errorf("fingerprint %q: every colon-separated group must be exactly 2 hex digits", s)
			}
		}
		compact = strings.Join(parts, "")
	}
	if len(compact) != 64 {
		return f, fmt.Errorf("fingerprint %q: a SHA-256 fingerprint is 64 hex digits, got %d", s, len(compact))
	}
	b, err := hex.DecodeString(compact)
	if err != nil {
		return f, fmt.Errorf("fingerprint %q: not valid hex", s)
	}
	copy(f[:], b)
	return f, nil
}

// ParseFingerprints parses a list, skipping blank entries (a trailing comma),
// removing duplicates and returning the result sorted by value so every output
// derived from it is deterministic.
func ParseFingerprints(in []string) ([]Fingerprint, error) {
	seen := map[Fingerprint]bool{}
	var out []Fingerprint
	for _, s := range in {
		if strings.TrimSpace(s) == "" {
			continue
		}
		f, err := ParseFingerprint(s)
		if err != nil {
			return nil, err
		}
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	sortFingerprints(out)
	return out, nil
}

func sortFingerprints(fs []Fingerprint) {
	sort.Slice(fs, func(i, j int) bool { return string(fs[i][:]) < string(fs[j][:]) })
}

// unreachableSuffixes is a small, deliberately conservative denylist of names
// that can never be verified by Google over the public internet (RFC 6761 /
// RFC 8375 special-use names plus the common private-network conventions). It
// is a best-effort screen for obvious misconfiguration, not a reachability
// proof.
var unreachableSuffixes = []string{
	".local", ".localhost", ".lan", ".internal", ".home.arpa",
	".test", ".example", ".invalid",
}

// ValidatePublicHTTPSOrigin returns nil when frontendURL is an https origin
// whose host is a plausible public domain name on the default port, or an
// error naming the first rule it fails.
func ValidatePublicHTTPSOrigin(frontendURL string) error {
	raw := strings.TrimSpace(frontendURL)
	if raw == "" || raw == "*" {
		return errors.New("FRONTEND_URL is not a concrete origin (empty or '*')")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return errors.New("FRONTEND_URL is not a valid URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("FRONTEND_URL must be https:// (got %s://)", u.Scheme)
	}
	if p := u.Port(); p != "" && p != "443" {
		return fmt.Errorf("FRONTEND_URL uses port %s; Google fetches assetlinks.json on the default HTTPS port", p)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if net.ParseIP(host) != nil {
		return errors.New("FRONTEND_URL host is an IP address; a passkey RP ID must be a domain name")
	}
	if host == "localhost" {
		return errors.New("FRONTEND_URL host is localhost")
	}
	if !strings.Contains(host, ".") {
		return fmt.Errorf("FRONTEND_URL host %q is a single-label name, not a public domain", host)
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" {
			return fmt.Errorf("FRONTEND_URL host %q has an empty label", host)
		}
	}
	for _, suf := range unreachableSuffixes {
		if strings.HasSuffix(host, suf) {
			return fmt.Errorf("FRONTEND_URL host %q ends in %s, which is not publicly resolvable", host, suf)
		}
	}
	return nil
}

// State is the effective-state decision: whether native Android passkeys are
// on for this instance, why not if they are off, and the trusted fingerprints.
type State struct {
	Effective    bool
	Reason       string // set when !Effective
	Fingerprints []Fingerprint
}

// channel is one release channel's signing certificate.
type channel struct {
	name        string
	fingerprint string // SHA-256, colon-separated; empty until verified
}

// channels is the ONE reviewed list of the project's own release-channel
// signing certificates (ADR 0034 Decision 3). Values must come from real
// signing material and are never guessed or generated; an empty entry is a
// channel still pending (tracked in issue #1334).
var channels = []channel{
	// Obtainium (GitHub Releases), project release keystore. Verified
	// 2026-09-29 (issue #1334) with apksigner on the v1.2.0
	// app-obtainium-release.apk (APK SHA-256
	// ee6df98b340927d7a8ce7e938579939540a69417673535b4d57ba4b278b17829) and
	// keytool on the maintainer's release keystore.
	{"obtainium", "24:CF:16:6F:59:36:A6:AD:05:B4:4B:6B:56:9D:71:17:7A:03:8D:4A:06:0F:A4:85:E1:50:2E:73:29:EF:06:5E"},
	// Google Play: the *app signing key certificate* from Play Console >
	// Setup > App signing (not the upload key). PENDING: issue #1201 / #1334.
	{"play", ""},
	// F-Droid: the key F-Droid signs com.mycorrhizal.crm with, from the app's
	// F-Droid index. PENDING: issue #1199 / #1334.
	{"foss", ""},
}

// builtinFingerprints returns the fingerprints of every channel that has one.
// A malformed built-in value is a programming error caught by
// TestBuiltinChannelsAreWellFormed, so it is skipped here rather than crashing
// a server.
func builtinFingerprints() []Fingerprint {
	out := []Fingerprint{}
	for _, c := range channels {
		if c.fingerprint == "" {
			continue
		}
		if f, err := ParseFingerprint(c.fingerprint); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// Builtin returns the built-in project fingerprints (currently the obtainium
// channel only; play and foss are pending).
func Builtin() []Fingerprint { return builtinFingerprints() }

// Resolve computes the effective state. It is a pure function of its inputs so
// it can be evaluated once at startup and again wherever the answer is needed.
func Resolve(frontendURL string, enabled bool, operatorFingerprints []string) State {
	return resolve(builtinFingerprints(), frontendURL, enabled, operatorFingerprints)
}

func resolve(builtin []Fingerprint, frontendURL string, enabled bool, operator []string) State {
	if !enabled {
		return State{Reason: "WEBAUTHN_ANDROID_ENABLED is not set"}
	}
	if err := ValidatePublicHTTPSOrigin(frontendURL); err != nil {
		return State{Reason: err.Error()}
	}
	extra, err := ParseFingerprints(operator)
	if err != nil {
		return State{Reason: "WEBAUTHN_ANDROID_CERT_SHA256: " + err.Error()}
	}
	seen := map[Fingerprint]bool{}
	var all []Fingerprint
	for _, f := range append(append([]Fingerprint{}, builtin...), extra...) {
		if !seen[f] {
			seen[f] = true
			all = append(all, f)
		}
	}
	if len(all) == 0 {
		return State{Reason: "no signing-certificate fingerprints are known (set WEBAUTHN_ANDROID_CERT_SHA256)"}
	}
	sortFingerprints(all)
	return State{Effective: true, Fingerprints: all}
}

// Origins returns the android:apk-key-hash: origins for the fingerprints, or
// nil when the feature is not effective.
func (s State) Origins() []string {
	if !s.Effective {
		return nil
	}
	out := make([]string, 0, len(s.Fingerprints))
	for _, f := range s.Fingerprints {
		out = append(out, f.Origin())
	}
	return out
}

type assetTarget struct {
	Namespace    string   `json:"namespace"`
	PackageName  string   `json:"package_name"`
	Fingerprints []string `json:"sha256_cert_fingerprints"`
}

type assetStatement struct {
	Relation []string    `json:"relation"`
	Target   assetTarget `json:"target"`
}

// AssetLinksJSON renders the Digital Asset Links document, or nil when the
// feature is not effective (the route then answers 404, never a placeholder).
func (s State) AssetLinksJSON() []byte {
	if !s.Effective {
		return nil
	}
	fps := make([]string, 0, len(s.Fingerprints))
	for _, f := range s.Fingerprints {
		fps = append(fps, f.Colon())
	}
	// Marshalling plain strings cannot fail.
	b, _ := json.Marshal([]assetStatement{{
		Relation: []string{Relation},
		Target:   assetTarget{Namespace: "android_app", PackageName: PackageName, Fingerprints: fps},
	}})
	return b
}
