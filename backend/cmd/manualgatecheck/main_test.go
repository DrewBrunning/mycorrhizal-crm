package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	commit = "1111111111111111111111111111111111111111"
	gate   = "android-local-mode-device"
	// 2026-10-06 is 5 days after the attestation date below.
	today = "2026-10-06T12:00:00Z"
)

// ledgerJSON builds a ledger with one gate and, when attested, one attestation
// of 2026-10-01 against `commit`.
func ledgerJSON(attested bool, evidence, sha string) string {
	att := ""
	if attested {
		att = fmt.Sprintf(`{"date":"2026-10-01","commit":%q,"attester":"Drew","evidence":%q,"device":"Pixel 8a","abi":"arm64-v8a","tests":1,"skipped":0,"failures":0,"junit_sha256":%q}`, commit, evidence, sha)
	}
	return fmt.Sprintf(`{"gates":[{"id":%q,"name":"Local mode","applies_to":["final","rc"],"max_age_days":14,"watch_paths":["backend/embedded/**"],"require_abi":"arm64-v8a","evidence_marker":"LocalOnlyModeE2eTest","run":"scripts/x.sh --record","attestations":[%s]}]}`, gate, att)
}

const noSHA = "abababababababababababababababababababababababababababababababab"

type fsys map[string]string

func (f fsys) read(p string) ([]byte, error) {
	if s, ok := f[p]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("no such file: " + p)
}

type writes struct {
	path string
	data string
	err  error
}

func (w *writes) write(p string, d []byte, _ os.FileMode) error {
	w.path, w.data = p, string(d)
	return w.err
}

func invoke(t *testing.T, f fsys, w *writes, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var o, e bytes.Buffer
	code = run(args, &o, &e, f.read, w.write)
	return code, o.String(), e.String()
}

func freshFiles() fsys {
	return fsys{
		"l.json": ledgerJSON(true, "https://example.invalid/run", noSHA),
		"f.json": fmt.Sprintf(`{%q:{"ancestor":true,"changed":["docs/a.md"]}}`, commit),
	}
}

func TestUsageAndUnknownSubcommand(t *testing.T) {
	code, _, stderr := invoke(t, fsys{}, &writes{})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "usage:")
	code, _, stderr = invoke(t, fsys{}, &writes{}, "bogus")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, `unknown subcommand "bogus"`)
	code, _, _ = invoke(t, fsys{}, &writes{}, "check", "-nosuchflag")
	assert.Equal(t, 2, code)
}

func TestUnreadableAndInvalidLedger(t *testing.T) {
	code, _, stderr := invoke(t, fsys{}, &writes{}, "validate", "-ledger", "l.json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "read ledger")

	code, stdout, _ := invoke(t, fsys{"l.json": `{"gates":[]}`}, &writes{}, "validate", "-ledger", "l.json")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "::error::manual-gates ledger lists no gates")
}

func TestValidate(t *testing.T) {
	code, stdout, _ := invoke(t, freshFiles(), &writes{}, "validate", "-ledger", "l.json")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "manual-gates ledger OK: 1 gate(s)")
}

func TestValidateVerifiesRetainedEvidenceUnderRoot(t *testing.T) {
	xml := `<testsuite tests="1" skipped="0" failures="0" errors="0"><testcase classname="LocalOnlyModeE2eTest"/></testsuite>`
	sum := sha256.Sum256([]byte(xml))
	ev := ".github/manual-gates-evidence/" + gate + "/e.xml"
	f := fsys{"l.json": ledgerJSON(true, ev, hex.EncodeToString(sum[:])), filepath.Join("root", ev): xml}
	code, stdout, _ := invoke(t, f, &writes{}, "validate", "-ledger", "l.json", "-root", "root")
	assert.Equal(t, 0, code, stdout)

	// edit the retained XML after attesting: the digest no longer matches.
	f[filepath.Join("root", ev)] = xml + " "
	code, stdout, _ = invoke(t, f, &writes{}, "validate", "-ledger", "l.json", "-root", "root")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "does not hash to the recorded junit_sha256")

	// the evidence path resolves against -root, so the wrong root finds nothing.
	code, stdout, _ = invoke(t, f, &writes{}, "validate", "-ledger", "l.json", "-root", "elsewhere")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "does not exist")
}

func TestCommits(t *testing.T) {
	code, stdout, _ := invoke(t, freshFiles(), &writes{}, "commits", "-ledger", "l.json")
	assert.Equal(t, 0, code)
	assert.Equal(t, commit+"\n", stdout)
	code, stdout, _ = invoke(t, fsys{"l.json": ledgerJSON(false, "", "")}, &writes{}, "commits", "-ledger", "l.json")
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
}

func TestCheckAttestedWritesTheReadinessJSON(t *testing.T) {
	w := &writes{}
	code, stdout, _ := invoke(t, freshFiles(), w, "check", "-ledger", "l.json", "-attest", gate, "-kind", "rc",
		"-facts", "f.json", "-now", today, "-actor", "drew", "-out", "gho")
	require.Equal(t, 0, code, stdout)
	assert.Contains(t, stdout, "manual gate `"+gate+"`: attested (ledger: fresh)")
	assert.Equal(t, "gho", w.path)
	require.True(t, strings.HasPrefix(w.data, "manual_gates=[") && strings.HasSuffix(w.data, "]\n"), w.data)
	var recs []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(w.data, "manual_gates=")), &recs))
	require.Len(t, recs, 1)
	assert.Equal(t, "attested", recs[0]["mode"])
	assert.Equal(t, "drew", recs[0]["attested_by"])
	assert.Equal(t, "fresh", recs[0]["ledger_state"])
	assert.Equal(t, commit, recs[0]["attestation"].(map[string]any)["commit"])
	assert.Equal(t, 1, strings.Count(w.data, "\n"), "one line: safe for $GITHUB_OUTPUT")
}

func TestCheckSkipRecordsTheReason(t *testing.T) {
	w := &writes{}
	f := freshFiles()
	f["l.json"] = ledgerJSON(false, "", "")
	code, stdout, _ := invoke(t, f, w, "check", "-ledger", "l.json", "-attest", gate+"=skip:the Pixel is being repaired",
		"-kind", "final", "-now", today, "-actor", "drew", "-out", "gho")
	require.Equal(t, 0, code, stdout)
	assert.Contains(t, stdout, "skipped (reason: the Pixel is being repaired; ledger: no attestation recorded in the ledger)")
	assert.Contains(t, w.data, `"reason":"the Pixel is being repaired"`)
	assert.Contains(t, w.data, `"mode":"skipped"`)
}

func TestCheckFailuresExitOneWithErrorAnnotations(t *testing.T) {
	// missing gate
	w := &writes{}
	code, stdout, _ := invoke(t, freshFiles(), w, "check", "-ledger", "l.json", "-attest", "", "-kind", "final", "-facts", "f.json", "-now", today, "-out", "gho")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "::error::manual gate `"+gate+"`")
	assert.Contains(t, stdout, "is not addressed by attest_manual_gates")
	assert.Empty(t, w.path, "nothing is written on failure")

	// malformed input
	code, stdout, _ = invoke(t, freshFiles(), &writes{}, "check", "-ledger", "l.json", "-attest", gate+"=maybe", "-kind", "final", "-facts", "f.json", "-now", today)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "only `skip:<reason>` is accepted")

	// stale: 30 days later
	code, stdout, _ = invoke(t, freshFiles(), &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-facts", "f.json", "-now", "2026-11-05T00:00:00Z")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "35 days ago (max 14)")

	// a watched path changed since the attested commit
	f := freshFiles()
	f["f.json"] = fmt.Sprintf(`{%q:{"ancestor":true,"changed":["backend/embedded/server.go"]}}`, commit)
	code, stdout, _ = invoke(t, f, &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-facts", "f.json", "-now", today)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "watched paths changed since the attested commit")
	assert.Contains(t, stdout, "backend/embedded/server.go")

	// no facts at all: the attested commit is unknown, so it fails closed
	code, stdout, _ = invoke(t, freshFiles(), &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-now", today)
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, "is not an ancestor of the release commit")
}

func TestCheckInputErrors(t *testing.T) {
	f := freshFiles()
	code, _, stderr := invoke(t, f, &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "beta")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "-kind must be")
	code, _, stderr = invoke(t, f, &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "rc", "-now", "yesterday")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "-now")
	code, _, stderr = invoke(t, f, &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "rc", "-facts", "missing.json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "read facts")
	f["bad.json"] = "{"
	code, _, stderr = invoke(t, f, &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "rc", "-facts", "bad.json")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "facts do not parse")
	w := &writes{err: errors.New("disk full")}
	code, _, stderr = invoke(t, f, w, "check", "-ledger", "l.json", "-attest", gate, "-kind", "rc", "-facts", "f.json", "-now", today, "-out", "gho")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "write output")
}

func TestCheckWithoutOutWritesNothing(t *testing.T) {
	w := &writes{}
	code, _, _ := invoke(t, freshFiles(), w, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-facts", "f.json", "-now", today)
	assert.Equal(t, 0, code)
	assert.Empty(t, w.path)
}

func TestCheckUsesTheClockSeamByDefault(t *testing.T) {
	old := now
	defer func() { now = old }()
	now = func() time.Time { return time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC) }
	code, _, _ := invoke(t, freshFiles(), &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-facts", "f.json")
	assert.Equal(t, 0, code)
	now = func() time.Time { return time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC) }
	code, _, _ = invoke(t, freshFiles(), &writes{}, "check", "-ledger", "l.json", "-attest", gate, "-kind", "final", "-facts", "f.json")
	assert.Equal(t, 1, code, "a stale attestation fails on the real-clock path too")
}

func TestStatus(t *testing.T) {
	code, stdout, _ := invoke(t, freshFiles(), &writes{}, "status", "-ledger", "l.json", "-kind", "final", "-facts", "f.json", "-now", today)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, gate+": fresh")

	code, stdout, _ = invoke(t, freshFiles(), &writes{}, "status", "-ledger", "l.json", "-kind", "final", "-facts", "f.json", "-now", "2026-12-01T00:00:00Z")
	assert.Equal(t, 1, code)
	assert.Contains(t, stdout, gate+": last attested")
	assert.Contains(t, stdout, "::error::")

	code, _, _ = invoke(t, freshFiles(), &writes{}, "status", "-ledger", "l.json", "-kind", "")
	assert.Equal(t, 2, code)
}

func TestAppendFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out")
	require.NoError(t, os.WriteFile(p, []byte("a=1\n"), 0o600))
	require.NoError(t, appendFile(p, []byte("b=2\n"), 0o644))
	require.NoError(t, appendFile(p, []byte("c=3\n"), 0o644))
	b, err := os.ReadFile(p) // #nosec G304 -- test temp file
	require.NoError(t, err)
	assert.Equal(t, "a=1\nb=2\nc=3\n", string(b), "appends; never truncates $GITHUB_OUTPUT")

	assert.Error(t, appendFile(filepath.Join(t.TempDir(), "no", "such", "dir", "f"), []byte("x"), 0o644))
}

func TestAppendFileWriteError(t *testing.T) {
	// /dev/full accepts open() but fails every write with ENOSPC.
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skip("no /dev/full on this platform")
	}
	assert.Error(t, appendFile("/dev/full", []byte("x"), 0o644))
}

func TestReportEmptyIsZero(t *testing.T) {
	var b bytes.Buffer
	assert.Equal(t, 0, report(&b, nil))
	assert.Empty(t, b.String())
	assert.Equal(t, 1, report(&b, []string{"multi\nline"}))
	assert.Equal(t, "::error::multi line\n", b.String(), "a newline cannot smuggle a second workflow command")
}

func TestMainExitsThroughTheSeam(t *testing.T) {
	oldExit, oldArgs := osExit, os.Args
	defer func() { osExit, os.Args = oldExit, oldArgs }()
	var got int
	osExit = func(c int) { got = c }
	os.Args = []string{"manualgatecheck"}
	main()
	assert.Equal(t, 2, got)
}
