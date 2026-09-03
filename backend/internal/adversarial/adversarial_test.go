package adversarial

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/jscontact"
	"mycorrhizal/services"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docsDir is the repo-root corpus this package's fixtures/ must mirror
// byte-for-byte (test working directory is backend/internal/adversarial).
const docsDir = "../../../docs/adversarial-fixtures"

// docsManifestPath is the human-readable manifest the binding Go table must
// never disagree with.
const docsManifestPath = "../../../docs/adversarial-fixtures/MANIFEST.md"

// TestEveryFixtureDeclared: a fixture file with no manifest entry is not a
// test. Listing every file (not the reverse) is the strong direction.
func TestEveryFixtureDeclared(t *testing.T) {
	t.Parallel()
	entries, err := fixturesFS.ReadDir("fixtures")
	require.NoError(t, err)
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".vcf") && !strings.HasSuffix(name, ".json") {
			continue
		}
		require.NotNil(t, ByName(name), "fixture %s has no declared expected tier in Manifest; a fixture with no declaration is not a test", name)
	}
}

// TestEveryManifestEntryResolves: a manifest entry pointing at a missing
// fixture is a dangling promise.
func TestEveryManifestEntryResolves(t *testing.T) {
	t.Parallel()
	for _, fx := range Manifest {
		if _, err := fixturesFS.ReadFile("fixtures/" + fx.Name); err != nil {
			t.Errorf("manifest entry %s has no fixture file: %v", fx.Name, err)
		}
	}
}

// TestDeclaredTierVocabulary: the tier strings are exactly the ADR-0002
// vocabulary; a typo like "presrve" would silently never be asserted.
func TestDeclaredTierVocabulary(t *testing.T) {
	t.Parallel()
	for _, fx := range Manifest {
		switch fx.Tier {
		case "preserve", "warn", "error", "bound":
		default:
			t.Errorf("manifest entry %s declares unknown tier %q", fx.Name, fx.Tier)
		}
		switch fx.Format {
		case "vcard", "jscontact":
		default:
			t.Errorf("manifest entry %s declares unknown format %q", fx.Name, fx.Format)
		}
	}
}

// TestManifestMirrorsDocs parses docs/adversarial-fixtures/MANIFEST.md's
// fixture table and asserts it and the binding Go manifest name every
// fixture identically, with the same category/format/tier. This is what
// stops the human-readable corpus description from drifting from the
// assertions actually run.
func TestManifestMirrorsDocs(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(docsManifestPath)
	require.NoError(t, err)

	docs := map[string][3]string{} // name -> {category, format, tier}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.Contains(line, ".vcf") && !strings.Contains(line, ".json") {
			continue
		}
		cols := []string{}
		for _, c := range strings.Split(line, "|") {
			cols = append(cols, strings.TrimSpace(c))
		}
		// cols[0] is the empty cell before the first pipe.
		if len(cols) < 5 {
			continue
		}
		docs[cols[1]] = [3]string{strings.ToLower(cols[2]), strings.ToLower(cols[3]), normTier(cols[4])}
	}

	require.Equal(t, len(Manifest), len(docs), "docs MANIFEST.md table and Go Manifest disagree on fixture count")

	for _, fx := range Manifest {
		got, ok := docs[fx.Name]
		if !ok {
			t.Errorf("Go Manifest entry %s is missing from docs MANIFEST.md", fx.Name)
			continue
		}
		assert.Equalf(t, fx.Category, got[0], "docs MANIFEST.md category for %s", fx.Name)
		assert.Equalf(t, fx.Format, got[1], "docs MANIFEST.md format for %s", fx.Name)
		assert.Equalf(t, fx.Tier, got[2], "docs MANIFEST.md tier for %s", fx.Name)
	}
}

// normTier maps the docs table's human "—" for multi-record fixtures onto
// the Go manifest's "bound".
func normTier(s string) string {
	if s == "—" || s == "-" {
		return "bound"
	}
	return s
}

// TestFixturesByteIdenticalToDocs proves the embedded copies are the docs
// corpus unchanged — no silent hand-edits to make a harness pass (the same
// rule ADR-0003 applies to golden fixtures).
func TestFixturesByteIdenticalToDocs(t *testing.T) {
	t.Parallel()
	for _, fx := range Manifest {
		embedded := LoadFixture(fx.Name)
		onDisk, err := os.ReadFile(filepath.Join(docsDir, fx.Name))
		require.NoError(t, err, fx.Name)
		assert.Equalf(t, onDisk, embedded, "embedded fixture %s differs from docs/adversarial-fixtures/%s", fx.Name, fx.Name)
	}
}

// importFixture routes one corpus file through the production import path —
// ImportVCardBlock (sniff + 2.1-normalize + vcard3/vcard4 dispatch, the same
// routing ParseVCF applies per block) for vCards, jscontact.Adapter for
// JSContact.
func importFixture(fx Fixture, raw []byte) (*contactmodel.Record, []contactmodel.Diagnostic, error) {
	if fx.Format == "jscontact" {
		return jscontact.Adapter{}.Import(raw)
	}
	return services.ImportVCardBlock(raw)
}

// TestDeclaredTiersHold is the core harness: every fixture gets exactly the
// tier its manifest declares. No fixture may panic or hang (a panic fails
// the test process; a hang trips the test timeout), and no fixture may
// produce a partially-written record where the tier says preserve.
func TestDeclaredTiersHold(t *testing.T) {
	t.Parallel()
	for _, fx := range Manifest {
		t.Run(fx.Name, func(t *testing.T) {
			raw := LoadFixture(fx.Name)
			record, diags, err := importFixture(fx, raw)

			switch fx.Tier {
			case "error":
				require.Error(t, err, "%s must error (not a valid instance of the format)", fx.Name)
			case "preserve":
				require.NoError(t, err, "%s must preserve (import must complete)", fx.Name)
				require.NotNil(t, record, "%s must produce a record, not a partial write", fx.Name)
			case "warn":
				require.NoError(t, err, "%s must complete with a warn diagnostic", fx.Name)
				require.NotNil(t, record, "%s must produce a record alongside its warn", fx.Name)
				require.True(t, hasWarn(diags), "%s must emit >=1 Diagnostic{Severity: warn}", fx.Name)
			case "bound":
				// Covered by the dedicated bounded-failure tests in
				// services/adversarial_import_test.go; nothing to assert
				// single-card.
			default:
				t.Fatalf("unknown tier %q (vocabulary test should have caught this)", fx.Tier)
			}
		})
	}
}

func hasWarn(diags []contactmodel.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == "warn" {
			return true
		}
	}
	return false
}

// TestPreservedDataLandsWhereDeclared: the preserve tier is not just
// "no error" — the demonstrated data has to land somewhere, not vanish.
// Unknown properties land in Passthrough (ADR-0002 tier 2); the specific
// landing spots for the fixtures that exist to prove passthrough are pinned
// here.
func TestPreservedDataLandsWhereDeclared(t *testing.T) {
	t.Parallel()
	// ven-x-properties.vcf: every X- property must survive into Passthrough.VCard.
	rec, _, err := importFixture(*ByName("ven-x-properties.vcf"), LoadFixture("ven-x-properties.vcf"))
	require.NoError(t, err)
	wantProps := map[string]bool{"x-vendor-note": false, "x-foo": false, "x-google-custom": false}
	for _, p := range rec.Passthrough.VCard {
		if _, ok := wantProps[strings.ToLower(p.Name)]; ok {
			wantProps[strings.ToLower(p.Name)] = true
		}
	}
	for name, found := range wantProps {
		assert.Truef(t, found, "ven-x-properties.vcf: %s did not land in Passthrough.VCard", name)
	}
	assert.Equal(t, "Ada", rec.Card.Name.Components[1].Value, "known FN data must still land in the neutral model alongside passthrough")

	// ven-apple-grouped.vcf: EMAIL/TEL land in neutral fields, X-ABLabel via passthrough.
	rec, _, err = importFixture(*ByName("ven-apple-grouped.vcf"), LoadFixture("ven-apple-grouped.vcf"))
	require.NoError(t, err)
	require.Len(t, rec.Card.Emails, 1)
	assert.Equal(t, "ada@example.com", rec.Card.Emails[0].Address)
	require.Len(t, rec.Card.Phones, 1)
	assert.Equal(t, "555-123-4567", rec.Card.Phones[0].Number)
	hasABLabel := false
	for _, p := range rec.Passthrough.VCard {
		if strings.EqualFold(p.Name, "X-ABLabel") {
			hasABLabel = true
		}
	}
	assert.True(t, hasABLabel, "ven-apple-grouped.vcf: X-ABLabel must be preserved via passthrough")

	// js-unknown-top-level.json: unknown top-level keys preserved via Passthrough.JSContact.
	rec, _, err = importFixture(*ByName("js-unknown-top-level.json"), LoadFixture("js-unknown-top-level.json"))
	require.NoError(t, err)
	for _, key := range []string{"/xCustomThing", "/extensions"} {
		_, ok := rec.Passthrough.JSContact[key]
		assert.Truef(t, ok, "js-unknown-top-level.json: %s did not land in Passthrough.JSContact", key)
	}
	assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
}

// TestSizeHostility_Amplified completes the size category the committed seed
// fixtures only start: the corpus's own NOTE/property counts amplified to the
// magnitudes issue #415's limits exist for, proving the parser itself
// completes (no panic, no hang) on them. The upload-boundary limits
// themselves are #415's suite, cross-referenced in docs/security/asvs-l2.md.
func TestSizeHostility_Amplified(t *testing.T) {
	t.Parallel()
	seed := LoadFixture("size-huge-property.vcf")
	// Amplify the single NOTE to ~2 MB — well under MaxVCFSize (50 MB) but
	// far past any line/property-size threshold a naive parser would trip.
	huge := strings.Replace(string(seed), strings.Repeat("A", 102400), strings.Repeat("A", 2*1024*1024), 1)
	require.NotEqual(t, string(seed), huge, "amplification must actually enlarge the NOTE")
	rec, _, err := services.ImportVCardBlock([]byte(huge))
	require.NoError(t, err)
	require.NotNil(t, rec)
	require.Len(t, rec.Card.Notes, 1)
	assert.GreaterOrEqual(t, len(rec.Card.Notes[0].Note), 2*1024*1024-10)

	// Amplify the 2000 X- properties to 20000 on one card.
	seed = LoadFixture("size-many-properties.vcf")
	var b strings.Builder
	b.WriteString("BEGIN:VCARD\r\nVERSION:4.0\r\nUID:size-many-amplified\r\nFN:Ada Lovelace\r\nN:Lovelace;Ada;;;\r\n")
	for i := 0; i < 20000; i++ {
		b.WriteString("X-EXTRA-")
		b.WriteString(strings.Repeat("0", len("20000")))
		b.WriteString(":")
		b.WriteString(strings.Repeat("v", 64))
		b.WriteString("\r\n")
	}
	b.WriteString("END:VCARD\r\n")
	rec, _, err = services.ImportVCardBlock([]byte(b.String()))
	require.NoError(t, err)
	require.NotNil(t, rec)
	assert.Equal(t, 20000, len(rec.Passthrough.VCard), "every unknown property must be preserved, none dropped")
}
