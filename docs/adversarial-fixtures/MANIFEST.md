# Adversarial fixture corpus — manifest

This directory is the **adversarial / merely-broken** complement to `docs/golden-fixtures/`.
Per `docs/adrs/0003-golden-fixtures-external-test-oracle.md` the golden directory holds only
RFC-verbatim examples; malformed and hostile cards live here, so the oracle stays clean.

Every fixture has a **declared expected tier** — the ADR-0002 degradation policy tested, not
re-opened (issue #432, TEST-04):

- **preserve** — the import completes without error. The data the fixture demonstrates lands in a
  neutral field or, for unknown data, in `Record.Passthrough` and is re-emitted on export.
  "Preserve, don't reject" is the default; nothing is silently dropped.
- **warn** — the import completes but emits a `Diagnostic{Severity:"warn"}`: mappable data that
  fails to land. The operation still completes.
- **error** — the import returns an error: the bytes are not a valid instance of the format at all.

The machine-readable copy of this table (the binding one the harness asserts against) is
`backend/internal/adversarial/manifest.go`; the drift test `adversarial_test.go` proves this
document and that table can never disagree, and that every fixture file is byte-identical to its
embedded copy in `backend/internal/adversarial/fixtures/`.

| Fixture | Category | Format | Tier | Notes / what it proves |
|---|---|---|---|---|
| str-truncated.vcf | structural | vCard | error | missing `END:VCARD`; go-vcard rejects ("no END field found") |
| str-missing-version.vcf | structural | vCard | preserve | no `VERSION`; defaults to 4.0 routing, FN/N land |
| str-bad-folding.vcf | structural | vCard | preserve | `FN` folded across two physical lines mid-property; unfolded to "Ada Lovelace" |
| str-mismatched-begin.vcf | structural | vCard | preserve | `BEGIN:vCard`/`end:vcard` lowercase; tokens are case-insensitive |
| str-crlf-mixed.vcf | structural | vCard | preserve | CRLF/LF mixture; line endings normalized |
| str-bom.vcf | structural | vCard | preserve | UTF-8 BOM prefix; stripped, not rejected (see fix in adapters + ParseVCF) |
| str-extra-end.vcf | structural | vCard | preserve | duplicate `END:VCARD`; first terminator wins, trailing line dropped |
| str-empty-card.vcf | structural | vCard | preserve | `BEGIN`/`END` only; parses to an empty record (downstream "First name required") |
| str-garbage-only.vcf | structural | vCard | error | no vCard framing at all; "VCF file contains no valid vCards" |
| str-note-ends-marker.vcf | structural | vCard | preserve | `NOTE` value contains the literal text `END:VCARD`; splitter must not truncate it (regression for the line-based splitter fix) |
| enc-invalid-utf8.vcf | encoding | vCard | preserve | invalid UTF-8 bytes in FN/N; preserved at parse, sanitized at the flat layer (`SanitizeImportedContact`) |
| enc-overlong-utf8.vcf | encoding | vCard | preserve | overlong UTF-8 sequences; carried through as bytes, no crash |
| enc-combining-marks.vcf | encoding | vCard | preserve | unnormalized combining marks; preserved verbatim |
| enc-rtl-override.vcf | encoding | vCard | preserve | RTL override (U+202E) in a field; preserved (inert, frontend renders as text) |
| enc-zero-width.vcf | encoding | vCard | preserve | zero-width characters (U+200B/U+200D); preserved |
| enc-emoji-everywhere.vcf | encoding | vCard | preserve | emoji in FN/N/EMAIL/TEL/NOTE; preserved byte-for-byte |
| sem-duplicate-uid.vcf | semantic | vCard | preserve | two cards, one `UID`; both parse; the create-time collision is handled gracefully (see `import_duplicate_uid_test.go`) |
| sem-self-referential-related.vcf | semantic | vCard | preserve | `RELATED` pointing at the card's own UID; no recursion, preserved |
| sem-date-zero.vcf | semantic | vCard | preserve | `BDAY`/`ANNIVERSARY` `0000-00-00`; preserved as a date value |
| sem-date-max.vcf | semantic | vCard | preserve | `BDAY`/`ANNIVERSARY` `9999-12-31`; preserved as a date value |
| sem-timestamp-no-tz.vcf | semantic | vCard | preserve | timezone-less `REV`/`CREATED`; preserved verbatim (no timezone coercion) |
| sem-negative-absurd.vcf | semantic | vCard | preserve | negative/absurd INDEX/PREF/LEVEL values and a leading-`-` org; preserved, no crash |
| sem-huge-number.vcf | semantic | vCard | preserve | 1000-digit phone number; preserved (validated at the flat layer) |
| sem-adr-ext-component.vcf | semantic | vCard | warn | ADR legacy extended-address component has no neutral kind; dropped with a warn diagnostic, rest of ADR lands |
| size-huge-property.vcf | size | vCard | preserve | one ~100 KB `NOTE`; the harness amplifies it to megabytes at runtime (bounded by `MaxVCFSize` 50 MB, issue #415) |
| size-many-properties.vcf | size | vCard | preserve | 2000 `X-` properties on one card; harness amplifies to tens of thousands at runtime; no per-property cap, bounded by file size |
| size-deeply-nested.vcf | size | vCard | preserve | 500-unit ORG + 18-component ADR; structured values are line-based, no recursion depth risk |
| inj-csv-formula-note.vcf | injection | vCard | preserve | CSV formula prefixes (`=`,`+`,`-`,`@`) in exportable fields; imported intact so export's `csvSafe` can neutralize them (see `export_csv_injection_test.go`) |
| inj-control-chars.vcf | injection | vCard | preserve | C0/C1 control characters in FN/N/NOTE; preserved at parse, stripped at the flat layer |
| inj-null-byte.vcf | injection | vCard | preserve | NUL byte in FN/NOTE; preserved at parse, stripped at the flat layer |
| ven-vcard21-bare-params.vcf | vendor | vCard | preserve | vCard 2.1 bare-token params (`;CELL;PREF:`); normalized (`normalizeVCard21`) then parsed as 3.0 |
| ven-vcard21-quoted-printable.vcf | vendor | vCard | preserve | vCard 2.1 `ENCODING=QUOTED-PRINTABLE`; decoded by `normalizeVCard21` |
| ven-x-properties.vcf | vendor | vCard | preserve | unknown `X-` properties with params; preserved via `Passthrough.VCard` and round-trip to export |
| ven-apple-grouped.vcf | vendor | vCard | preserve | Apple `item1.EMAIL`/`X-ABLabel` grouped properties; EMAIL/TEL land, `X-ABLabel` via passthrough |
| ven-cr-only.vcf | vendor | vCard | preserve | CR-only line endings (legacy Outlook-style); normalized |
| js-bom.json | structural | JSContact | preserve | UTF-8 BOM prefix; stripped, not rejected (fix in `jscontact.Unmarshal`) |
| js-trailing-garbage.json | structural | JSContact | error | valid card + trailing non-JSON bytes; file is not valid JSON |
| js-unknown-top-level.json | semantic | JSContact | preserve | unknown top-level keys (`xCustomThing`, `extensions`); preserved via `Passthrough.JSContact` |
| js-absurd-types.json | semantic | JSContact | error | typed fields with wrong JSON types (`name.full` as a number); not a valid Card instance |
| js-null-card.json | semantic | JSContact | error | top-level `null`; not a Card instance (fix in `jscontact.Unmarshal`) |
| js-deeply-nested.json | size | JSContact | error | 10001-deep nesting exceeds encoding/json's `maxNestingDepth` (10000) — resource exhaustion, issue #415 |
| multi-malformed-middle.vcf | multi-record | vCard | — | bounded-failure: good + invalid-`END` + good; valid records import, the broken one is a named skip row (asserted in `services/adversarial_import_test.go`) |
| multi-truncated-middle.vcf | multi-record | vCard | — | bounded-failure: good + truncated + good; truncated card is a named skip row, neighbors import (regression for the line-based splitter fix) |

## Scope boundaries

- **Resource exhaustion is NOT re-tested here** — the size fixtures are the input corpus for issue
  #415's limits (`MaxVCFSize`/`MaxVCFContacts` at the upload boundary), cross-referenced in
  `docs/security/asvs-l2.md` rows 11.1.4 / 12.1.1 / API4. What the corpus *does* prove is that the
  parsers themselves complete (no panic, no hang, no partial contact) on hostile sizes.
- **Per-record loss reporting** ("which records were affected and why") is DATA-02's job (issue
  #442). This corpus proves the *bound*: a malformed record inside a batch is isolated to a named
  skip row and never aborts the batch. Interstitial garbage between cards is dropped by the
  splitter without a row — naming it is #442's surface.
- **Property-seed input for TEST-07 (#435)**: these fixtures are the realistic starting points the
  property generator's shrinking should land on; they are not themselves generated.
- **Live CardDAV path** is issue #512's separate hostile-input suite
  (`services/contact_sync_hostile_input_test.go`); this corpus feeds the *import* surface.
- **Security rows touched**: see `docs/security/asvs-l2.md` 5.2.2 (sanitize), 5.3.1–5.3.3 (CSV
  formula injection import side), 5.1.4 (duplicate-UID unique index), 11.1.4/12.1.1/API4 (size).
