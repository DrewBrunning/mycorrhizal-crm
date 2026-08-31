// Package adversarial hosts the malformed/merely-broken import corpus
// (issue #432, TEST-04) and the harness that asserts ADR-0002's degradation
// tier for every fixture.
//
// The corpus lives in docs/adversarial-fixtures/ (with MANIFEST.md); this
// package embeds byte-identical copies under fixtures/ and carries the
// machine-readable manifest (manifest.go) that the docs table must mirror.
// Tier vocabulary (docs/adrs/0002-correspondence-table-locked-oracle.md):
//
//   - "preserve" — import completes without error; the demonstrated data
//     lands in a neutral field or Passthrough.
//   - "warn"     — import completes but emits >=1 Diagnostic{Severity:"warn"}.
//   - "error"    — import returns an error (not a valid instance of the
//     format at all).
//   - "bound"    — not a single-card tier; a multi-record fixture covered by
//     the dedicated bounded-failure tests in services/adversarial_import_test.go.
package adversarial

import (
	"embed"
	"fmt"
)

//go:embed fixtures
var fixturesFS embed.FS

// LoadFixture reads a file from fixtures/ by name (e.g. "str-truncated.vcf").
// It panics on error rather than taking a *testing.T: a missing fixture is a
// setup error, not an assertion failure.
func LoadFixture(name string) []byte {
	data, err := fixturesFS.ReadFile("fixtures/" + name)
	if err != nil {
		panic(fmt.Sprintf("adversarial.LoadFixture(%q): %v", name, err))
	}
	return data
}

// Fixture is one corpus entry: the declared expected behavior, not just a
// file. A fixture with no declaration is not a test.
type Fixture struct {
	Name     string // filename within fixtures/
	Category string // structural | encoding | semantic | size | injection | vendor | multi-record
	Format   string // "vcard" | "jscontact"
	Tier     string // preserve | warn | error | bound
	Note     string // what the fixture demonstrates / why the tier holds
}

// ByName returns the manifest entry for name, or nil.
func ByName(name string) *Fixture {
	for i := range Manifest {
		if Manifest[i].Name == name {
			return &Manifest[i]
		}
	}
	return nil
}
