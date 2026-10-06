package manualgates

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func junit(tests, skipped, failures, errs int, class string) string {
	return fmt.Sprintf(`<?xml version="1.0"?><testsuite name="Pixel" tests="%d" failures="%d" errors="%d" skipped="%d"><testcase classname="%s" name="x"/></testsuite>`, tests, failures, errs, skipped, class)
}

func TestJUnitCounts(t *testing.T) {
	tests, skipped, failures, err := JUnitCounts([]byte(junit(3, 1, 1, 1, "c")))
	require.NoError(t, err)
	assert.Equal(t, []int{3, 1, 2}, []int{tests, skipped, failures}, "errors count as failures")

	wrapped := `<testsuites><testsuite tests="2" skipped="0" failures="0" errors="0"/><testsuite tests="5" skipped="2" failures="1" errors="0"/></testsuites>`
	tests, skipped, failures, err = JUnitCounts([]byte(wrapped))
	require.NoError(t, err)
	assert.Equal(t, []int{7, 2, 1}, []int{tests, skipped, failures}, "every suite is summed")

	// a non-numeric attribute is ignored, not fatal
	tests, _, _, err = JUnitCounts([]byte(`<testsuite tests="x" skipped="0"/>`))
	require.NoError(t, err)
	assert.Equal(t, 0, tests)

	_, _, _, err = JUnitCounts([]byte(`<nothing/>`))
	assert.ErrorContains(t, err, "no <testsuite>")
	_, _, _, err = JUnitCounts([]byte(`<testsuite`))
	assert.Error(t, err)
	_, _, _, err = JUnitCounts(nil)
	assert.Error(t, err)
}

func evidenceFixture(xml string) (Ledger, func(string) ([]byte, bool)) {
	sum := sha256.Sum256([]byte(xml))
	g := goodGate()
	g.EvidenceMarker = "LocalOnlyModeE2eTest"
	a := goodAttestation()
	a.Evidence = EvidenceDir + gid + "/2026-10-01-1111111.xml"
	a.JUnitSHA256 = hex.EncodeToString(sum[:])
	g.Attestations = []Attestation{a}
	files := map[string][]byte{a.Evidence: []byte(xml)}
	return ledgerOf(g), func(p string) ([]byte, bool) { b, ok := files[p]; return b, ok }
}

func TestVerifyEvidenceAcceptsConsistentEvidence(t *testing.T) {
	l, read := evidenceFixture(junit(1, 0, 0, 0, "com.x.LocalOnlyModeE2eTest"))
	assert.Empty(t, VerifyEvidence(l, read))
}

func TestVerifyEvidenceIgnoresNonRepoEvidence(t *testing.T) {
	// a URL/note is accepted as a note; it is not verifiable and not an error.
	l := ledgerOf(goodGate())
	assert.Empty(t, VerifyEvidence(l, func(string) ([]byte, bool) { t.Fatal("must not read"); return nil, false }))
}

func TestVerifyEvidenceCatchesEachLie(t *testing.T) {
	good := junit(1, 0, 0, 0, "com.x.LocalOnlyModeE2eTest")
	cases := []struct {
		name   string
		mutate func(l *Ledger, files map[string][]byte, path string)
		want   string
	}{
		{"file missing", func(l *Ledger, f map[string][]byte, p string) { delete(f, p) }, "does not exist"},
		{"file edited after attesting", func(l *Ledger, f map[string][]byte, p string) { f[p] = []byte(good + " ") }, "does not hash to the recorded junit_sha256"},
		{"claims fewer skips than the XML reports", func(l *Ledger, f map[string][]byte, p string) {
			x := junit(1, 1, 0, 0, "com.x.LocalOnlyModeE2eTest")
			f[p] = []byte(x)
			sum := sha256.Sum256([]byte(x))
			l.Gates[0].Attestations[0].JUnitSHA256 = hex.EncodeToString(sum[:])
		}, "reports tests=1 skipped=1 failures=0 but the attestation claims tests=1 skipped=0 failures=0"},
		{"XML is for some other test", func(l *Ledger, f map[string][]byte, p string) {
			x := junit(1, 0, 0, 0, "com.x.SomethingElse")
			f[p] = []byte(x)
			sum := sha256.Sum256([]byte(x))
			l.Gates[0].Attestations[0].JUnitSHA256 = hex.EncodeToString(sum[:])
		}, "never mentions LocalOnlyModeE2eTest"},
		{"not JUnit at all", func(l *Ledger, f map[string][]byte, p string) {
			x := "LocalOnlyModeE2eTest passed, trust me"
			f[p] = []byte(x)
			sum := sha256.Sum256([]byte(x))
			l.Gates[0].Attestations[0].JUnitSHA256 = hex.EncodeToString(sum[:])
		}, "is not a JUnit XML report"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l, _ := evidenceFixture(good)
			p := l.Gates[0].Attestations[0].Evidence
			files := map[string][]byte{p: []byte(good)}
			c.mutate(&l, files, p)
			f := VerifyEvidence(l, func(path string) ([]byte, bool) { b, ok := files[path]; return b, ok })
			require.NotEmpty(t, f)
			assert.Contains(t, strings.Join(f, "\n"), c.want)
		})
	}
}
