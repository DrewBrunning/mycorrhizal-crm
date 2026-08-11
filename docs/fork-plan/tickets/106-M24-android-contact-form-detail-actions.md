# M24 — Contact form & detail-page actions on Android

| | |
|---|---|
| **Rating** | 4 — delete/archive missing from the detail page is a real gap, not polish |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

This is distinct from [M7](88-M7-android-contact-record-coverage.md) — M7 is the depth gap *inside*
the `Card`/`CRMEnvelope` record (addresses, orgs, links, etc.); this ticket is the set of top-level
actions and form fields around the record that M7 explicitly excluded.

## Scope

**Contact form** (`ContactFormViewModel.ContactFormState` vs. `AddContactDialog.tsx`):
- Prefix / middle name / suffix fields — not in `ContactFormState` at all.
- Kind (human/animal) — new Android contacts always default to the backend's default kind.
- Language field.
- Circles: autocomplete of existing circles, not a free-text comma-separated field
  (`ContactFormScreen.kt:195-201`).
- Tags field — entirely absent from the Android create/edit form (only reachable in reverse, from a
  tag's own detail screen).

**Contact detail page top-level actions** — none of these exist at the `ContactRepository`
interface level today, not just missing UI:
- **Delete contact**, with confirmation.
- **Archive / unarchive contact**, with confirmation.
- Export contact (vCard 4.0 / vCard 3.0 / JSContact download).
- "Stay in touch" one-tap quick action (pre-fills a recurring reminder — distinct from manually
  creating one via the Reminders screen).
- Profile-picture upload (with crop; web additionally offers Immich linking — Immich itself was
  marked deliberately-not-on-mobile at M8 sign-off, so a plain upload/crop path is sufficient here,
  no Immich picker needed).
- Inline circle chip editor on the detail page (add/remove without going through the full edit
  form).
- Inline tag chip editor on the detail page (currently tags aren't even displayed read-only on
  Android's contact detail screen).
- Share-contact entry point (wires into [M15](97-M15-android-contact-sharing.md) once that lands —
  this ticket can add the menu item as a stub pointing at it, or land after M15, whichever is more
  convenient at implementation time).
- View-prep entry point (wires into [M11](93-M11-android-prep-view.md) similarly).

## Done when

- Delete and archive/unarchive both work from the contact detail screen, with confirmation,
  matching web's semantics (soft-delete per `/CLAUDE.md`'s delete-semantics rules — this is calling
  the existing backend endpoint, not inventing new delete behavior).
- Export produces a file matching one of web's three formats.
- Circles and tags are both visible and editable inline from the detail page, not only via the full
  edit form.
- Hand-verified on-device: archive a contact, confirm it's excluded from the default list; delete a
  contact, confirm the standard soft-delete/undo story holds (check against T60's audit undo once
  M16 lands, or against the web audit page in the meantime).
- New strings translated in all five locales.
