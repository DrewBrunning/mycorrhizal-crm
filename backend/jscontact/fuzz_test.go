package jscontact

import (
	"encoding/json"
	"testing"

	"mycorrhizal/contactmodel"
	"mycorrhizal/internal/rfctest"
)

// jscontactFuzzSeedFiles lists the JSContact golden fixtures
// (docs/golden-fixtures) used to seed both fuzz targets below. Matches
// roundtrip_test.go's coverage (TestRoundTrip_JohnDoe/_TitleRole/_Phone plus
// the email fixture) — same deliberate-opt-in rationale as vcard4/vcard3's
// seed lists.
var jscontactFuzzSeedFiles = []string{
	"johndoe.jscontact.json",
	"title-role.jscontact.json",
	"phone.jscontact.json",
	"email.jscontact.json",
}

// FuzzImportJSContact mirrors vcard4's FuzzImportVCard4 (issue #376,
// extending #265's coverage to the other hostile-input parsers).
// Adapter.Import here parses raw JSON directly (no third-party decoder to
// wrap, unlike vcard3/vcard4) but is the same untrusted-input boundary:
// file-upload import of a JSContact export.
//
// The fuzz harness itself only needs to not panic; go test -fuzz catches
// panics as failures automatically. The round-trip check mirrors
// vcard3/vcard4's: a record that imports cleanly should also export and
// re-import cleanly.
func FuzzImportJSContact(f *testing.F) {
	for _, name := range jscontactFuzzSeedFiles {
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

// FuzzExportJSContact mirrors vcard4's FuzzExportVCard4: fuzzes the JSON
// encoding of a Record (Go's native fuzzer only takes primitives, not
// structs) rather than a Record directly. Seeds are real records produced by
// importing the golden fixtures, plus the zero value — the other edge every
// export* helper must tolerate besides any explicit nil guard.
func FuzzExportJSContact(f *testing.F) {
	for _, name := range jscontactFuzzSeedFiles {
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
