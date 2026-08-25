package services

import (
	"strings"
	"testing"
)

// csvFuzzSeeds mirrors the shapes import_service_gaps_test.go already
// exercises for ParseCSV: a happy path, ragged (inconsistent column count)
// rows — deliberately tolerated, not an error — quoted fields, and an
// embedded quote/comma edge case. LazyQuotes is on in ParseCSV, so malformed
// quoting is also worth seeding rather than only well-formed CSV.
var csvFuzzSeeds = []string{
	"First Name,Email\nAda,ada@example.com\nBob,bob@example.com\n",
	"A,B,C\n1,2\n1,2,3,4\n",
	`Name,Note` + "\n" + `"Doe, Jane","Said ""hello"" once"` + "\n",
	"",
	"First Name,Email\n",
	"A,B\n\"unterminated quote,x\n",
}

// FuzzParseCSV covers issue #376's CSV import target: services.ParseCSV
// (import_service.go:49) is a pure parser with no DB dependency — unlike
// ParseVCF/ParseJSContact, which already round-trip through the
// vcard3/vcard4/jscontact adapters fuzzed elsewhere (#265, #376) — so it's
// fuzzed directly here as the untrusted-input boundary for CSV file-upload
// import.
//
// The fuzz harness itself only needs to not panic; go test -fuzz catches
// panics as failures automatically.
func FuzzParseCSV(f *testing.F) {
	for _, seed := range csvFuzzSeeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, data string) {
		_, _, _ = ParseCSV(strings.NewReader(data))
	})
}
