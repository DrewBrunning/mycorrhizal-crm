# M16 — Audit trail + undo (T60) on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — T60/T18's backend already exists and serves web today |
| **Status** | TO BE DONE |

`/audit` (`AuditPage.tsx`) has zero Android footprint — a repo-wide search for "audit" in
`android/**/*.kt` returns no hits.

## Scope (mirrors `AuditPage.tsx`)

- Reverse-chronological event list, paginated ("load more").
- Filter by entity type.
- Filter by entity ID (debounced).
- Clear filters.
- Undo a contact-update event.
- Navigate from an audit row to the linked contact.

## Done when

- All actions above work on Android against the same T18 event log web reads.
- Undo behaves identically to web's (same confirmation gate, same result).
- Hand-verified on-device: make a change, undo it via the Android audit screen, confirm the change
  reverted.
- New strings translated in all five locales.
