# M8 — Web ↔ Android parity audit: a screen-by-screen matrix, then tickets from its gaps

| | |
|---|---|
| **Rating** | 4 — not a feature; the instrument that tells us which features are missing |
| **Source** | Post-M1 review pass, 2026-08-11. M1's landing note reports Phases 1–5 complete without ever stating what "complete" was measured against. |
| **Depends on** | Nothing. This is inspection, and its output is tickets. |
| **Status** | **DONE — pass 1/2/3 complete 2026-08-11.** See the landing note below for the matrix and the tickets it produced. |

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

## The method used

Four passes, coarse to fine, per the original proposal below. Six parallel research passes covered
routes + every user-initiated action per screen (pass 1+2); results compiled into the matrix in the
landing note; gaps classified (pass 3) against the target agreed at sign-off; tickets filed (pass 4).

**Target agreed at sign-off (2026-08-11):** parity is the *default* — every gap becomes a "build it"
ticket unless a human explicitly marks it "deliberately not on mobile" with a reason. Claude was
explicitly instructed not to use its own judgment on what's desktop-only; candidates were surfaced
and the user decided.

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
this ticket's landing note, not a new top-level doc.

**Do not add a CI check that diffs the two route lists.** It would be red permanently by design
(Android is legitimately a subset), so it would be ignored within a week and then trusted by nobody.

## Done when

- [x] Pass 1 and 2 complete: a matrix covering every web route and every user-initiated action on it,
  with an Android verdict per row and both directions represented.
- [x] Every gap classified into one of the four buckets, with a recorded reason for each
  "deliberately not on mobile".
- [x] Tickets filed for the "build it" clusters, ranked on the board.
- [x] The matrix in this ticket's landing note, dated, with a one-line note that it is a point-in-time
  measurement and the tickets it produced are the live record.

---

## Landing note — audit results (2026-08-11)

Point-in-time measurement. Six parallel research passes read the actual `.tsx`/`.kt` source on both
sides (not clicked through, not inferred from names) as of commit range ending at `T64`
(`0572227`). **The tickets filed below are the live record from here forward — this matrix is not
maintained and will drift.**

### Confirmed corrections to the ticket's original (unaudited) starting table

- **`/reminders` is already gone.** The dead placeholder route the ticket described no longer
  exists in `frontend/src/App.tsx` — someone already deleted it since the ticket was filed. Nothing
  to do.
- **DataSettingsPage does not have backup/restore.** The ticket's starting table implied it did
  ("Import/export; the API client shipped, the UI didn't"). N6 backup/restore is a `make backup`
  CLI/CalDAV-adjacent procedure (see the N6 ticket), not a web UI, and was never meant to be — no
  gap to record between web and Android here because neither has one.
- **Notes/Activities global routes are confirmed literal `PlaceholderScreen` stubs**
  (`MycorrhizalApp.kt:454-455`) — the per-contact `NotesScreen`/`ActivitiesScreen` are real,
  full implementations, just unreached from the drawer. Ticket's framing holds.
- **Search's "only the contact-list search bar exists" is accurate but not the whole story** — that
  embedded bar hits a naive `LIKE` scan (`GET /contacts?search=`), not the T11 FTS5 `/search`
  endpoint with synonym resolution and notes/activities coverage. A separate, unrelated local FTS4
  mirror backs offline fallback only. All three are different code paths.

### New structural findings (not in the original starting table)

1. **Android's Dashboard does not call the M3 `/dashboard` composite endpoint at all** — it
   independently fans out to two legacy list endpoints. Of the four M3 widgets: birthdays partial
   (no age/today-highlight/click-through), reminders partial (no overdue styling/chips/actions),
   random-contacts and overdue-cadences both fully absent — despite
   `ApiClient.listOverdueCadences()` already existing and simply never being called.
2. **Cadence policy (T19, rating 5) has zero Android footprint** — no screen, ViewModel,
   repository, or route. Not a wiring gap; a whole feature was never started on Android.
3. **Life Events / Gifts / Preferences / Conversation Agenda share one generic Android scaffold**
   (`EntityListScreens.kt` + `TimelineEntitiesViewModel.kt`) that structurally has **no edit
   affordance and no delete-confirmation**, identically, across all four entity types. A fix to the
   shared scaffold very likely resolves all four at once rather than needing four separate tickets.
4. **Reverse finding — web has dead API-ready code, not just Android**: circle/tag **rename and
   delete** already exist end-to-end on web (`PUT`/`DELETE /circles/:id` and `/tags/:id`, the typed
   API client functions, and `handleUpdate`/`handleDelete` callbacks in `useCircles.ts`/
   `useTags.ts`) — they're just never wired to a button on any page. Same "wire it up" shape as
   Android's notes/activities, just on the other platform.
5. **Reverse finding — Android's Circles/Tags feature is more complete than web's** in one specific
   way: Android has genuine standalone circle/tag management (browse all, create, and — per #4 —
   rename/delete only reachable there) with dedicated list+detail screens; web only manages
   circles/tags as a side effect of editing a contact, or via the triage page for legacy strings.
6. **Dead/unwired code found on both platforms**, not gaps but worth a cheap fix each:
   `ContactListViewModel.loadNextPage()` (Android contact-list pagination past page 1 is
   unreachable — implemented, unit-tested, never called from the screen), `BulkOperationsViewModel`
   circle/tag actions (model + backend support them, `BulkOperationsScreen`'s UI only offers
   archive/unarchive/delete), `ApiClient.uploadVcfImport()` (hits the same backend endpoint web's
   VCF import uses, zero callers), and web's own `?search=` URL param on `/contacts` (nothing links
   to it — likely dead from a past refactor).

### The matrix

Six research passes, one per cluster. Full detail (file:line citations) lives in each pass's raw
output, summarized here at ticket-filing granularity. Verdict key: **native** (parity) /
**partial** (works, missing sub-features) / **missing** (absent) / **wire-it-up** (built,
unreachable) / **n/a-mobile** (technical, not a scope decision — hover states, keyboard shortcuts,
URL-param-only dead code) / **android-only** (Android leads; a reverse question, not a gap).

#### Cluster A — Contacts (list, create/edit, bulk, import, merge, detail top-level actions)

| Area | Verdict | Note |
|---|---|---|
| Search / add / merge / bulk archive-unarchive-delete | native | |
| Circle filter, archived-contacts toggle, per-row select on main list | missing | Bulk select only exists on a separate `BulkOperationsScreen` |
| Bulk add/remove circle/tag | wire-it-up | Model + backend done; `BulkOperationsScreen` UI only shows archive/unarchive/delete |
| Contact-list pagination past page 1 | wire-it-up | `loadNextPage()` implemented, unit-tested, never called |
| CSV upload + column mapping import | missing | Android's only import path is device-contacts (different, android-only, see below) |
| VCF upload import | wire-it-up | `ApiClient.uploadVcfImport()` exists, zero callers |
| Merge: search-based target picker | missing | Android requires typing the target's raw numeric ID |
| Merge: full association-count breakdown | partial | Android shows notes/activities/edges only, web shows ~11 categories |
| Contact form: prefix/middle/suffix, kind, language, tags | missing | Circles present but free-text only, no autocomplete |
| Contact detail: delete, archive/unarchive, export (vCard/JSContact), stay-in-touch quick action, profile-picture upload, tag chip editor, inline circle chip editor | missing | None of these exist on `ContactRepository` at all, not just missing UI |
| Contact detail: share contact, view prep | missing | Consistent with `/shares` and `/prep` both being wholly absent (see below) |
| Device address-book import, "Open in Contacts" deep link | android-only | Browser has no contacts API |

#### Cluster B — Contact sub-resources (notes, activities, reminders, life events, gifts, preferences, cadence, agenda, relationships)

| Area | Verdict | Note |
|---|---|---|
| Global `/notes`, `/activities` inbox routes | wire-it-up | Real per-contact screens exist; drawer routes are placeholders |
| Notes/Activities: search, date filter, pagination, delete | missing | Per-contact screens lack all four |
| Activities: multi-contact picker on create/edit | missing | Silently reuses the single route contact instead |
| Reminders: delete, overdue styling, "reoccur from completion", auto-date-from-recurrence, by-mail/flexible chips | missing | Mark-complete/edit/recurrence-select/send-by-email all native |
| Life Events, Gifts, Preferences, Conversation Agenda: edit, delete-confirmation | missing | Shared scaffold gap (structural finding #3 above) — one fix, four entities |
| Life Events: category, partial date, related contacts, remind-me | missing | Create dialog is type+description only |
| Gifts: status/section, URL, notes, occasion, amount, life-event/activity link, clothing sizes panel | missing | Create dialog is description-only |
| Preferences: key autocomplete, sensitivity, section grouping | missing | Create dialog is free-text category+value only |
| Conversation Agenda: reference URL, mark-discussed, open/discussed split | missing | |
| Cadence policy panel (whole feature) | missing | Structural finding #2 — rating 5, zero footprint |
| Relationships: search-based linking, sensitivity, gender/birthday on manual entry, edit, other-party name resolution + navigation, distinct reject action | missing | Create/accept/delete/type-select native |

#### Cluster C — Dashboard, Prep View, Search, Network

| Area | Verdict | Note |
|---|---|---|
| Dashboard | partial | Doesn't call M3 composite; 2 of 4 widgets fully missing (random contacts, overdue cadences), other 2 partial, no reminder complete/skip actions (structural finding #1) |
| Prep view (`/contacts/:id/prep`, N2, rating 5) | missing | Zero Android footprint — not even a placeholder route |
| Global search (`/search`) | missing | Placeholder route; embedded contact-list search is a different, weaker mechanism (naive LIKE, no synonyms, no notes/activities) |
| Network graph (`/network`, T10) | missing | Placeholder route |

#### Cluster D — Households, Settings, Data Settings

| Area | Verdict | Note |
|---|---|---|
| Household CRUD, add/remove member | native | |
| Household: role editing on existing member, name resolution (shows raw vCard UID), AI relationship suggestions | missing | |
| T40 shared-address household suggestions (scan/accept/dismiss) | missing | |
| Settings: language/date-format | missing (read-only) | Confirmed |
| Settings: theme, password change | missing | |
| Settings: webhooks, Immich, ntfy/Gotify notification channels, API tokens, link-field-type registry (server-side) | missing | Candidates for the exclusion decision below |
| Data Settings: custom field definitions, calendar sync (CalDAV subscriptions), contact-field visibility, CSV/VCF export | missing | Candidates for the exclusion decision below |
| Data Settings: CSV/VCF file import | missing | Distinct from Android's device-contacts import, which is a different (android-only) mechanism |

#### Cluster E — Shares, Audit, Users, Circle/Tag triage vs Android Circles/Tags, Register/Login

| Area | Verdict | Note |
|---|---|---|
| Contact sharing (`/shares`, P1) | missing | Zero footprint, including the entry point on the contact header |
| Audit trail + undo (`/audit`, T60) | missing | Zero footprint |
| Admin user management (`/users`) | missing | Candidate for the exclusion decision below |
| Circle/Tag triage (`/circle-tag-triage`, T2) | missing | Candidate for the exclusion decision below |
| Registration (`/register`) | missing | Candidate for the exclusion decision below |
| Login (password, OIDC/SSO) | native | Android additionally supports server-URL entry and API-token login (android-only, both structural necessities for a multi-origin native client) |
| Forgot-password flow | missing | |
| Circle/Tag rename, delete | **wire-it-up, but on web** | Structural finding #4 — full stack exists, no button calls it |
| Circle/Tag standalone browse, create | android-only | Structural finding #5 |

#### Cluster F — Android-only (reverse direction)

Call/SMS tracking (auto-logged activities, SMS body never leaves device), device-contacts import,
quick-capture post-call overlay pill (the pre-filled activity sheet itself is still unbuilt — a
known M5 gap, not counted here), custom link actions (local device-intent mapping, deliberately
never synced to server), installed-app link-action enrichment, QuickContact/"Open in Contacts".
All explicitly designed as native-only in M1/M5 — none are silent gaps. No reverse-port decision
requested at this time; noted for completeness per the ticket's "parity is not one-directional"
point.

### Pass 3 — classification decision (2026-08-11)

Per the sign-off target, parity is the default. Claude was explicitly instructed not to use its own
judgment on what's desktop-only — candidates were surfaced from the matrix above and the user
decided. Recorded decision:

**Deliberately not on mobile** (excluded from build tickets, with reason):
- Immich integration config — photo-sync provider config, plausible desktop/admin task.
- API token management + server-side link-field-type registry CRUD — distinct from Android's own
  local device-intent link-action mapping, which stays (it's a different, already-native feature).
- Custom field definitions (schema authoring) — instance-wide field-type design, not filling in
  values.
- Calendar sync (CalDAV subscriptions) — server/integration config.
- Contact-field visibility toggle — instance-wide form config.
- CSV/VCF/JSContact export + field picker.
- Admin user management (`/users`) — create/edit/delete other users, admin-only.
- CSV/VCF file-based import + column mapping — Android's device-contacts import stays either way
  (a different, android-only mechanism); this exclusion is specifically the upload-a-file path.

**Everything else defaults to build-it**, including several surfaces that plausibly *could* have
been waved off but weren't asked to be excluded: webhooks settings, ntfy/Gotify notification-channel
config, circle/tag triage, and registration. All of those got tickets below.

### Pass 4 — tickets filed (2026-08-11)

19 tickets, ranked onto `tickets/README.md`'s board at their assigned ratings:

| Ticket | Rating | Scope |
|---|---|---|
| [M9](91-M9-android-wire-up-existing-screens.md) | 4 | Wire up already-built screens & dead code |
| [M10](92-M10-android-dashboard-composite.md) | 4 | Dashboard: actually consume the M3 composite |
| [M11](93-M11-android-prep-view.md) | 5 | Prep view (N2) |
| [M12](94-M12-android-cadence-policy.md) | 5 | Cadence policy panel (whole feature) |
| [M13](95-M13-android-real-search.md) | 4 | Real FTS5 search, replacing the placeholder |
| [M14](96-M14-android-network-graph.md) | 3 | Network graph — needs a mobile design pass first |
| [M15](97-M15-android-contact-sharing.md) | 3 | Contact sharing (P1) |
| [M16](98-M16-android-audit-trail.md) | 3 | Audit trail + undo (T60) |
| [M17](99-M17-android-entity-scaffold-edit-delete-confirm.md) | 4 | Shared scaffold: edit + delete-confirm (fixes 4 entities at once) |
| [M18](100-M18-android-entity-field-richness.md) | 3 | Field richness for those same 4 entities (blocked on M17) |
| [M19](101-M19-android-notes-activities-depth.md) | 4 | Notes/Activities depth |
| [M20](102-M20-android-reminders-depth.md) | 4 | Reminders depth |
| [M21](103-M21-android-relationships-depth.md) | 4 | Relationships depth (name resolution is the standout item) |
| [M22](104-M22-android-household-depth.md) | 3 | Household management depth |
| [M23](105-M23-android-contact-list-bulk-breadth.md) | 3 | Contact list & bulk breadth |
| [M24](106-M24-android-contact-form-detail-actions.md) | 4 | Contact form & detail-page actions (delete/archive missing) |
| [M25](107-M25-android-settings-profile-channels.md) | 3 | Settings: profile & channels |
| [M26](108-M26-android-registration-triage.md) | 2 | Registration + circle/tag triage |
| [T65](109-T65-web-circle-tag-rename-delete.md) | 3 | Web-side: wire up circle/tag rename & delete (structural finding #4) |
