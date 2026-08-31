package adversarial

// Manifest is the binding tier table, mirrored by
// docs/adversarial-fixtures/MANIFEST.md. The drift test (adversarial_test.go)
// proves the two never disagree and that every entry resolves to a real
// fixture file. Every row is the ADR-0002 degradation policy tested, not
// re-opened: preserve by default, warn when mappable data fails to land,
// error only for input that is not a valid instance of the format at all.
var Manifest = []Fixture{
	// structural — vCard
	{Name: "str-truncated.vcf", Category: "structural", Format: "vcard", Tier: "error", Note: "missing END:VCARD; go-vcard rejects (no END field found)"},
	{Name: "str-missing-version.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "no VERSION; defaults to 4.0 routing, FN/N land"},
	{Name: "str-bad-folding.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "FN folded mid-property; unfolded to Ada Lovelace"},
	{Name: "str-mismatched-begin.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "lowercase BEGIN:vCard/end:vcard; tokens case-insensitive"},
	{Name: "str-crlf-mixed.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "CRLF/LF mixture; line endings normalized"},
	{Name: "str-bom.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "UTF-8 BOM prefix; stripped, not rejected"},
	{Name: "str-extra-end.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "duplicate END:VCARD; first terminator wins"},
	{Name: "str-empty-card.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "BEGIN/END only; empty record (downstream First-name-required)"},
	{Name: "str-garbage-only.vcf", Category: "structural", Format: "vcard", Tier: "error", Note: "no vCard framing; VCF file contains no valid vCards"},
	{Name: "str-note-ends-marker.vcf", Category: "structural", Format: "vcard", Tier: "preserve", Note: "NOTE value contains literal END:VCARD; splitter must not truncate (services-level assertion)"},

	// encoding — vCard
	{Name: "enc-invalid-utf8.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "invalid UTF-8 in FN/N; sanitized at flat layer"},
	{Name: "enc-overlong-utf8.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "overlong UTF-8 sequences carried through as bytes"},
	{Name: "enc-combining-marks.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "unnormalized combining marks preserved verbatim"},
	{Name: "enc-rtl-override.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "RTL override (U+202E) preserved"},
	{Name: "enc-zero-width.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "zero-width chars (U+200B/U+200D) preserved"},
	{Name: "enc-emoji-everywhere.vcf", Category: "encoding", Format: "vcard", Tier: "preserve", Note: "emoji in FN/N/EMAIL/TEL/NOTE preserved"},

	// semantic — vCard
	{Name: "sem-duplicate-uid.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "two cards one UID; parse ok, create-time collision graceful (import_duplicate_uid_test.go)"},
	{Name: "sem-self-referential-related.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "RELATED to own UID; no recursion, preserved"},
	{Name: "sem-date-zero.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "BDAY 0000-00-00 preserved"},
	{Name: "sem-date-max.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "BDAY 9999-12-31 preserved"},
	{Name: "sem-timestamp-no-tz.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "timezone-less REV/CREATED preserved verbatim"},
	{Name: "sem-negative-absurd.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "negative/absurd INDEX/PREF/LEVEL and -org preserved"},
	{Name: "sem-huge-number.vcf", Category: "semantic", Format: "vcard", Tier: "preserve", Note: "1000-digit phone preserved"},
	{Name: "sem-adr-ext-component.vcf", Category: "semantic", Format: "vcard", Tier: "warn", Note: "ADR legacy extended-address component dropped with warn; rest of ADR lands"},

	// size — vCard
	{Name: "size-huge-property.vcf", Category: "size", Format: "vcard", Tier: "preserve", Note: "one ~100KB NOTE; harness amplifies to megabytes (bounded by MaxVCFSize, #415)"},
	{Name: "size-many-properties.vcf", Category: "size", Format: "vcard", Tier: "preserve", Note: "2000 X- props; harness amplifies; no per-property cap"},
	{Name: "size-deeply-nested.vcf", Category: "size", Format: "vcard", Tier: "preserve", Note: "500-unit ORG + 18-component ADR; line-based, no recursion risk"},

	// injection — vCard
	{Name: "inj-csv-formula-note.vcf", Category: "injection", Format: "vcard", Tier: "preserve", Note: "CSV formula prefixes imported intact so export csvSafe neutralizes them"},
	{Name: "inj-control-chars.vcf", Category: "injection", Format: "vcard", Tier: "preserve", Note: "C0/C1 control chars preserved at parse, stripped at flat layer"},
	{Name: "inj-null-byte.vcf", Category: "injection", Format: "vcard", Tier: "preserve", Note: "NUL byte preserved at parse, stripped at flat layer"},

	// vendor — vCard
	{Name: "ven-vcard21-bare-params.vcf", Category: "vendor", Format: "vcard", Tier: "preserve", Note: "vCard 2.1 bare-token params normalized then parsed"},
	{Name: "ven-vcard21-quoted-printable.vcf", Category: "vendor", Format: "vcard", Tier: "preserve", Note: "vCard 2.1 QUOTED-PRINTABLE decoded by normalizeVCard21"},
	{Name: "ven-x-properties.vcf", Category: "vendor", Format: "vcard", Tier: "preserve", Note: "unknown X- props with params preserved via Passthrough.VCard and round-tripped"},
	{Name: "ven-apple-grouped.vcf", Category: "vendor", Format: "vcard", Tier: "preserve", Note: "Apple itemN.EMAIL/X-ABLabel grouped properties; EMAIL/TEL land, X-ABLabel passthrough"},
	{Name: "ven-cr-only.vcf", Category: "vendor", Format: "vcard", Tier: "preserve", Note: "CR-only line endings normalized"},

	// JSContact
	{Name: "js-bom.json", Category: "structural", Format: "jscontact", Tier: "preserve", Note: "UTF-8 BOM prefix stripped, not rejected"},
	{Name: "js-trailing-garbage.json", Category: "structural", Format: "jscontact", Tier: "error", Note: "valid card + trailing non-JSON; file not valid JSON"},
	{Name: "js-unknown-top-level.json", Category: "semantic", Format: "jscontact", Tier: "preserve", Note: "unknown top-level keys preserved via Passthrough.JSContact"},
	{Name: "js-absurd-types.json", Category: "semantic", Format: "jscontact", Tier: "error", Note: "typed fields with wrong JSON types; not a valid Card instance"},
	{Name: "js-null-card.json", Category: "semantic", Format: "jscontact", Tier: "error", Note: "top-level null; not a Card instance"},
	{Name: "js-deeply-nested.json", Category: "size", Format: "jscontact", Tier: "error", Note: "10001-deep nesting exceeds encoding/json maxNestingDepth 10000 (#415)"},

	// multi-record — covered by dedicated bounded-failure tests
	{Name: "multi-malformed-middle.vcf", Category: "multi-record", Format: "vcard", Tier: "bound", Note: "good + invalid-END + good; valid import, broken named skip row"},
	{Name: "multi-truncated-middle.vcf", Category: "multi-record", Format: "vcard", Tier: "bound", Note: "good + truncated + good; truncated named skip row, neighbors import"},
}
