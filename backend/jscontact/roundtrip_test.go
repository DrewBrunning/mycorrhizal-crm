package jscontact

import (
	"bytes"
	"testing"

	"mycorrhizal/internal/rfctest"
)

// Spot-check round-trips (docs/adrs/0003-golden-fixtures-external-test-oracle.md point 4): jscontact -> neutral
// -> jscontact on whole fixtures, asserting key fields survive. Confidence
// only, not exhaustive — the per-concept import/export tests already cover
// individual fields directly against the correspondence table.

func TestRoundTrip_JohnDoe(t *testing.T) {
	t.Parallel()
	raw := rfctest.LoadFixture("johndoe.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/uid", "22B2C7DF-9120-4969-8460-05956FE6B065")
	rfctest.AssertJSONPointer(t, out, "/kind", "individual")
	rfctest.AssertJSONPointer(t, out, "/name/components/0/kind", "given")
	rfctest.AssertJSONPointer(t, out, "/name/components/0/value", "John")
	rfctest.AssertJSONPointer(t, out, "/name/components/1/kind", "surname")
	rfctest.AssertJSONPointer(t, out, "/name/components/1/value", "Doe")
	rfctest.AssertJSONPointer(t, out, "/name/isOrdered", true)
}

func TestRoundTrip_TitleRole(t *testing.T) {
	t.Parallel()
	raw := rfctest.LoadFixture("title-role.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/titles/TITLE-1/kind", "title")
	rfctest.AssertJSONPointer(t, out, "/titles/TITLE-1/name", "Research Scientist")
	rfctest.AssertJSONPointer(t, out, "/titles/TITLE-2/kind", "role")
	rfctest.AssertJSONPointer(t, out, "/titles/TITLE-2/organizationId", "ORG-1")
	rfctest.AssertJSONPointer(t, out, "/organizations/ORG-1/name", "ABC, Inc.")
}

func TestRoundTrip_Phone(t *testing.T) {
	t.Parallel()
	raw := rfctest.LoadFixture("phone.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/phones/k1/number", "+15551234567")
	rfctest.AssertJSONPointer(t, out, "/phones/k1/features/voice", true)
}

func TestRoundTrip_Email(t *testing.T) {
	t.Parallel()
	raw := rfctest.LoadFixture("email.jscontact.json")
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/emails/k1/address", "alice@example.com")
}

// TestRoundTrip_MultiConceptCard is a hand-built card (RFC-syntax-conformant,
// no dedicated fixture) exercising several concepts at once through a full
// import->export cycle, including the vCardProps and unknown-top-level-
// property passthrough escape hatches together.
func TestRoundTrip_MultiConceptCard(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"@type": "Card", "version": "1.0", "uid": "multi-concept-example",
		"kind": "individual",
		"name": { "@type": "Name", "full": "Jane Q. Public", "components": [
			{ "kind": "given", "value": "Jane" }, { "kind": "surname", "value": "Public" }
		]},
		"emails": { "E1": { "@type": "EmailAddress", "address": "jane@example.com", "contexts": { "work": true } } },
		"phones": { "P1": { "@type": "Phone", "number": "+15559876543", "features": { "cell": true } } },
		"addresses": { "A1": { "@type": "Address", "components": [
			{ "kind": "locality", "value": "Springfield" }
		]}},
		"keywords": { "vip": true },
		"vCardProps": [ { "name": "x-custom", "type": "text", "value": "keep-me" } ],
		"x-unknown-vendor-prop": "verbatim"
	}`)
	rec, _, err := Adapter{}.Import(raw)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	out, _, err := Adapter{}.Export(rec)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	rfctest.AssertJSONPointer(t, out, "/uid", "multi-concept-example")
	rfctest.AssertJSONPointer(t, out, "/kind", "individual")
	rfctest.AssertJSONPointer(t, out, "/name/full", "Jane Q. Public")
	rfctest.AssertJSONPointer(t, out, "/emails/E1/address", "jane@example.com")
	rfctest.AssertJSONPointer(t, out, "/phones/P1/number", "+15559876543")
	rfctest.AssertJSONPointer(t, out, "/addresses/A1/components/0/value", "Springfield")
	rfctest.AssertJSONPointer(t, out, "/keywords/vip", true)
	rfctest.AssertJSONPointer(t, out, "/vCardProps/0/name", "x-custom")
	rfctest.AssertJSONPointer(t, out, "/x-unknown-vendor-prop", "verbatim")
}

// goldenFixturesJSContact is every docs/golden-fixtures/*.jscontact.json
// file (issue #255), mirrored byte-for-byte into internal/rfctest/fixtures.
var goldenFixturesJSContact = []string{
	"johndoe.jscontact.json",
	"title-role.jscontact.json",
}

// TestRoundtripIdempotent_GoldenFixtures is the vcard4 property of the same
// name, applied here: once a card has been through one Import->Export
// cycle, a second cycle must reproduce byte-identical output (Go's
// encoding/json sorts map keys deterministically, and this package's
// Import/Export call no time.Now/uuid.New/rand.* anything, so this is a
// safe, non-flaky property). See vcard4/roundtrip_test.go's copy of this
// test for the full rationale.
func TestRoundtripIdempotent_GoldenFixtures(t *testing.T) {
	t.Parallel()
	for _, name := range goldenFixturesJSContact {
		t.Run(name, func(t *testing.T) {
			raw := rfctest.LoadFixture(name)

			rec1, _, err := Adapter{}.Import(raw)
			if err != nil {
				t.Fatalf("Import (1st): %v", err)
			}
			out1, _, err := Adapter{}.Export(rec1)
			if err != nil {
				t.Fatalf("Export (1st): %v", err)
			}
			rec2, _, err := Adapter{}.Import(out1)
			if err != nil {
				t.Fatalf("Import (2nd): %v", err)
			}
			out2, _, err := Adapter{}.Export(rec2)
			if err != nil {
				t.Fatalf("Export (2nd): %v", err)
			}

			if !bytes.Equal(out1, out2) {
				t.Errorf("export is not idempotent after the first round trip:\n--- out1 ---\n%s\n--- out2 ---\n%s", out1, out2)
			}
		})
	}
}
