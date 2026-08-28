# ADR 0002: Correspondence table as the locked mapping oracle

- **Status:** accepted
- **Date:** 2026-08-15
- **Supersedes:** ad-hoc, per-adapter mapping decisions encoded inline in adapter code

## Context

Every adapter maps between the neutral model and a serialized format, and every directional test
asserts an expected value. If each adapter and test authored its own mapping, a shared misconception
would be invisible: the code and its tests would be consistently, confidently wrong — and green.

## Decision

The **correspondence table** (`backend/correspondence/testdata/correspondence.tsv`, loaded by
`backend/correspondence/table.go`) is the **single source of mapping truth**. One row per concept,
with the neutral path, the JSContact pointer, the vCard 4.0 and 3.0 property/params, the value
transform, and the RFC 9555 reference/rule in `notes`.

- **It is locked.** Authored and verified against `docs/specs/rfc9555-correspondence.md` (RFC 9555).
  Implementers MUST NOT add, remove, or alter rows.
- **Missing or ambiguous mapping → escalate, never invent.** The escalation protocol is part of the
  rule so "don't invent a mapping" does not depend on an implementer's self-restraint.
- **No adapter or test may encode a mapping not present in the table.** A reflection test verifies
  every `neutral_path` resolves on `contactmodel.Record`, and a coverage meta-test verifies every
  for-this-format concept has both an import and an export test.

## Degradation policy (bound to this decision)

Two tiers of loss, never a hard failure of import/export:

1. **Mappable data that fails to land** (e.g. a phone number) → a **defect**, caught by a red
   directional test at dev time; at runtime emit a `Diagnostic{Severity:"warn"}`. The operation still
   completes.
2. **Genuinely unmappable/unknown data** → **preserve, don't reject**: unknown vCard properties on
   import go to `Record.Passthrough.VCard`, unknown JSContact properties to `Record.Passthrough.JSContact`
   (re-emitted on export); a neutral field with no target-format home yields a warn diagnostic and is
   dropped from that serialization only.

`error` is returned **only** for input that is not a valid instance of the format at all.

## Consequences

- The RFC specs (`docs/specs/`) are the ground truth; the correspondence table is the working, testable
  materialization of the RFC 9555 mapping.
- Adding a concept is a deliberate, reviewed act against the RFC, not a local code edit.

## Appendix: Canonical-field round-trip audit (issue #515)

Audit of every canonical field on the flat `models.Contact` against its neutral-model home, done 2026-08-28
(issue #515). "Round trip" here means the neutral `Record` (the hub): `Contact -> Record` via
`RecordFromContact` and `Record -> Contact` via `ApplyRecordToContact`. The correspondence table above maps
`Card`/`Passthrough` into serialized formats; the **envelope is deliberately never serialized** ("Format
adapters MUST ignore it entirely", `backend/contactmodel/envelope.go`), so a field whose only home is the
envelope round-trips through the neutral model but is **absent from every vCard/JSContact file export** —
that loss is reported by name (`models.EnvelopeExportLossDiagnostics`) so it is never silent.

| Canonical field | Neutral home | Disposition |
|---|---|---|
| `Firstname` / `Lastname` / `MiddleName` / `Prefix` / `Suffix` | `Card.Name.Components` (`name.*` rows) | lossless |
| `Nickname` | `Card.Nicknames[0]` (`nickname` row) | lossless (first entry; extra entries live only on `Card`) |
| **`Gender`** | **`CRMEnvelope.Gender`** (added 2026-08-28) | **lossless via the neutral Record; lossy-in-file, named.** Free-text CRM concept, deliberately not vCard `GENDER`/JSContact `speakToAs` (see `docs/specs/rfc6350-baseline.md` closing note) — was previously the canary for "no home at all". |
| `Email` / `Emails[]` | `Card.Emails` (`email` row) | lossless |
| `Phone` / `Phones[]` | `Card.Phones` (`phone` row) | lossless |
| `Birthday` | `Card.Anniversaries[kind=birth]` (`anniversary.birth` row) | lossless for validator-shaped strings; other legacy strings stay on the flat scalar only |
| `Anniversary` | `Card.Anniversaries[kind=wedding]` (`anniversary.wedding` row) | lossless |
| `Photo` / `PhotoThumbnail` | `Card.Media[kind=photo]` (`photo` row) | lossless |
| `Address` / `Addresses[]` | `Card.Addresses` (`adr` row) | lossless for structured entries; a `Full`-only entry (flattened legacy scalar) has no structured recovery — documented lossy in `applyAddresses` |
| `HowWeMet` | `CRMEnvelope.HowWeMet` | lossy-in-file, named |
| `WorkInformation` | `CRMEnvelope.WorkInformation` | lossy-in-file, named |
| `ContactInformation` | `CRMEnvelope.ContactInformation` | lossy-in-file, named |
| `Circles` | `CRMEnvelope.Circles` | lossy-in-file, named (also superseded as a *data source* by `circle_members`, T2/T3 — the column is legacy) |
| `Organization` / `Department` | `Card.Organizations` (`org` / `org.unit` rows) | lossless (first org + first unit; extra entries live only on `Card`) |
| `JobTitle` / `Role` | `Card.Titles` (`title` / `role` rows) | lossless |
| `URLs[]` | `Card.Links` (`link` row) | lossless for URI; `Label`/TYPE fidelity is the adapter's own contract |
| `IMPPs[]` | `Card.ImppAddresses` (`impp` row) | lossless |
| `VCardExtra` | `Passthrough.VCard` (`pt.vcard` row) | best-effort, lossless in `Passthrough` (not re-serialized back into the legacy column — documented) |
| `FN` / `Org` | derived, not stored | derived from `Card` via `DeriveProjection` on every save |
| `Archived` | none | CRM-local flag; deliberately never exported (same standing as `IsFavorite`) |
| `IsFavorite` | none | CRM-local flag; deliberately never exported (issue #173) |
| `Notes` (table) | none | **deliberately not projected into `Card.Notes`**: relational timeline entity like `Activities`/`Reminders` (separate table keyed by contact), not a card field; `Card.Notes` stays reserved for imported vCard `NOTE` content |
| `Activities` / `Reminders` (tables) | none | separate relational entities, keyed by contact ID — never embedded in the single-contact `Record` (documented in `CRMEnvelope`) |

Rules this audit applied (and enforces going forward):

1. **A canonical field must have a `Card` home (standards round trip) or a `CRMEnvelope` home (neutral-record
   round trip).** Neither is optional for user-authored content.
2. **`Passthrough` is not a home for a canonical field.** It is reserved for spec-blessed escape hatches
   (unknown vCard/JSContact properties) — a canonical field routed there would be misrepresented as
   standards-originated. `VCardExtra` is the legacy exception, grandfathered.
3. **A field whose only home is the envelope is a named loss on file export**, never a silent drop
   (`EnvelopeExportLossDiagnostics`), matching the degradation policy above.
4. **CRM-local flags (`Archived`, `IsFavorite`) and relational tables (`Notes`, `Activities`, `Reminders`)
   are explicitly out of scope for the standards surface** and are listed so the exclusion is a written
   decision, not an omission.

