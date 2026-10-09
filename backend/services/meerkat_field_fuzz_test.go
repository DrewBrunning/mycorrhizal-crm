package services

import (
	"strings"
	"testing"
)

// meerkatFieldFuzzSeeds are the JSON column shapes the shared Meerkat import
// fixture (testdata/meerkat-fixture/manifest.json) writes into a contact row:
// Meerkat stores emails/phones/urls/impps/addresses as JSON arrays and
// vcard_extra as a legacy property bag (see the reader in meerkat/reader.go).
// Kept literal rather than derived from the manifest so a reader/mapper change
// is a reviewable edit here too.
var meerkatFieldFuzzSeeds = []string{
	`[{"type":"home","value":"ada@example.com"},{"type":"work","value":"ada.lovelace@analytical.example"}]`,
	`[{"type":"cell","value":"+1-555-0100"},{"type":"home","value":"+44-20-7946-0101"}]`,
	`[{"type":"telegram","value":"@adalovelace"}]`,
	`[{"type":"home","value":"https://ada.example.com"}]`,
	`[{"type":"home","street":"12 Byron St","city":"London","region":"England","postal":"SW1A 1AA","country":"United Kingdom"}]`,
	`[{"value":""},{"value":"  "}]`,
	`{"properties":{"X-FOO":[{"Value":"bar","Params":{"TYPE":["home"]},"Group":"item1"}]}}`,
	`null`,
	``,
}

// FuzzMeerkatFieldParsers covers issue #1625's Meerkat field-parser target:
// the JSON-array column parsers in meerkat_import.go are the untrusted-input
// boundary once a source database row is read — every value traces back to a
// Meerkat deployment's own free-form JSON columns, and a malformed column must
// degrade to "no data" rather than panic or emit a half-built row.
//
// The parsers take a *string, so the target feeds the mutated value to all of
// them (plus one explicit nil call each, the absent-column branch) and asserts
// two cheap invariants: the number of parsed entities can never exceed the
// input length (each parsed entity needs at least a few bytes of JSON), and
// every emitted entity carries a non-empty, trimmed value — a parser must not
// signal "ok" by returning a zero-valued element.
func FuzzMeerkatFieldParsers(f *testing.F) {
	for _, seed := range meerkatFieldFuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data string) {
		raw := &data

		emails := parseMeerkatEmails(raw)
		if len(emails) > len(data)+1 {
			t.Fatalf("parseMeerkatEmails emitted %d emails for %d bytes of input", len(emails), len(data))
		}
		for _, e := range emails {
			if strings.TrimSpace(e.Address) == "" {
				t.Fatalf("parseMeerkatEmails emitted an empty address for input %q", data)
			}
		}

		phones := parseMeerkatPhones(raw)
		if len(phones) > len(data)+1 {
			t.Fatalf("parseMeerkatPhones emitted %d phones for %d bytes of input", len(phones), len(data))
		}
		for _, p := range phones {
			if strings.TrimSpace(p.Number) == "" {
				t.Fatalf("parseMeerkatPhones emitted an empty number for input %q", data)
			}
		}

		impps := parseMeerkatIMPPs(raw)
		if len(impps) > len(data)+1 {
			t.Fatalf("parseMeerkatIMPPs emitted %d services for %d bytes of input", len(impps), len(data))
		}
		for _, i := range impps {
			if strings.TrimSpace(i.URI) == "" {
				t.Fatalf("parseMeerkatIMPPs emitted an empty URI for input %q", data)
			}
		}

		links := parseMeerkatLinks(raw)
		if len(links) > len(data)+1 {
			t.Fatalf("parseMeerkatLinks emitted %d links for %d bytes of input", len(links), len(data))
		}
		for _, l := range links {
			if strings.TrimSpace(l.URI) == "" {
				t.Fatalf("parseMeerkatLinks emitted an empty URI for input %q", data)
			}
		}

		addresses := parseMeerkatAddresses(raw)
		if len(addresses) > len(data)+1 {
			t.Fatalf("parseMeerkatAddresses emitted %d addresses for %d bytes of input", len(addresses), len(data))
		}
		for _, a := range addresses {
			if len(a.Components) == 0 {
				t.Fatalf("parseMeerkatAddresses emitted an all-empty address for input %q", data)
			}
		}

		extra := parseMeerkatVCardExtra(raw)
		if len(extra.VCard) > len(data)+1 {
			t.Fatalf("parseMeerkatVCardExtra emitted %d props for %d bytes of input", len(extra.VCard), len(data))
		}

		// normalizeSourceDate only trims; it must be idempotent and never grow
		// its input.
		normalized := normalizeSourceDate(data)
		if len(normalized) > len(data) {
			t.Fatalf("normalizeSourceDate grew %d bytes to %d", len(data), len(normalized))
		}
		if normalizeSourceDate(normalized) != normalized {
			t.Fatalf("normalizeSourceDate not idempotent for %q", data)
		}

		// The nil-column branch of every parser (an absent SQLite column reads
		// back as nil): must be a clean no-op, never a panic or a zero entity.
		if got := parseMeerkatEmails(nil); got != nil {
			t.Fatalf("parseMeerkatEmails(nil) = %v, want nil", got)
		}
		if got := parseMeerkatPhones(nil); got != nil {
			t.Fatalf("parseMeerkatPhones(nil) = %v, want nil", got)
		}
		if got := parseMeerkatIMPPs(nil); got != nil {
			t.Fatalf("parseMeerkatIMPPs(nil) = %v, want nil", got)
		}
		if got := parseMeerkatLinks(nil); got != nil {
			t.Fatalf("parseMeerkatLinks(nil) = %v, want nil", got)
		}
		if got := parseMeerkatAddresses(nil); got != nil {
			t.Fatalf("parseMeerkatAddresses(nil) = %v, want nil", got)
		}
		if got := parseMeerkatVCardExtra(nil); len(got.VCard) != 0 {
			t.Fatalf("parseMeerkatVCardExtra(nil) = %v, want empty", got)
		}
	})
}
