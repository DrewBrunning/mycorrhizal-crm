# M11 — Prep view (N2) for Android

| | |
|---|---|
| **Rating** | 5 — N2 is a rating-5 capability on web; this is porting it, not a new design |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — the backend N2 endpoint(s) already exist and serve web today |
| **Status** | TO BE DONE |

`/contacts/:id/prep` (`PrepViewPage.tsx`) has **zero** Android footprint — not even a placeholder
route in `MycorrhizalApp.kt`'s nav graph. It's the single largest capability gap this audit found:
the "what do I need to know before I talk to this person" briefing is exactly the kind of thing
someone reaches for on their phone right before a call or a visit, more than at a desk.

## Scope (mirrors `PrepViewPage.tsx:120-326`)

- Cadence health card: overdue-by / on-track / next-due / last-interaction.
- Open agenda items list (Conversation Agenda entries not yet marked discussed).
- Last interaction + recent notes.
- Relationships list, click-through to the other party's contact record.
- Life events list.
- Upcoming reminders list.
- Upcoming dates (birthday/anniversary, "in N days").
- Entry point: add to `ContactDetailScreen`'s header/menu (web reaches it from
  `ContactHeader.tsx:369,486`) and to the nav route `contacts/{contactId}/prep`.

## Done when

- All seven sections above present and populated from the real backend data (not stubbed).
- Reachable from the contact detail screen the same way web reaches it.
- Relationship/life-event entries navigate to their target contact, matching web.
- Hand-verified on-device against a contact with cadence, agenda items, notes, relationships, life
  events, and reminders all populated, per `/CLAUDE.md`'s workflow section.
- New strings translated in all five locales.
