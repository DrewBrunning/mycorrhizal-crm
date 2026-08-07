# T52 — Simplify the contact-add flow to name + contact fields; defer everything else to edit

| | |
|---|---|
| **Rating** | 3 — real UX and maintenance win, not urgent |
| **Size** | M |
| **Depends on** | — |
| **Alpha** | n/a — real data exists; frontend-only reduction in scope, no schema/API contract change required (the full create endpoint can stay as-is; the dialog just stops exercising most of it) |
| **Source** | v0.3.0 post-release testing, 2026-08-06 |

## Why this exists

`AddContactDialog.tsx` currently renders the same breadth of fields as the full contact edit
surface (36 distinct field/input elements at last count). Every field added to the contact model
going forward has to be wired into two places that both need to stay in sync — the add dialog and
the post-creation edit surface — which is real, avoidable duplication: a contact's full field set is
already reachable immediately after creation via the normal edit flow, so the add dialog doesn't
need to be a complete form to be useful.

## What to build

- Reduce `AddContactDialog` to name + the core contact fields (email/phone at minimum — confirm the
  exact bare-minimum set with the person who'll use it; "Name and contact fields" per the report).
  Everything else the current dialog exposes moves to being editable immediately after creation,
  which the app already supports.
- After a bare-bones create, land the user on the new contact's detail page (or otherwise make it
  obvious that more fields are one click away) rather than leaving them to discover editing on their
  own.
- Audit whatever validation/duplicate-detection logic currently lives duplicated between the add
  dialog and the edit path — this is the concrete maintenance payoff the ticket exists for, so it's
  worth actually removing the duplicate implementation, not just visually hiding fields while
  keeping two parallel validation code paths alive underneath.

## Traps

- Don't remove fields from the *edit* surface — this is scoped to what's asked for at creation time
  only. The full field set must remain exactly as reachable as it is today, just via edit instead of
  create.
- Check whether any e2e specs assert on the full add-dialog field set (`e2e/contacts.spec.ts` and
  similar) — those will need updating to match the reduced flow, not just the dialog's own component
  tests.
- If duplicate-detection (matching against existing contacts) currently runs off fields this ticket
  removes from the add flow, confirm it still has enough signal post-simplification, or explicitly
  decide that duplicate detection now only runs at edit time.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green (updated `AddContactDialog` tests reflecting the
  reduced field set).
- e2e specs touching contact creation pass with the simplified flow.
- Hand-verified: create a contact through the simplified dialog, confirm every field the old dialog
  exposed is still reachable and editable immediately afterward.
- All 5 locale files have real translations for any changed strings.
