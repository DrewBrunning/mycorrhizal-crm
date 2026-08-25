package vcard3

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// vcard3FuzzSeedFiles lists the v3 golden fixtures (docs/golden-fixtures)
// used to seed both fuzz targets below. Same rationale as vcard4's
// vcard4FuzzSeedFiles: a new fixture is deliberately opted in, not picked up
// silently. Only one v3 fixture exists today (roundtrip_test.go's own
// coverage) — still a valid seed corpus, just a thin one; add to this list
// as more v3 fixtures land.
var vcard3FuzzSeedFiles = []string{
	"rfc2426-baseline.v3.vcf",
}

// FuzzImportVCard3 mirrors vcard4's FuzzImportVCard4 (issue #376, extending
// #265's coverage to the other hostile-input parsers): Adapter.Import wraps
// the same third-party github.com/emersion/go-vcard decoder vcard4 does, so
// it's the same untrusted-input boundary (CardDAV, file-upload import) for
// v3 cards specifically.
//
// The fuzz harness itself only needs to not panic; go test -fuzz catches
// panics as failures automatically. The round-trip check mirrors vcard4's:
// a record that imports cleanly should also export and re-import cleanly.
func FuzzImportVCard3(f *testing.F) {
	for _, name := range vcard3FuzzSeedFiles {
		f.Add(rfctest.LoadFixture(name))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		rec, _, err := Adapter{}.Import(data)
		if err != nil || rec == nil {
			return
		}
		out, _, err := Adapter{}.Export(rec)
		if err != nil {
			return
		}
		_, _, _ = Adapter{}.Import(out)
	})
}

// FuzzExportVCard3 mirrors vcard4's FuzzExportVCard4: fuzzes the JSON
// encoding of a Record (Go's native fuzzer only takes primitives, not
// structs) rather than a Record directly. Seeds are real records produced by
// importing the golden fixture, plus the zero value — the other edge every
// export* helper must tolerate besides the explicit nil guard.
func FuzzExportVCard3(f *testing.F) {
	for _, name := range vcard3FuzzSeedFiles {
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
