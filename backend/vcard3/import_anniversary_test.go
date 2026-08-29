package vcard3

import "testing"

// Concept covered: anniversary.birth.
func init() {
	registerImportCoverage("anniversary.birth")
}

const anniversaryBirthImportVCF = "BEGIN:VCARD\n" +
	"VERSION:3.0\n" +
	"FN:Frank Dawson\n" +
	"BDAY:1996-04-15\n" +
	"END:VCARD\n"

func TestImport_AnniversaryBirth(t *testing.T) {
	rec, _, err := (Adapter{}).Import([]byte(anniversaryBirthImportVCF))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var found *int
	for _, a := range rec.Card.Anniversaries {
		if a.Kind == "birth" && a.Date.Partial != nil {
			found = a.Date.Partial.Year
		}
	}
	if found == nil || *found != 1996 {
		t.Errorf("birth anniversary year = %v, want 1996 (anniversaries=%+v)", found, rec.Card.Anniversaries)
	}
}

// TestImport_AnniversaryBirthYearlessDashed pins the extended dashed --MM-DD
// form (the same form this package's exporter emits and vcard4's parser
// accepts): a year-less leap-day birthday must come back as a PartialDate,
// not fall through to the timestamp default (issue #431 round-trip surfaced
// this as a mangled date).
func TestImport_AnniversaryBirthYearlessDashed(t *testing.T) {
	rec, _, err := (Adapter{}).Import([]byte("BEGIN:VCARD\nVERSION:3.0\nFN:Leap\nBDAY:--02-29\nEND:VCARD\n"))
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if len(rec.Card.Anniversaries) != 1 {
		t.Fatalf("anniversaries = %+v, want exactly one", rec.Card.Anniversaries)
	}
	a := rec.Card.Anniversaries[0]
	if a.Date.Partial == nil {
		t.Fatalf("anniversary date = %+v, want a PartialDate (year-less leap day)", a.Date)
	}
	if a.Date.Partial.Year != nil || a.Date.Partial.Month == nil || *a.Date.Partial.Month != 2 || a.Date.Partial.Day == nil || *a.Date.Partial.Day != 29 {
		t.Errorf("anniversary = %+v, want partial month 2 day 29 with no year", a.Date.Partial)
	}
}
