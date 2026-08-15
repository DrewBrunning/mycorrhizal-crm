# ADR 0003: Golden fixtures as the external test oracle

- **Status:** accepted
- **Date:** 2026-08-15

## Context

Each adapter writes its own code *and* its own tests. Self-authored green tests can share a
misconception with the code they test, so "tests green" does not imply "correct." The mapping is locked
against the RFCs (ADR 0002), but something still has to anchor the *bytes* — the verbatim worked
examples that no implementer wrote.

## Decision

Maintain a directory of **golden fixtures** — vCard/JSContact example cards copied **verbatim from the
RFCs** — as the external ground truth for directional tests (import: file → expected neutral fields;
export: expected neutral → these bytes).

- Golden fixtures are **locked**: do not hand-edit a value. The provenance table
  (`docs/golden-fixtures/SOURCES.md`) records each fixture's RFC source and the concept it exercises.
- Adapters copy them into `backend/internal/rfctest/fixtures/` **unchanged**, and tests assert against
  the RFC-verbatim bytes/fields. Fixtures are verified byte-identical — no silent edits to make a test
  pass.
- Concepts introduced or changed by RFC 9553/9554/9555 **must** be RFC-verbatim. Unchanged
  RFC 6350/2426 baseline concepts may use hand-authored, minimal, RFC-syntax-conformant fixtures (there
  is no novel semantic to get wrong), based on the baseline cards by extension, not invention.

## Consequences

- **Green tests ≠ correct.** Acceptance of adapter work requires the golden-fixture tests to pass, and
  the fixtures to remain verbatim.
- The `docs/specs/` RFC transcriptions remain the readable authority behind the fixtures and the
  correspondence table.
