package services

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mycorrhizal/internal/clock"
	"mycorrhizal/monica"
)

// f1625ReadFixture resolves a repo-root-relative fixture by walking up from the
// test process's working directory (works from backend/, backend/services/, or
// the repo root). testing.TB is satisfied by both *testing.T and *testing.F, so
// the outer fuzz function can seed the corpus with the same helper a normal
// test uses. Named for the issue to avoid colliding with helpers another agent
// may add to this concurrently-edited package.
func f1625ReadFixture(tb testing.TB, rel string) []byte {
	tb.Helper()
	dir, err := os.Getwd()
	if err != nil {
		tb.Fatalf("getwd: %v", err)
	}
	for {
		if b, err := os.ReadFile(filepath.Join(dir, rel)); err == nil { // #nosec G304 -- checked-in fixture path
			return b
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatalf("fixture %q not found above %s", rel, dir)
		}
		dir = parent
	}
}

// FuzzMonicaDecodeAndMap covers issue #1625's Monica target: a Monica snapshot
// fixture is reused here as an untrusted upload — monica.LoadSnapshot parses
// arbitrary JSON into the wire model, then MapMonicaSnapshot maps it (through
// mapMonicaContact and the rest of monica_import.go) onto the import plan. Both
// halves are the boundary a source import actually crosses, so a decoder or
// mapper panic is reachable from a malformed snapshot.
//
// The decode lives in package monica and the mapping in package services (no
// cycle: services imports monica). LoadSnapshot takes an io.Reader, so the
// fuzzed bytes go straight in.
//
// The harness only needs to not panic; go test -fuzz catches panics
// automatically. The extra invariant is the mapper's core promise: it never
// invents contacts — the plan can carry at most one contact per snapshot
// contact. A fixed fake clock keeps the mapper's reminder/fallback-date
// branches off the wall clock (issue #1494), so runs are deterministic.
func FuzzMonicaDecodeAndMap(f *testing.F) {
	seed := f1625ReadFixture(f, "testdata/monica-fixture/snapshot.json")
	f.Add(seed)
	f.Add([]byte(`{"contacts":[]}`))
	f.Add([]byte(`{`))
	f.Add([]byte(`null`))

	// Fixed "now" for the mapper's scheduling; also install it as the services
	// clock so the mapper's internal Now() fallbacks agree. Restored after the
	// fuzz run returns.
	fixedNow := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	defer SetClock(clock.NewFake(fixedNow))()

	f.Fuzz(func(t *testing.T, data []byte) {
		snap, err := monica.LoadSnapshot(bytes.NewReader(data))
		if err != nil || snap == nil {
			return
		}
		plan := MapMonicaSnapshot(snap, fixedNow)
		if plan == nil {
			t.Fatal("MapMonicaSnapshot returned a nil plan for a decoded snapshot")
		}
		if len(plan.Contacts) > len(snap.Contacts) {
			t.Fatalf("mapped %d contacts from a %d-contact snapshot", len(plan.Contacts), len(snap.Contacts))
		}
		// Exercise mapMonicaContact directly too: the snapshot mapper skips
		// partial contacts before calling it, so this reaches the per-contact
		// mapper with every shape the JSON can express.
		for _, c := range snap.Contacts {
			ref := SourceRef{System: "monica", ExternalID: monicaContactRef(c.ID)}
			_ = mapMonicaContact(c, ref, plan)
		}
	})
}
