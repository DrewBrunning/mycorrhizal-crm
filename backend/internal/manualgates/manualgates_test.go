package manualgates

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sha1c = "1111111111111111111111111111111111111111"
	sha2c = "2222222222222222222222222222222222222222"
	hash  = "abababababababababababababababababababababababababababababababab"
	gid   = "android-local-mode-device"
)

var clock = time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)

func goodAttestation() Attestation {
	return Attestation{
		Date: "2026-10-01", Commit: sha1c, Attester: "Drew", Evidence: "https://example.invalid/run/1",
		Device: "Pixel 8a", ABI: "arm64-v8a", Tests: 1, JUnitSHA256: hash,
	}
}

func goodGate() Gate {
	return Gate{
		ID: gid, Name: "Android local mode", AppliesTo: []string{KindFinal, KindRC}, MaxAgeDays: 14,
		WatchPaths: []string{"backend/embedded/**", "backend/main.go", "android/core/data/**/local/*.kt"},
		RequireABI: "arm64-v8a", Run: "scripts/android-local-mode-device-test.sh --record",
		Attestations: []Attestation{goodAttestation()},
	}
}

func ledgerOf(g ...Gate) Ledger { return Ledger{Gates: g} }

func marshal(t *testing.T, l Ledger) []byte {
	t.Helper()
	b, err := json.Marshal(l)
	require.NoError(t, err)
	return b
}

func TestParseLedgerAcceptsAGoodLedger(t *testing.T) {
	l, findings := ParseLedger(marshal(t, ledgerOf(goodGate())))
	assert.Empty(t, findings)
	assert.Len(t, l.Gates, 1)
}

func TestParseLedgerRejectsBadJSONAndEmpty(t *testing.T) {
	_, f := ParseLedger([]byte("{nope"))
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "does not parse")
	_, f = ParseLedger([]byte(`{"gates":[]}`))
	assert.Contains(t, strings.Join(f, "\n"), "lists no gates")
}

// Each case breaks one structural rule of the ledger; the finding must name it.
func TestParseLedgerFindsEachStructuralProblem(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(g *Gate)
		want   string
	}{
		{"bad id", func(g *Gate) { g.ID = "Bad_ID" }, "id must be lowercase"},
		{"empty id", func(g *Gate) { g.ID = "" }, "id must be lowercase"},
		{"empty name", func(g *Gate) { g.Name = " " }, "name is empty"},
		{"empty run", func(g *Gate) { g.Run = "" }, "run (how to discharge it) is empty"},
		{"zero max age", func(g *Gate) { g.MaxAgeDays = 0 }, "max_age_days must be positive"},
		{"no applies_to", func(g *Gate) { g.AppliesTo = nil }, "applies_to is empty"},
		{"unknown kind", func(g *Gate) { g.AppliesTo = []string{"beta"} }, `unknown kind "beta"`},
		{"no watch paths", func(g *Gate) { g.WatchPaths = nil }, "watch_paths is empty"},
		{"absolute watch path", func(g *Gate) { g.WatchPaths = []string{"/etc/**"} }, "non-empty repo-relative glob"},
		{"blank watch path", func(g *Gate) { g.WatchPaths = []string{" "} }, "non-empty repo-relative glob"},
		{"invalid glob", func(g *Gate) { g.WatchPaths = []string{"a/[/b"} }, "not a valid glob"},
		{"bad date", func(g *Gate) { g.Attestations[0].Date = "10/01/2026" }, "must be YYYY-MM-DD"},
		{"short commit", func(g *Gate) { g.Attestations[0].Commit = "abc123" }, "full 40-character"},
		{"no attester", func(g *Gate) { g.Attestations[0].Attester = "" }, "attester is empty"},
		{"no evidence", func(g *Gate) { g.Attestations[0].Evidence = "" }, "evidence is empty"},
		{"no device", func(g *Gate) { g.Attestations[0].Device = "" }, "device is empty"},
		{"zero tests", func(g *Gate) { g.Attestations[0].Tests = 0 }, "tests must be at least 1"},
		{"skipped test", func(g *Gate) { g.Attestations[0].Skipped = 1 }, "a skipped test is not evidence"},
		{"failed test", func(g *Gate) { g.Attestations[0].Failures = 2 }, "the run did not pass"},
		{"bad digest", func(g *Gate) { g.Attestations[0].JUnitSHA256 = "xyz" }, "junit_sha256 must be"},
		{"wrong abi", func(g *Gate) { g.Attestations[0].ABI = "x86_64" }, `the gate requires "arm64-v8a"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := goodGate()
			g.Attestations = []Attestation{goodAttestation()}
			c.mutate(&g)
			_, findings := ParseLedger(marshal(t, ledgerOf(g)))
			assert.Contains(t, strings.Join(findings, "\n"), c.want)
		})
	}
}

func TestParseLedgerRejectsDuplicateIDs(t *testing.T) {
	_, f := ParseLedger(marshal(t, ledgerOf(goodGate(), goodGate())))
	assert.Contains(t, strings.Join(f, "\n"), "duplicate id")
}

func TestParseLedgerNamesTheGateByPositionWhenIDIsEmpty(t *testing.T) {
	g := goodGate()
	g.ID = ""
	_, f := ParseLedger(marshal(t, ledgerOf(g)))
	assert.Contains(t, strings.Join(f, "\n"), "gate #1:")
}

func TestParseAttest(t *testing.T) {
	in, f := ParseAttest("android-local-mode-device; other-gate=skip:no device this week, sorry\n third=skip:  a reason long enough ")
	require.Empty(t, f)
	assert.Equal(t, Input{}, in["android-local-mode-device"])
	assert.Equal(t, Input{Skip: true, Reason: "no device this week, sorry"}, in["other-gate"])
	assert.Equal(t, Input{Skip: true, Reason: "a reason long enough"}, in["third"])

	in, f = ParseAttest("")
	assert.Empty(t, in)
	assert.Empty(t, f)
	in, f = ParseAttest(" ; ;\r\n")
	assert.Empty(t, in)
	assert.Empty(t, f)

	for _, c := range []struct{ in, want string }{
		{"Bad Id", "is not a gate id"},
		{"a-gate; a-gate", "names `a-gate` twice"},
		{"a-gate=yes", "only `skip:<reason>` is accepted"},
		{"a-gate=skip:short", "needs a real reason"},
		{"a-gate=skip:", "needs a real reason"},
		{"a-gate=", "only `skip:<reason>` is accepted"},
	} {
		_, f := ParseAttest(c.in)
		assert.Contains(t, strings.Join(f, "\n"), c.want, c.in)
	}
}

func TestMatches(t *testing.T) {
	for _, c := range []struct {
		glob, file string
		want       bool
	}{
		{"backend/embedded/**", "backend/embedded/server.go", true},
		{"backend/embedded/**", "backend/embedded/deep/er/x.go", true},
		{"backend/embedded/**", "backend/embedded", true}, // a trailing ** matches zero segments too
		{"backend/main.go", "backend/main.go", true},
		{"backend/main.go", "backend/main_test.go", false},
		{"backend/main.go", "backend/main.go/extra", false},
		{"android/core/data/**/local/*.kt", "android/core/data/src/main/kotlin/x/local/Host.kt", true},
		{"android/core/data/**/local/*.kt", "android/core/data/local/Host.kt", true},
		{"android/core/data/**/local/*.kt", "android/core/data/src/local/sub/Host.kt", false},
		{"android/core/data/**/local/*.kt", "android/core/data/src/local/Host.java", false},
		{"a/*/c", "a/b/c", true},
		{"a/*/c", "a/b/x/c", false},
		{"**", "anything/at/all", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/x/y/c", false},
		{"a/b", "a", false},
		{"a/[", "a/b", false}, // malformed glob never matches
	} {
		assert.Equal(t, c.want, Matches(c.glob, c.file), "%s vs %s", c.glob, c.file)
	}
}

func TestGateHelpers(t *testing.T) {
	g := goodGate()
	assert.True(t, g.Applies(KindRC))
	assert.True(t, g.Applies(KindFinal))
	g.AppliesTo = []string{KindFinal}
	assert.False(t, g.Applies(KindRC))

	assert.Nil(t, Gate{}.Latest())
	older := goodAttestation()
	older.Date = "2026-09-01"
	older.Commit = sha2c
	g.Attestations = []Attestation{goodAttestation(), older}
	assert.Equal(t, "2026-10-01", g.Latest().Date, "newest by date wins regardless of order")
	same := goodAttestation()
	same.Attester = "Later entry"
	g.Attestations = []Attestation{goodAttestation(), same}
	assert.Equal(t, "Later entry", g.Latest().Attester, "ties go to the later entry")

	l := ledgerOf(g, Gate{ID: "no-att"}, func() Gate { x := goodGate(); x.ID = "other"; x.Attestations = []Attestation{older}; return x }())
	assert.Equal(t, []string{sha1c, sha2c}, l.Commits())
}

func freshFacts() Facts {
	return Facts{sha1c: {Ancestor: true, Changed: []string{"docs/x.md", "backend/foo.go"}}}
}

func decide(in map[string]Input) map[string]Input { return in }

func TestCheckAttestedFresh(t *testing.T) {
	recs, f := Check(ledgerOf(goodGate()), KindFinal, decide(map[string]Input{gid: {}}), clock, freshFacts(), "drew")
	require.Empty(t, f)
	require.Len(t, recs, 1)
	r := recs[0]
	assert.Equal(t, gid, r.ID)
	assert.Equal(t, ModeAttested, r.Mode)
	assert.Equal(t, "fresh", r.LedgerState)
	assert.Equal(t, "drew", r.AttestedBy)
	assert.Equal(t, "2026-10-06T12:00:00Z", r.CheckedAt)
	require.NotNil(t, r.Attestation)
	assert.Equal(t, sha1c, r.Attestation.Commit)
}

func TestCheckAttestedFailures(t *testing.T) {
	cases := []struct {
		name  string
		g     func() Gate
		facts Facts
		now   time.Time
		want  string
	}{
		{"no attestation at all", func() Gate { g := goodGate(); g.Attestations = nil; return g }, freshFacts(), clock, "no attestation recorded"},
		{"older than max_age_days", goodGate, freshFacts(), clock.AddDate(0, 0, 30), "last attested 2026-10-01, 35 days ago (max 14)"},
		{"unparseable date", func() Gate { g := goodGate(); g.Attestations[0].Date = "garbage"; return g }, freshFacts(), clock, "date \"garbage\" is unparseable"},
		{"commit not an ancestor", goodGate, Facts{sha1c: {Ancestor: false}}, clock, "is not an ancestor of the release commit"},
		{"commit unknown to the checkout", goodGate, Facts{}, clock, "is not an ancestor of the release commit"},
		{"watched path changed", goodGate, Facts{sha1c: {Ancestor: true, Changed: []string{"backend/embedded/server.go"}}}, clock, "watched paths changed since the attested commit 111111111111: backend/embedded/server.go"},
		{"watched glob with ** in the middle", goodGate, Facts{sha1c: {Ancestor: true, Changed: []string{"android/core/data/src/main/local/Host.kt"}}}, clock, "android/core/data/src/main/local/Host.kt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			recs, f := Check(ledgerOf(c.g()), KindFinal, map[string]Input{gid: {}}, c.now, c.facts, "drew")
			require.Len(t, f, 1)
			assert.Contains(t, f[0], "cannot be attested")
			assert.Contains(t, f[0], c.want)
			assert.Contains(t, f[0], "scripts/android-local-mode-device-test.sh --record", "the finding says how to fix it")
			require.Len(t, recs, 1, "the record is still produced alongside the finding")
			assert.NotEqual(t, "fresh", recs[0].LedgerState)
		})
	}
}

func TestCheckAgeBoundaryIsInclusive(t *testing.T) {
	// attested 2026-10-01 00:00 UTC; 14 days later is exactly at the limit.
	_, f := Check(ledgerOf(goodGate()), KindFinal, map[string]Input{gid: {}}, time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC), freshFacts(), "x")
	assert.Empty(t, f)
	_, f = Check(ledgerOf(goodGate()), KindFinal, map[string]Input{gid: {}}, time.Date(2026, 10, 15, 0, 0, 1, 0, time.UTC), freshFacts(), "x")
	assert.NotEmpty(t, f)
}

func TestCheckListsAtMostFiveChangedWatchedPaths(t *testing.T) {
	var changed []string
	for _, n := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		changed = append(changed, "backend/embedded/"+n+".go")
	}
	_, f := Check(ledgerOf(goodGate()), KindFinal, map[string]Input{gid: {}}, clock, Facts{sha1c: {Ancestor: true, Changed: changed}}, "x")
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "(+2 more)")
	assert.NotContains(t, f[0], "backend/embedded/g.go")
}

func TestCheckSkipIsRecordedWithReasonAndLedgerState(t *testing.T) {
	g := goodGate()
	g.Attestations = nil
	recs, f := Check(ledgerOf(g), KindRC, map[string]Input{gid: {Skip: true, Reason: "device in the shop"}}, clock, nil, "drew")
	require.Empty(t, f, "a reasoned skip proceeds even though the ledger has nothing")
	require.Len(t, recs, 1)
	assert.Equal(t, ModeSkipped, recs[0].Mode)
	assert.Equal(t, "device in the shop", recs[0].Reason)
	assert.Equal(t, "no attestation recorded in the ledger", recs[0].LedgerState, "the artifact shows what was skipped")
	assert.Nil(t, recs[0].Attestation)
}

func TestCheckMissingGateNamesIt(t *testing.T) {
	recs, f := Check(ledgerOf(goodGate()), KindFinal, nil, clock, freshFacts(), "drew")
	require.Len(t, f, 1)
	assert.Contains(t, f[0], "manual gate `"+gid+"`")
	assert.Contains(t, f[0], "is not addressed by attest_manual_gates")
	assert.Contains(t, f[0], gid+"=skip:<reason>")
	assert.Empty(t, recs)
}

func TestCheckUnknownAndInapplicableGates(t *testing.T) {
	g := goodGate()
	g.AppliesTo = []string{KindFinal}
	other := goodGate()
	other.ID = "rc-only"
	other.AppliesTo = []string{KindRC}
	// an RC release: `gid` is final-only so not required; naming it is an error, as is a typo.
	recs, f := Check(ledgerOf(g, other), KindRC, map[string]Input{gid: {}, "rc-only": {Skip: true, Reason: "no device handy"}, "typo": {}}, clock, freshFacts(), "x")
	joined := strings.Join(f, "\n")
	assert.Contains(t, joined, "names `"+gid+"`, which does not apply to a rc release")
	assert.Contains(t, joined, "names `typo`, which is not a gate in the manual-gates ledger")
	require.Len(t, recs, 1)
	assert.Equal(t, "rc-only", recs[0].ID)

	// and a final release does not require the rc-only gate.
	recs, f = Check(ledgerOf(g, other), KindFinal, map[string]Input{gid: {}}, clock, freshFacts(), "x")
	assert.Empty(t, f)
	require.Len(t, recs, 1)
}
