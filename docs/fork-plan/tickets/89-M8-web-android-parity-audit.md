# M8 — Web ↔ Android parity audit: a screen-by-screen matrix, then tickets from its gaps

| | |
|---|---|
| **Rating** | 4 — not a feature; the instrument that tells us which features are missing |
| **Source** | Post-M1 review pass, 2026-08-11. M1's landing note reports Phases 1–5 complete without ever stating what "complete" was measured against. |
| **Depends on** | Nothing. This is inspection, and its output is tickets. |
| **Status** | Proposed — the method below needs a yes before anyone spends a day on it. |

This is the **breadth** gap: whole web screens with no Android equivalent. The **depth** gap inside
the contact record is [M7](88-M7-android-contact-record-coverage.md).

## Why this exists

M1 was built phase by phase against its own design document, not against the web app. Each phase's
landing note says what it built; none says what it *didn't*. The result is that "Phases 1–5 core
DONE" is true against the design and materially untrue against the product — four drawer routes are
literal `PlaceholderScreen` stubs, and at least seven web pages have no Android counterpart at all.

Nobody currently knows the size of the gap. That is the actual problem: not any one missing screen,
but that the answer to "can I do X on my phone?" requires reading source. This ticket produces the
artifact that answers it, and stops the answer going stale.

## What already turned up (unaudited, from a 20-minute pass)

Enough to show the exercise is worth doing. **This is a starting point, not the finding** — the
audit's job is to make it complete and to go a level below routes into per-screen actions.

| Web route | Android | Note |
|---|---|---|
| `/` DashboardPage | ✅ `DashboardScreen` | Costs 3 requests on Android; [M3](82-M3-dashboard-overview-endpoint.md) fixes |
| `/contacts` | ✅ `ContactListScreen` | |
| `/contacts/:id` | ⚠️ partial | See [M7](88-M7-android-contact-record-coverage.md) |
| `/contacts/:id/prep` PrepViewPage | ❌ **none** | N2 prep view — a rating-5 capability, absent |
| `/notes` NotesPage | ⚠️ **placeholder** | `NotesScreen` exists, reachable only per-contact |
| `/activities` ActivitiesPage | ⚠️ **placeholder** | `ActivitiesScreen` exists, reachable only per-contact |
| `/search` SearchPage | ⚠️ **placeholder** | Only the contact-list search bar exists |
| `/network` NetworkPage | ⚠️ **placeholder** | The T10 relationship **graph** — no Android equivalent |
| `/households` | ✅ | |
| `/settings` | ⚠️ partial | Language/date-format read-only ([M6](85-M6-photo-url-user-prefs-oidc.md) §3) |
| `/settings/data` DataSettingsPage | ❌ **none** | Import/export; the API client shipped, the UI didn't |
| `/shares` ContactSharesPage | ❌ **none** | P1 contact sharing |
| `/audit` AuditPage | ❌ **none** | T60 audit trail + undo |
| `/users` UsersPage | ❌ **none** | Admin user management |
| `/circle-tag-triage` | ❌ **none** | T2 triage |
| `/register` RegisterPage | ❌ **none** | Android is sign-in only |
| `/reminders` | — | **Dead route on web.** Renders `<div>{t('pages.reminders')}</div>` → the literal text "Reminders Page", and *nothing links to it* — its `<Route>` is the only reference in the whole frontend. Leftover fork scaffolding, not an unfinished feature. Both platforms do reminders per-contact; neither has a global reminders list. **Delete the route and the one-key `pages` namespace from all five locales** rather than porting it. |

Three things that table already teaches. First, the notes/activities gap is a **wiring** job, not a
feature build — the screens exist and work. Second, parity is not one-directional: native call/SMS
tracking, device-contacts import and quick capture have no web equivalent by design, so the audit
must record both directions or it will quietly turn into "make Android a worse web app."

Third — and this is the one that justifies pass 2 — **a route existing on web does not mean the
feature exists there.** `/reminders` is a live route that renders the literal string "Reminders
Page" and is linked from nowhere. A route-level diff (pass 1) would have scored it as "web has it,
Android doesn't" and generated a ticket to port a feature that was never built. Only opening the
page source catches that. Expect more of these: this repo is a hard fork of meerkat-crm, so
inherited scaffolding is a live hazard for exactly this kind of audit.

## The proposed method

Four passes, coarse to fine. Stop after pass 2 if the gap turns out to be small — the point is a
decision, not a document.

**Pass 1 — routes.** Enumerate `frontend/src/App.tsx`'s `<Route>` list against
`MycorrhizalApp.kt`'s `composable(...)` list. Mechanical; the table above is a first draft.

**Pass 2 — activities within each screen.** The real work, and where "activity-by-activity" bites.
For each web page, enumerate every *user-initiated action* — every button, menu item, dialog,
inline edit, bulk action, filter, sort, keyboard shortcut — and mark each `native` / `missing` /
`n/a-on-mobile` / `android-only`. A screen that exists on both platforms can still be missing half
its verbs; `/contacts` is the obvious candidate (bulk select, archive, merge, export, tag, filter).
Derive the list from the page source rather than by clicking, so it is reproducible and reviewable.

**Pass 3 — classify each gap.** Four buckets, and the third and fourth are the valuable ones:
- **Wire it up** — the Android implementation exists and just isn't reachable (notes, activities).
- **Build it** — genuinely absent, and wanted on mobile.
- **Deliberately not on mobile** — admin user management, circle/tag triage and registration are
  plausible desktop-only calls. Recording the *decision* is the deliverable; an unrecorded
  omission is indistinguishable from an oversight, which is exactly the state we are in now.
- **Android-only** — call/SMS tracking, device-contacts import, quick capture. Feeds a reverse
  question: should any of it reach the web?

**Pass 4 — file tickets.** One per "build it" cluster, ranked normally. The matrix itself lands in
this ticket's landing note; a matrix nobody converts to tickets is a document, not a plan.

## Where the matrix should live

In this ticket's landing note, not a new top-level doc — `docs/fork-plan/tickets/README.md` is the
status board and `95` is the grooming journal, and neither wants a 200-row table. It is a
point-in-time measurement, valuable for the tickets it generates and then superseded by them.

**Do not add a CI check that diffs the two route lists.** It would be red permanently by design
(Android is legitimately a subset), so it would be ignored within a week and then trusted by nobody.

## Open question for the sign-off

**What is the actual target?** "100% of web functionality implemented natively" is the brief, and
it is worth pressure-testing before committing:

- Some web surfaces are plainly not mobile work — `/users` admin management, `/circle-tag-triage`,
  `/register`.
- Some are large builds for arguable mobile value — `/network`'s graph visualization is the clearest
  case: a force-directed graph on a 6" screen is a real design problem, not a port.
- Meanwhile Android already leads in places (native tracking, device import, quick capture), which a
  strict "match web" target would not capture at all — and `/reminders` shows the target can point
  at work that shouldn't happen, since porting it would mean building a page web never had.

A more defensible target is **"every gap is either closed or recorded as a deliberate decision"**,
with 100% as the default and each exception written down with its reason. That is what this ticket
assumes. Say so explicitly if the literal 100% is wanted instead — it changes pass 3 from a
judgement exercise into a work queue, and roughly doubles the resulting ticket count.

## Done when

- Pass 1 and 2 complete: a matrix covering every web route and every user-initiated action on it,
  with an Android verdict per row and both directions represented.
- Every gap classified into one of the four buckets, with a recorded reason for each
  "deliberately not on mobile".
- Tickets filed for the "build it" clusters, ranked on the board.
- The matrix in this ticket's landing note, dated, with a one-line note that it is a point-in-time
  measurement and the tickets it produced are the live record.
