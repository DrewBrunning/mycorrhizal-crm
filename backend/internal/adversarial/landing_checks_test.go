package adversarial

// TEST-04 hand-verification (issue #910, RH-01/F1/F9/F12): TestDeclaredTiersHold
// used to treat "preserve" as "import returned no error and a non-nil
// record" — an empty Record satisfies that, so a parser regression that
// silently discards the fixture's data passed unnoticed. "bound" was an
// unfailable no-op (comment only), and "warn" only counted a diagnostic
// without checking the surviving data or the documented loss.
//
// These three maps are the fix: every preserve/warn/bound fixture must have
// an entry here that asserts the specific data the manifest claims survives.
// TestDeclaredTiersHold requires an entry for every fixture at that tier —
// a fixture with tier preserve/warn/bound and no entry fails the harness,
// so a newly added fixture can't ship silently unasserted.

import (
	"strings"
	"testing"

	"mycorrhizal/contactmodel"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addrComponentValue returns the value of the first AddressComponent with
// the given kind, and whether one was found.
func addrComponentValue(comps []contactmodel.AddressComponent, kind string) (string, bool) {
	for _, c := range comps {
		if c.Kind == kind {
			return c.Value, true
		}
	}
	return "", false
}

// nameComponentValue returns the value of the first NameComponent with the
// given kind, and whether one was found.
func nameComponentValue(comps []contactmodel.NameComponent, kind string) (string, bool) {
	for _, c := range comps {
		if c.Kind == kind {
			return c.Value, true
		}
	}
	return "", false
}

// preserveLandingChecks: fixture name -> assertion that the data the
// manifest declares actually landed on the imported Record. Every
// "preserve"-tier row in Manifest must have an entry.
var preserveLandingChecks = map[string]func(t *testing.T, rec *contactmodel.Record){
	// structural
	"str-missing-version.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
	},
	"str-bad-folding.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full, "FN folded across two lines must unfold to one value")
	},
	"str-mismatched-begin.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
		assert.Equal(t, "str-mismatched-begin", rec.Card.UID)
	},
	"str-crlf-mixed.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
		assert.Equal(t, "str-crlf-mixed", rec.Card.UID, "CR must not survive into a value pulled from a CRLF-terminated line")
	},
	"str-bom.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
	},
	"str-extra-end.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
	},
	"str-empty-card.vcf": func(t *testing.T, rec *contactmodel.Record) {
		assert.Nil(t, rec.Card.Name, "BEGIN/END-only card must produce a genuinely empty record, not a partial one with fabricated data")
		assert.Empty(t, rec.Card.UID)
	},
	"str-note-ends-marker.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "see END:VCARD inside this note value", rec.Card.Notes[0].Note)
	},

	// encoding
	"enc-invalid-utf8.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada \xff Lovelace", rec.Card.Name.Full, "invalid UTF-8 byte must survive unmodified at parse (sanitized only at the flat layer)")
		given, ok := nameComponentValue(rec.Card.Name.Components, "given")
		require.True(t, ok)
		assert.Equal(t, "Ada\xff", given)
	},
	"enc-overlong-utf8.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada \xc0\xaf\xe0\x80\xaf\xf0\x80\x80\xaf", rec.Card.Name.Full, "overlong UTF-8 sequences must be carried through as raw bytes")
	},
	"enc-combining-marks.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Café (unnormalized combining e-acute)", rec.Card.Name.Full, "combining mark must survive unnormalized")
	},
	"enc-rtl-override.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "‮gnpL‮Ada", rec.Card.Name.Full, "RTL override characters must be preserved, not stripped")
		given, ok := nameComponentValue(rec.Card.Name.Components, "given")
		require.True(t, ok)
		assert.Equal(t, "Ada‮", given)
	},
	"enc-zero-width.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada‍Love", rec.Card.Name.Full, "zero-width joiner must survive")
		surname, ok := nameComponentValue(rec.Card.Name.Components, "surname")
		require.True(t, ok)
		assert.Equal(t, "Love‌lace", surname, "zero-width non-joiner must survive")
	},
	"enc-emoji-everywhere.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada 😀 Lovelace 🚀", rec.Card.Name.Full)
		given, ok := nameComponentValue(rec.Card.Name.Components, "given")
		require.True(t, ok)
		assert.Equal(t, "Ada 😀", given)
		require.Len(t, rec.Card.Emails, 1)
		assert.Equal(t, "ada😀@example.com", rec.Card.Emails[0].Address)
		require.Len(t, rec.Card.Phones, 1)
		assert.Equal(t, "+1😀5551234567", rec.Card.Phones[0].Number)
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "🎉 emoji in the note too", rec.Card.Notes[0].Note)
	},

	// semantic
	"sem-duplicate-uid.vcf": func(t *testing.T, rec *contactmodel.Record) {
		// ImportVCardBlock (the adapter-level single-card entry point this
		// harness drives) decodes only the first BEGIN..END block of a
		// multi-card blob; the full-file bounded-parse guarantee that both
		// records survive is services.TestParseVCF_DuplicateUIDWithinFile_BothParse.
		assert.Equal(t, "11111111-1111-1111-1111-111111111111", rec.Card.UID)
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Alice Anderson", rec.Card.Name.Full)
		require.Len(t, rec.Card.Emails, 1)
		assert.Equal(t, "alice@example.com", rec.Card.Emails[0].Address)
	},
	"sem-self-referential-related.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.RelatedTo, 2)
		assert.Equal(t, "sem-self-related", rec.Card.RelatedTo[0].Target, "self-referential RELATED must preserve its own UID as the target")
		assert.Equal(t, []string{"friend"}, rec.Card.RelatedTo[0].Relations)
		assert.Equal(t, "urn:uuid:other-contact", rec.Card.RelatedTo[1].Target)
		assert.Equal(t, []string{"child"}, rec.Card.RelatedTo[1].Relations)
	},
	"sem-date-zero.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Anniversaries, 2)
		birth := rec.Card.Anniversaries[0]
		assert.Equal(t, "birth", birth.Kind)
		require.NotNil(t, birth.Date.Partial, "0000-00-00 must land as a partial date, not be dropped")
		require.NotNil(t, birth.Date.Partial.Year)
		require.NotNil(t, birth.Date.Partial.Month)
		require.NotNil(t, birth.Date.Partial.Day)
		assert.Equal(t, 0, *birth.Date.Partial.Year)
		assert.Equal(t, 0, *birth.Date.Partial.Month)
		assert.Equal(t, 0, *birth.Date.Partial.Day)
		wedding := rec.Card.Anniversaries[1]
		assert.Equal(t, "wedding", wedding.Kind)
		require.NotNil(t, wedding.Date.Partial)
		require.NotNil(t, wedding.Date.Partial.Year)
		assert.Equal(t, 0, *wedding.Date.Partial.Year)
	},
	"sem-date-max.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Anniversaries, 2)
		birth := rec.Card.Anniversaries[0]
		assert.Equal(t, "birth", birth.Kind)
		require.NotNil(t, birth.Date.Partial)
		require.NotNil(t, birth.Date.Partial.Year)
		require.NotNil(t, birth.Date.Partial.Month)
		require.NotNil(t, birth.Date.Partial.Day)
		assert.Equal(t, 9999, *birth.Date.Partial.Year)
		assert.Equal(t, 12, *birth.Date.Partial.Month)
		assert.Equal(t, 31, *birth.Date.Partial.Day)
		wedding := rec.Card.Anniversaries[1]
		assert.Equal(t, "wedding", wedding.Kind)
		require.NotNil(t, wedding.Date.Partial)
		require.NotNil(t, wedding.Date.Partial.Year)
		assert.Equal(t, 9999, *wedding.Date.Partial.Year)
	},
	"sem-timestamp-no-tz.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Updated, "REV must land")
		assert.Equal(t, "20260801T120000", rec.Card.Updated.UTC, "timezone-less REV must be preserved verbatim, not coerced")
		require.NotNil(t, rec.Card.Created, "CREATED must land")
		assert.Equal(t, "20260801T120000", rec.Card.Created.UTC)
	},
	"sem-negative-absurd.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.PersonalInfo, 3)
		expertise := rec.Card.PersonalInfo[0]
		assert.Equal(t, "expertise", expertise.Kind)
		assert.Equal(t, "-5 years of chemistry", expertise.Value)
		assert.Equal(t, "low", expertise.Level)
		require.NotNil(t, expertise.ListAs)
		assert.Equal(t, -7, *expertise.ListAs, "negative INDEX must survive, not be clamped or dropped")

		hobby := rec.Card.PersonalInfo[1]
		assert.Equal(t, "hobby", hobby.Kind)
		assert.Equal(t, "reading", hobby.Value)
		assert.Equal(t, "high", hobby.Level)
		require.NotNil(t, hobby.ListAs)
		assert.Equal(t, 999999999, *hobby.ListAs, "absurdly large INDEX must survive, not overflow or be dropped")

		interest := rec.Card.PersonalInfo[2]
		assert.Equal(t, "interest", interest.Kind)
		assert.Equal(t, "quantum teleportation", interest.Value)
		assert.Equal(t, "medium", interest.Level)

		require.Len(t, rec.Card.Nicknames, 1)
		require.NotNil(t, rec.Card.Nicknames[0].Pref)
		assert.Equal(t, -3, *rec.Card.Nicknames[0].Pref, "negative PREF must survive")
		assert.Equal(t, "Zero", rec.Card.Nicknames[0].Name)

		require.Len(t, rec.Card.Organizations, 1)
		assert.Equal(t, "-Acme Corp", rec.Card.Organizations[0].Name, "leading-hyphen org name must survive")
	},
	"sem-huge-number.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Phones, 1)
		assert.Equal(t, strings.Repeat("1", 1000), rec.Card.Phones[0].Number, "the full 1000-digit number must survive at parse, not be truncated")
	},

	// size
	"size-huge-property.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, strings.Repeat("A", 102400), rec.Card.Notes[0].Note, "the seed NOTE must survive whole before amplification")
	},
	"size-many-properties.vcf": func(t *testing.T, rec *contactmodel.Record) {
		assert.Equal(t, 2000, len(rec.Passthrough.VCard), "every seed X- property must be preserved, none dropped")
	},
	"size-deeply-nested.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.Len(t, rec.Card.Organizations, 1)
		assert.Equal(t, "Acme", rec.Card.Organizations[0].Name)
		require.Len(t, rec.Card.Organizations[0].Units, 500, "all 500 ORG units must survive, none truncated")
		assert.Equal(t, "Unit0", rec.Card.Organizations[0].Units[0].Name)
		assert.Equal(t, "Unit499", rec.Card.Organizations[0].Units[499].Name)

		require.Len(t, rec.Card.Addresses, 1)
		comps := rec.Card.Addresses[0].Components
		require.Len(t, comps, 15, "17-component ADR minus the dropped legacy Ext component minus the unset Direction component")
		for kind, want := range map[string]string{
			"postOfficeBox": "Box 1",
			"name":          "Bldg 11", // modern StreetName (position 12) wins over legacy Street (position 3)
			"locality":      "Locality 4",
			"region":        "Region 5",
			"postcode":      "12345",
			"country":       "Country 6",
			"room":          "Room 7",
			"apartment":     "Apt 8",
			"floor":         "Floor 9",
			"number":        "Num 10",
			"building":      "Block 12",
			"block":         "Sub 13",
			"subdistrict":   "Dist 14",
			"district":      "Landmark 15",
			"landmark":      "Dir 16",
		} {
			got, ok := addrComponentValue(comps, kind)
			assert.Truef(t, ok, "ADR component kind %s did not land", kind)
			assert.Equalf(t, want, got, "ADR component kind %s", kind)
		}
	},

	// injection
	"inj-csv-formula-note.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "=1+1", rec.Card.Name.Full, "leading-= FN must survive into the neutral Card")
		surname, ok := nameComponentValue(rec.Card.Name.Components, "surname")
		require.True(t, ok)
		assert.Equal(t, "+SUM(A1)", surname)
		given2, ok := nameComponentValue(rec.Card.Name.Components, "given2")
		require.True(t, ok)
		assert.Equal(t, "-1", given2)
		credential, ok := nameComponentValue(rec.Card.Name.Components, "credential")
		require.True(t, ok)
		assert.Equal(t, `@HYPERLINK("http://x")`, credential)
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "=cmd|'/c calc'!A1", rec.Card.Notes[0].Note)
		require.Len(t, rec.Card.Nicknames, 1)
		assert.Equal(t, "@EVALUATE(1)", rec.Card.Nicknames[0].Name)
		require.Len(t, rec.Card.Organizations, 1)
		assert.Equal(t, "-Acme Corp", rec.Card.Organizations[0].Name)
	},
	"inj-control-chars.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada\x01Lovelace\x1f", rec.Card.Name.Full, "C0 control chars must survive at parse, stripped only at the flat layer")
		given, ok := nameComponentValue(rec.Card.Name.Components, "given")
		require.True(t, ok)
		assert.Equal(t, "Ada\x02", given)
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "has \x07 bell and \x1b escape", rec.Card.Notes[0].Note)
	},
	"inj-null-byte.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada\x00Lovelace", rec.Card.Name.Full, "NUL byte must survive at parse, stripped only at the flat layer")
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "null \x00 inside", rec.Card.Notes[0].Note)
	},

	// vendor
	"ven-vcard21-bare-params.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
		require.Len(t, rec.Card.Phones, 2, "both bare-token TELs must land")
		numbers := []string{rec.Card.Phones[0].Number, rec.Card.Phones[1].Number}
		assert.Contains(t, numbers, "555-123-4567")
		assert.Contains(t, numbers, "+1-555-555-5555")
		require.Len(t, rec.Card.Emails, 1)
		assert.Equal(t, "ada@example.com", rec.Card.Emails[0].Address)
	},
	"ven-vcard21-quoted-printable.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full, "QP-encoded FN must be decoded")
		require.Len(t, rec.Card.Phones, 1)
		assert.Equal(t, "+1-555-555-5555", rec.Card.Phones[0].Number, "QP-encoded TEL must be decoded")
		require.Len(t, rec.Card.Notes, 1)
		assert.Equal(t, "first line\nsecond line", rec.Card.Notes[0].Note, "QP soft line break must decode to a real line break")
	},
	"ven-x-properties.vcf": func(t *testing.T, rec *contactmodel.Record) {
		wantProps := map[string]bool{"x-vendor-note": false, "x-foo": false, "x-google-custom": false}
		for _, p := range rec.Passthrough.VCard {
			if _, ok := wantProps[strings.ToLower(p.Name)]; ok {
				wantProps[strings.ToLower(p.Name)] = true
			}
		}
		for name, found := range wantProps {
			assert.Truef(t, found, "ven-x-properties.vcf: %s did not land in Passthrough.VCard", name)
		}
		require.NotNil(t, rec.Card.Name)
		given, ok := nameComponentValue(rec.Card.Name.Components, "given")
		require.True(t, ok)
		assert.Equal(t, "Ada", given, "known FN/N data must still land in the neutral model alongside passthrough")
	},
	"ven-apple-grouped.vcf": func(t *testing.T, rec *contactmodel.Record) {
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
	},
	"ven-cr-only.vcf": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
		assert.Equal(t, "ven-cr-only", rec.Card.UID)
	},

	// JSContact
	"js-bom.json": func(t *testing.T, rec *contactmodel.Record) {
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
	},
	"js-unknown-top-level.json": func(t *testing.T, rec *contactmodel.Record) {
		for _, key := range []string{"/xCustomThing", "/extensions"} {
			_, ok := rec.Passthrough.JSContact[key]
			assert.Truef(t, ok, "js-unknown-top-level.json: %s did not land in Passthrough.JSContact", key)
		}
		require.NotNil(t, rec.Card.Name)
		assert.Equal(t, "Ada Lovelace", rec.Card.Name.Full)
	},
}

// warnLandingChecks: fixture name -> assertion of BOTH halves of the warn
// contract — the documented loss (the diagnostic) and the surviving data
// (everything else the fixture carries). Every "warn"-tier row in Manifest
// must have an entry.
var warnLandingChecks = map[string]func(t *testing.T, rec *contactmodel.Record, diags []contactmodel.Diagnostic){
	"sem-adr-ext-component.vcf": func(t *testing.T, rec *contactmodel.Record, diags []contactmodel.Diagnostic) {
		found := false
		for _, d := range diags {
			if d.Severity == "warn" && strings.Contains(d.Message, "extended-address") {
				found = true
			}
		}
		assert.True(t, found, "must emit a warn diagnostic naming the dropped extended-address component")

		require.Len(t, rec.Card.Addresses, 1)
		comps := rec.Card.Addresses[0].Components
		for _, c := range comps {
			assert.NotEqual(t, "Suite 100", c.Value, "the dropped legacy extended-address value must not land anywhere")
		}
		for kind, want := range map[string]string{
			"name":     "1 Main St",
			"locality": "Springfield",
			"region":   "IL",
			"postcode": "12345",
			"country":  "USA",
		} {
			got, ok := addrComponentValue(comps, kind)
			assert.Truef(t, ok, "rest-of-ADR component kind %s did not land", kind)
			assert.Equalf(t, want, got, "rest-of-ADR component kind %s", kind)
		}
	},
}

// boundFixtureCoverage: fixture name -> the dedicated services-level test(s)
// that assert its bounded-failure guarantee (single-card tier assertions
// don't apply to multi-record fixtures, so the coverage lives one layer up,
// in services/adversarial_import_test.go). Every "bound"-tier row in
// Manifest must have an entry, so a new bound fixture added without a
// companion test fails here instead of silently passing.
var boundFixtureCoverage = map[string]string{
	"multi-malformed-middle.vcf": "services.TestParseVCF_BoundedFailure_MalformedMiddle",
	"multi-truncated-middle.vcf": "services.TestParseVCF_BoundedFailure_TruncatedMiddle",
}
