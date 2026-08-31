// Package differential implements the TEST-08 differential suite (issue #680):
// for every format (vCard 3.0, vCard 4.0, JSContact, iCalendar) and every
// contact in the shared corpus (TEST-02's pathological fixture #430, the
// golden fixtures of ADR-0003, and the property generator's seeds #435), the
// same input is run through our pipeline and through an *independent*,
// pinned reference implementation, and semantic equivalence is asserted per
// TEST-03's comparator (backend/internal/semanticequal, #431).
//
// Why a reference at all: the round-trip suite (#431) and the golden
// fixtures (ADR-0003) both compare our output against an oracle we wrote.
// The failure mode #496 names is code and tests consistently, confidently
// wrong — a parser and an exporter sharing the same (wrong) reading of the
// spec pass each other's round trips. A reference implementation that shares
// none of our code is the countermeasure: a disagreement between us and it
// is evidence at least one of us misreads the RFC, and the RFC is the
// tiebreaker.
//
// # Independence requirement
//
// The reference must not be a library our own adapters are built on. That
// rules out emersion/go-vcard (backend/vcard3 and backend/vcard4 are layered
// directly on it) and emersion/go-ical (backend/caldav uses it) — comparing
// against them would compare our mapping against our own low-level code.
// The pinned references are therefore:
//
//   - vCard 3.0 / 4.0: Python vobject 0.9.9
//     (backend/differential/reference/vobject/vcard_ref.py). The Go test
//     shells out to `python3`; the script is self-contained and ships here.
//   - JSContact: Rust calcard (pinned revision) via the reference CLI in
//     backend/differential/reference/calcard, using calcard's independent
//     RFC 9555 vCard<->JSContact conversion engine as the reference middle.
//   - iCalendar: github.com/arran4/golang-ical (pinned in go.mod) — a
//     genuinely independent pure-Go implementation (our caldav backend uses
//     emersion/go-ical), so this leg runs per-PR with no extra runtime.
//
// Supply-chain pins live in docs/development/testing.md (TEST-08 section);
// reference CLIs and versions are pinned so a disagreement is reproducible.
//
// # Divergence policy
//
// The corpus deliberately contains pathological cards (TEST-02) and the
// references are imperfect implementations, so "reference disagrees with us"
// is not always a bug. The suite implements the pinned allow-list with drift
// detection (the #496 pattern): known reference-side divergences are pinned
// per corpus entry + direction + concept with a written reason, and:
//
//   - an UNPINNED disagreement fails the test, naming the concept (never
//     "outputs differ");
//   - a pin that no longer reproduces (the reference got fixed, or a corpus
//     card changed so the divergence is gone) fails the test, so pins cannot
//     silently rot.
//
// A divergence we confirm is OUR bug is fixed as a red directional test, not
// pinned. A divergence we confirm is the reference's is pinned (and feeds
// the #432 fixture corpus).
package differential

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/canonicalfixture"
	"mycorrhizal/internal/rfctest"
	"mycorrhizal/jscontact"
	"mycorrhizal/vcard3"
	"mycorrhizal/vcard4"
)

// CorpusEntry is one contact in the differential corpus. ID is the stable
// key the divergence pins reference (e.g. "fixture/ada", "golden/rfc6350-baseline.v4.vcf",
// "seed/gen-0003").
type CorpusEntry struct {
	ID     string
	Record *contactmodel.Record
}

// goldenFixtureNames is the ADR-0003 golden-fixture corpus (via
// internal/rfctest, which embeds backend/internal/rfctest/fixtures — the
// checked-in verbatim RFC example cards). Each is imported through OUR
// importer for its own format to obtain a neutral record; the differential
// then treats that record as ground truth and runs it through the reference
// in both directions. Import failures are fixture bugs (the golden cards
// must parse) and fail the corpus test loudly.
var goldenFixtureNames = []struct {
	name     string
	importer contactmodel.Importer
}{
	{"rfc6350-baseline.v4.vcf", vcard4.Adapter{}},
	{"rfc2426-baseline.v3.vcf", vcard3.Adapter{}},
	{"created.v4.vcf", vcard4.Adapter{}},
	{"derived-fn.v4.vcf", vcard4.Adapter{}},
	{"n-expanded.v4.vcf", vcard4.Adapter{}},
	{"phonetic-n.v4.vcf", vcard4.Adapter{}},
	{"note-author.v4.vcf", vcard4.Adapter{}},
	{"gramgender.v4.vcf", vcard4.Adapter{}},
	{"pronouns.v4.vcf", vcard4.Adapter{}},
	{"adr-expanded.v4.vcf", vcard4.Adapter{}},
	{"socialprofile.v4.vcf", vcard4.Adapter{}},
	{"title-role.v4.vcf", vcard4.Adapter{}},
	{"johndoe.jscontact.json", jscontact.Adapter{}},
	{"title-role.jscontact.json", jscontact.Adapter{}},
	{"email.jscontact.json", jscontact.Adapter{}},
	{"phone.jscontact.json", jscontact.Adapter{}},
}

// ContactCorpus assembles every neutral contact the contact-format
// differential legs run: the canonical fixture contacts, the golden
// fixtures, and the pinned generated seeds. Each entry's ID is stable so
// divergence pins survive corpus regeneration.
func ContactCorpus() ([]CorpusEntry, error) {
	var out []CorpusEntry

	m, err := canonicalfixture.Read()
	if err != nil { // # pragma: no cover — a broken checked-in manifest fails the canonicalfixture suite first
		return nil, err // # pragma: no cover
	}
	for _, c := range m.Contacts {
		rec := c.Record()
		if c.RecreatesVCardUIDOf != "" {
			rec.Card.UID = inheritedUID(m, c.RecreatesVCardUIDOf)
		}
		out = append(out, CorpusEntry{ID: "fixture/" + c.Name, Record: rec})
	}

	for _, gf := range goldenFixtureNames {
		raw := rfctest.LoadFixture(gf.name)
		rec, _, err := gf.importer.Import(raw)
		if err != nil { // # pragma: no cover — a golden fixture that stops parsing is a fixture bug caught by the golden-fixture suites first
			return nil, fmt.Errorf("differential: golden fixture %s failed to import: %w", gf.name, err) // # pragma: no cover
		}
		out = append(out, CorpusEntry{ID: "golden/" + gf.name, Record: rec})
	}

	seeds, err := readSeedCorpus()
	if err != nil { // # pragma: no cover — the seeds file is committed; a missing/corrupt file fails the corpus test below
		return nil, err // # pragma: no cover
	}
	for i, rec := range seeds {
		out = append(out, CorpusEntry{ID: fmt.Sprintf("seed/gen-%04d", i), Record: rec})
	}

	return out, nil
}

func inheritedUID(m *canonicalfixture.Manifest, name string) string {
	for _, c := range m.Contacts {
		if c.Name == name {
			return c.Card.UID
		}
	}
	return "" // # pragma: no cover — Validate() guarantees RecreatesVCardUIDOf names an earlier contact
}

// readSeedCorpus loads the pinned generated-seed records (testdata/generated-seeds.json).
// The file is generated from the shared TEST-07 generator (internal/contactgen)
// with a fixed RAPID_SEED, so the seeds are stable across runs and reviewable
// as a diff; see the "Regenerating the seed corpus" note in corpus_test.go.
func readSeedCorpus() ([]*contactmodel.Record, error) {
	path := filepath.Join("testdata", "generated-seeds.json")
	data, err := os.ReadFile(path) // #nosec G304 -- a fixed relative testdata path, not request input
	if err != nil {                // # pragma: no cover — the seeds file is committed; TestContactCorpus guards its presence
		return nil, fmt.Errorf("differential: reading %s: %w (regenerate with the pinned contactgen generator)", path, err) // # pragma: no cover
	}
	var seeds []*contactmodel.Record
	if err := json.Unmarshal(data, &seeds); err != nil { // # pragma: no cover — a corrupt committed seeds file fails TestContactCorpus first
		return nil, fmt.Errorf("differential: parsing %s: %w", path, err) // # pragma: no cover
	}
	return seeds, nil
}
