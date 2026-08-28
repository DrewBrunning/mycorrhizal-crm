package vcard4

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// vcard4FuzzSeedFiles lists every v4 golden fixture (docs/golden-fixtures)
// used to seed both fuzz targets below. Kept as one list rather than a glob
// so a new fixture is deliberately opted into fuzzing, not picked up
// silently — golden fixtures are the external RFC oracle (see
// docs/adrs/0003-golden-fixtures-external-test-oracle.md); this list should
// track roundtrip_test.go's coverage, not exceed it.
var vcard4FuzzSeedFiles = []string{
	"rfc6350-baseline.v4.vcf",
	"n-expanded.v4.vcf",
	"adr-expanded.v4.vcf",
	"created.v4.vcf",
	"derived-fn.v4.vcf",
	"gramgender.v4.vcf",
	"note-author.v4.vcf",
	"phonetic-n.v4.vcf",
	"pronouns.v4.vcf",
	"socialprofile.v4.vcf",
	"title-role.v4.vcf",
}

// maxVCard4FuzzInput bounds what FuzzImportVCard4 will process. The
// Import->Export->Import chain is linear in input size, which is fine for a
// one-shot file import but means a large fuzzer-generated input can
// legitimately take seconds — enough to stall the time-boxed CI fuzz smoke
// ("context deadline exceeded" flake on 2-core runners when a big input lands
// at the -fuzztime boundary, the FuzzImportJSContact failure that motivated
// this cap). The target is a panic/crash tripwire, not a wall-clock
// benchmark; parser bugs are shape-driven, not size-driven, so 1MiB covers
// the meaningful space with a wide margin. Real uploads are separately
// bounded by MaxVCFSize.
const maxVCard4FuzzInput = 1 << 20 // 1 MiB

// FuzzImportVCard4 is issue #265's primary target: malformed property
// values are the classic crash vector at the sync boundary (CardDAV
// incremental sync, file-upload import), and Adapter.Import is exactly that
// boundary — it wraps the third-party github.com/emersion/go-vcard decoder
// with this package's own neutral-model field mapping, so a fuzz failure in
// either layer is caught here.
//
// The fuzz harness itself only needs to not panic; go test -fuzz catches
// panics as failures automatically. The extra round-trip check below is a
// bonus invariant (mirrors roundtrip_test.go's spot checks): a record that
// imports cleanly should also export and re-import cleanly, so a crash
// reachable only via Export from fuzzer-discovered input is caught too,
// not just crashes directly in Import.
func FuzzImportVCard4(f *testing.F) {
	for _, name := range vcard4FuzzSeedFiles {
		f.Add(rfctest.LoadFixture(name))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > maxVCard4FuzzInput {
			return
		}
		rec, _, err := Adapter{}.Import(data)
		if err != nil || rec == nil {
			return
		}
		out, _, err := Adapter{}.Export(rec)
		if err != nil {
			return
		}
		// A second Import of our own Export output must not panic either —
		// the round trip is the same code path a CardDAV client's re-fetch
		// after a push would exercise.
		_, _, _ = Adapter{}.Import(out)
	})
}

// FuzzExportVCard4 covers the exporter direction the ticket also names,
// lower-value than Import (its input is always this app's own internal
// Record, never untrusted bytes directly) but still a Go struct built from
// data that ultimately traces back to a CardDAV PUT or a file import once
// persisted and reloaded. Fuzzing a struct directly isn't supported by Go's
// native fuzzer (only primitives), so this fuzzes the JSON encoding of a
// Record instead: seeds are real records (produced by importing the golden
// fixtures, then re-marshaled), and the fuzzer mutates that JSON. Inputs
// that no longer unmarshal are simply skipped (not every mutation produces
// a meaningful Record, which is fine — go test -fuzz only cares that
// Export itself never panics on whatever *does* unmarshal).
func FuzzExportVCard4(f *testing.F) {
	for _, name := range vcard4FuzzSeedFiles {
		rec, _, err := Adapter{}.Import(rfctest.LoadFixture(name))
		if err != nil {
			f.Fatalf("seed fixture %s failed to import: %v", name, err)
		}
		seed, err := json.Marshal(rec)
		if err != nil {
			f.Fatalf("seed fixture %s failed to marshal: %v", name, err)
		}
		f.Add(seed)
	}
	// Also seed the zero value — Export's one explicit guard (nil) aside,
	// an all-empty-but-non-nil Record is the other edge every export*
	// helper must tolerate.
	zero, err := json.Marshal(&contactmodel.Record{})
	if err != nil {
		f.Fatalf("marshal zero Record: %v", err)
	}
	f.Add(zero)

	f.Fuzz(func(t *testing.T, data []byte) {
		var rec contactmodel.Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return
		}
		_, _, _ = Adapter{}.Export(&rec)
	})
}
