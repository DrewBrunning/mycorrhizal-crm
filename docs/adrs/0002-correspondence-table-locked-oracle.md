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
