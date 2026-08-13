# Tickets — the live status board

One file per ticket, in execution order. Each is meant to be self-contained enough to implement
from without reading anything else in `docs/fork-plan/` — but **read `/CLAUDE.md` first**, since
it carries the repo-wide conventions and recurring traps that every ticket assumes.

**This file is the single source of truth for status and ordering.** `../95-backlog-and-
priorities.md` and `../92-delivery-roadmap.md` are historical/decision records — read them for
*why* a past decision was made, never for *what's currently outstanding*. If either ever
disagrees with this file about a ticket's status, this file is right.

**Ratings** (`R`): 5 practically necessary · 4 strong, frequent use · 3 nice to have · 2 rarely
used · 1 re-evaluate whether a CRM should do this.

Real production data exists as of `v0.2.0-alpha-candidate` (2026-08-04) — see `/CLAUDE.md`'s
data-safety rules, binding on every open ticket below.

## To be done

**Restructured into three platform lists, 2026-08-11.** Previously one global ranking, which kept
producing an order that disagreed with how the work actually gets done — Android parity tickets
inherited the rating of the web capability they port, so two of them sat above every open bug.

The rule now:

- **One list per platform: Backend, Web, Android.** A ticket spanning platforms is **split** into a
  per-platform ticket, each ranked on its own list, with the dependent half marked blocked. T66/T78
  and T73/T77 were split this way in this pass.
- **Within a list, rank by user impact first, then by relative size** (smaller first at equal
  impact). Ratings are still recorded on each ticket, but the list order is impact, not rating.
- **Platform order is Backend → Web → Android** as a general work-order heuristic, not a hard gate:
  a blocked web ticket doesn't stop an unblocked Android one.

Ticket numbers are stable IDs, not rank — the table order is the rank. Tickets touched in this pass
carry a **Platform** row in their header table; the `M`-series are Android by definition and were
not edited just to add one.

**Three mobile-API tickets filed 2026-08-10** (see the Deferred table's M1 row): M1's Android app
is being built, and this session pulls the backend/web API additions it needs out as their own
ranked tickets — M2 (mobile push device registration + FCM delivery), M3 (dashboard
today/overview composite), M4 (contact-detail composite). All three are backend + web only — their
Android-side consumers are [M5](84-M5-android-polish-and-hardening.md). (The note filed with these
said "the Android client itself is external to this repo"; that was already untrue when written —
the app lives in `android/` as of M1, 2026-08-10. Corrected 2026-08-11.)

> **Numbering cleanup, 2026-08-11.** Filing M2/M3/M4 at 81/82/83 left the tickets they overlapped
> in place, so `main` briefly carried two `81-` and two `82-` files. Resolved: `81-N10-fcm-push`
> was fully superseded and is retired — its backend became M2, and its Android half (the
> `FirebaseMessagingService`, the keep-polling-for-de-Googled-devices constraint, and the
> double-notify idempotence key) moved to [M5](84-M5-android-polish-and-hardening.md) §5a rather
> than being dropped. `82-M1-missing-endpoints` was only *partly* superseded — M3 took its
> dashboard item, its other three are still live — so it is renumbered
> [85/M6](85-M6-photo-url-user-prefs-oidc.md). Old links to either old filename will not resolve.

**M8 landed 2026-08-11** (see its own file for the full matrix) and filed the 19 tickets below —
M9–M26 (Android build-out) plus T65 (a web-side wiring gap the audit found in passing). Target
agreed at sign-off: parity is the default, every gap becomes a ticket unless explicitly marked
"deliberately not on mobile" — 8 surfaces were (Immich config, API-token + link-field-type registry
management, custom field schema authoring, calendar sync, contact-field visibility toggle, CSV/VCF/
JSContact export, admin user management, CSV/VCF file import). Everything else defaults to build-it,
including webhooks, notification-channel config, circle/tag triage, and registration.

**T67–T74 filed 2026-08-11** from a batch of testing notes — triaged with parallel research agents
per file before writing (see each ticket's "Why this exists" for root-cause detail; the phone
complaint split into two tickets, T68 dedup vs T69 search, since they're separate code paths sharing
one root cause). The "combine Contacts and Search" suggestion from the same batch was a product/IA
idea, not a bug — filed in Deferred → Feature ideas instead, not ranked here.

> **Grooming pass, 2026-08-11** (after the above). Restructured this table into the three platform
> lists documented above, and worked the open design questions rather than leaving them to
> implementation time. What changed:
>
> - **[T75](119-T75-plain-save-destroys-card-only-data.md) filed, R5** — found while checking T67's
>   claim that the backend "silently drops unmatched address kinds." That claim was wrong in a much
>   more interesting way: the drop is correct for invalid kinds, but *valid* Card-only data is
>   destroyed by any plain `db.Save` on a loaded contact. Reproduced against a `database.InitDB`
>   schema, with two live triggers on real production data. Nothing on the board covered it.
> - **T68, T69, T73, T66 and T74 had their deferred design decisions made**, with the reasoning
>   recorded in each ticket. T74's was a full design pass, per the user's request.
> - **T69 was re-scoped** after finding three phone-search code paths where the ticket assumed one;
>   the third became [T76](120-T76-android-local-fts-phone-search.md).
> - **T73→T77 and T66→T78 split** along the platform boundary, per the new structure.
> - **[T79](123-T79-flat-address-projection-too-narrow.md)/[T80](124-T80-web-address-editor-line-two.md) filed**
>   from a user question about apartment/PO-box support: hand-typed addresses lose nothing (nothing
>   parses them), but VCF-imported sub-street parts are invisible and uneditable.
> - **The i18n open question was closed** — see below.
>
> **Second pass, same day** — the two remaining design passes ([M7](88-M7-android-contact-record-coverage.md)'s
> multi-value editor, [M14](96-M14-android-network-graph.md)'s graph interaction) were completed, and
> each turned up something the design work exposed:
>
> - **[T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md) filed** out of M7 — the
>   Android form reconstructs email/phone objects on save, so editing a contact's *name* rewrites
>   every phone's label to `cell` and clears T58's preferred flag.
> - **[T82](126-T82-audit-snapshots-miss-nested-contact-data.md) filed** out of a T75 re-read — the
>   audit trail has never captured nested contact data, which is why T75's third trigger (the Undo
>   button) exists. T75 stops the bleed; T82 makes undo complete.
> - **M14's design shrank the ticket**: `/graph/connections` already returns resolved names and
>   per-hop relation chains, so the mobile view needs no layout engine at all.
>
> **Readiness pass, 2026-08-12.** A ticket-by-ticket audit found the T-series ready but the 19
> M-series tickets short of the bar: **0 of 19** named the CI gate, **18 of 19** had no defined test
> cases, and **15 of 19** had no API contract. Not a defect in M8 — those files came out of a *parity
> audit*, whose job was to prove a gap was real, not to specify the build. The difference only showed
> once they were ranked as work.
>
> Every M-series ticket now carries an **Implementation contract** section: the exact endpoints it
> needs, diffed against `ApiClient`'s 83 existing methods so "already there" vs "must be added" is
> stated rather than discovered; concrete test cases; and the three tasks
> `.github/workflows/android-tests.yml` actually runs. Several diffs were informative in their own
> right — M17 and M26's triage half need **no new endpoints at all** (pure UI gaps), while M25 needs
> ten and M15 seven. The gate wording in T67/T76/T81 was corrected too: `./gradlew test lint` is not
> what CI runs.

> **Contacts/Search fold designed and filed, 2026-08-12.** The "combine Contacts and Search"
> suggestion had sat unfiled in Deferred → Feature ideas since the 2026-08-11 testing notes, pending
> the design pass its row called for. Done, and it split three ways across the platform lists:
> [T85](129-T85-contacts-list-fts-search.md) (backend), [T86](130-T86-web-fold-search-into-contacts.md)
> (web), [T87](131-T87-android-fold-search-into-contact-list.md) (Android).
>
> The pass found the problem was bigger than the note said. Web has **three** search surfaces, not
> two — the AppBar autocomplete is a third, on the same endpoint as the Contacts list. And the two
> engines behind them can't substitute for each other: `applyContactSearch`'s LIKE scan is what makes
> circle/archived/sort/cursor composable, while `services.Search`'s FTS5 has the better matching and
> the only cross-entity coverage. **The decision that unlocked it: use FTS as a filter, not a
> ranker.** `/search` orders by bm25, which cannot back T17/T73's keyset cursor — but as a row-set
> filter, FTS composes with every existing list param untouched. That is T85, and it makes the other
> two tickets small.
>
> Consequences worth flagging: the LIKE clause is **kept and `OR`-ed** rather than replaced, because
> substring and token-prefix matching return different rows and three live consumers depend on
> today's behavior; and **[M13](95-M13-android-real-search.md) is superseded** — it specifies an
> Android screen mirroring the `SearchPage.tsx` that T86 deletes.

### Backend

None currently — the last two (T79, T85) landed and moved to Done below.

### Web

None currently — the last two (T74, T78) landed and moved to Done below.

### Android

| Ticket | Status |
|---|---|
| [M21](103-M21-android-relationships-depth.md) · Relationships depth | **IMPLEMENTED, AWAITING ON-DEVICE VERIFICATION** (2026-08-12). Name resolution, search-based linking, edit, sensitivity, gender/birthday, reject-vs-delete, and confirmed/suggested sectioning all landed; `testDebugUnitTest`/`lintDebug`/`assembleDebug` green. The ticket's on-device hand-verify step (link via search, edit sensitivity, reject a suggestion) is still outstanding — no device/emulator available in the build environment. |
| [M9](91-M9-android-wire-up-existing-screens.md) · Wire up already-built screens & dead code | **TO BE DONE**. R4. Cheapest impact on the list: global Notes/Activities routes, bulk circle/tag actions, contact-list pagination past page 1, and VCF-upload wiring are all implemented and just unreachable. |
| [M24](106-M24-android-contact-form-detail-actions.md) · Contact form & detail-page actions | **TO BE DONE**. R4. Delete and archive/unarchive don't exist at the repository level, not just missing UI — a real gap, not polish. |
| [M11](93-M11-android-prep-view.md) · Prep view (N2) | **TO BE DONE**. R5 capability, zero Android footprint — not even a placeholder route. |
| [M12](94-M12-android-cadence-policy.md) · Cadence policy panel | **TO BE DONE**. R5 capability, whole feature absent — no screen, ViewModel, repo, or route. Feeds M11's health card and M10's overdue-cadences widget. |
| [M10](92-M10-android-dashboard-composite.md) · Dashboard: actually consume the M3 composite endpoint | **TO BE DONE**. R4. Never was rewired onto M3 — still calls two legacy endpoints and is missing 2 of 4 widgets plus reminder complete/skip actions. |
| [M17](99-M17-android-entity-scaffold-edit-delete-confirm.md) · Entity-list scaffold: add edit + delete-confirmation | **TO BE DONE**. R4. One shared-scaffold fix resolves Life Events/Gifts/Preferences/Agenda at once. Unblocks M18. |
| ~~[M13](95-M13-android-real-search.md) · Real full-text search~~ | **SUPERSEDED** by [T87](131-T87-android-fold-search-into-contact-list.md), 2026-08-12. Not dropped — its endpoint contract, test cases and conventions were carried over verbatim. The file is kept for that provenance; the work is T87's. |
| [M7](88-M7-android-contact-record-coverage.md) · Contact record: the editor covers 8 of ~30 field groups | **TO BE DONE**. R4. **Design pass completed 2026-08-11** — one generic `MultiValueEditor<T>` driven by a per-type spec covers Email/Phone/OnlineService (and Title/PersonalInfo with `kind` bound instead of `label`); `Address` gets its own editor since it's structurally different; `Organization` needs none. The load-bearing rule is *edit entries in place via `.copy()`, never reconstruct* — the client-side mirror of backend traps #2/#3. The corruption that rule prevents was split out as [T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md) so it can ship first. Addresses/organizations/titles/online services/links/personal info are rendered but not editable; `how_we_met`/`work_information`/`contact_information` appear nowhere. |
| [M19](101-M19-android-notes-activities-depth.md) · Notes/Activities depth | **TO BE DONE**. R4. No search/date-filter/pagination/delete per-contact; activities silently can't have more than one participant. |
| [M20](102-M20-android-reminders-depth.md) · Reminders depth | **TO BE DONE**. R4. No delete, no overdue styling, no reoccur-from-completion, no auto-date-from-recurrence. |
| [M23](105-M23-android-contact-list-bulk-breadth.md) · Contact list & bulk breadth | **TO BE DONE**. R3. No circle filter or archived toggle on the main list; merge requires typing a raw numeric ID instead of searching. |
| [M22](104-M22-android-household-depth.md) · Household management depth | **TO BE DONE**. R3. Core CRUD has parity; role-editing, name resolution, AI suggestions, and T40 address-based suggestions don't. |
| [M25](107-M25-android-settings-profile-channels.md) · Settings: profile & channels | **TO BE DONE**. R3. Language/date-format are read-only; theme, password change, webhooks, and ntfy/Gotify config don't exist at all. |
| [M15](97-M15-android-contact-sharing.md) · Contact sharing (P1) | **TO BE DONE**. R3. Zero footprint, including the entry point on a contact's own header. |
| [M16](98-M16-android-audit-trail.md) · Audit trail + undo (T60) | **TO BE DONE**. R3. Zero footprint. |
| [M14](96-M14-android-network-graph.md) · Network graph | **TO BE DONE**. R3. M. **Design pass completed 2026-08-11** — ego-centric and list-first over `GET /graph/connections`, not a force-graph. That endpoint (T10) already returns resolved names and per-hop relation chains with inverses applied, so the hard part is server-side and the client needs no layout engine, canvas, or gesture arbitration — and every row is TalkBack-readable, which a drawn graph never is. Activity nodes are a deliberate v1 exclusion (the timeline answers that better on a phone); a radial view is deferred, not rejected. |
| [M18](100-M18-android-entity-field-richness.md) · Field richness: Life Events/Gifts/Preferences/Agenda | **BLOCKED** on M17 (edit needs to exist before these fields are worth adding to an edit form). R3. |
| [M26](108-M26-android-registration-triage.md) · Registration + circle/tag triage | **TO BE DONE**. R2. Both real but low-frequency: one-time account creation, one-time legacy cleanup. |

> **N8 (2FA/TOTP) moved to Feature ideas, 2026-08-07.** For a self-hosted instance
> going through OIDC the IdP already owns 2FA, so app-level TOTP is redundant there; it only
> matters for local password accounts, which a single-operator instance rarely has. Not dropped —
> it's genuinely more likely than the live-sync/Dawarich ideas below — just not scheduled. (N7, the
> last task before the v0.4.0 alpha cut, and N6, deferred to v0.5.0 to batch schema/model changes,
> have since both shipped — the two tickets this note originally deferred N8 behind are done.)

### Deferred — not ranked, no plan to schedule

None of these are implementation-ready. Each needs its own design pass before it's even a sizeable
ticket — pulled in only when a concrete need arises, never implemented straight from its file. Split
into three categories, 2026-08-06, because "deferred" was hiding a real difference in how solidified
each idea actually is.

**Mobile clients** — a real, intended project (a native Android app). **M1's Phase 1 (core client)
shipped 2026-08-10** (see its landing note); the remaining phases and the mobile-only features
(call/SMS tracking, quick-capture, T57 import) are still deferred on API-surface stability as
later phases of the same ticket.

| Ticket | Notes |
|---|---|
| [T57](66-T57-bulk-import-api-for-external-clients.md) · Documented/stable bulk-import API for external clients | R1–2. A named sub-piece of M1 — a repeatable contact-import contract the mobile app calls from both a first-run prompt and a standing "Import from contacts" entry point in Data, not a one-shot setup-only call. No concrete consumer until M1's later phases. |
| [P4](68-P4-local-model-pilot.md) · Local-model code-gen pilot | R1. Re-enters scope specifically when M1's work begins, independent of the rest of this roadmap. |

**Planned features** — concrete, scoped-enough-to-name integrations; higher confidence they'll
actually get built than the Feature ideas below, just not scheduled yet.

| Ticket | Notes |
|---|---|
| [P2a](70-P2a-paperless-ngx-integration.md) · Paperless-ngx integration (API) | R2. Link a contact to a document that already lives in Paperless-ngx. |
| [P2b](71-P2b-seafile-integration.md) · Seafile integration (API) | R2. Same idea, for Seafile-hosted files/folders. |
| [P2c](72-P2c-nextcloud-owncloud-integration.md) · Nextcloud / ownCloud integration (WebDAV) | R2. Same idea again, reached via WebDAV rather than a bespoke API — a genuinely different integration shape from P2a/P2b. |

**Feature ideas** — real, but "might come back to" rather than planned. Lower confidence than
Planned features that these get built at all.

| Ticket | Notes |
|---|---|
| [N8](25-N8-2fa.md) · 2FA / TOTP | R3. Highest-confidence item here — moved down from the ranked table 2026-08-07 because OIDC already covers 2FA for IdP-backed instances (see the note on the To-be-done table). Pulled in if local-account instances ever matter. |
| [P1b](69-P1b-standing-contact-share.md) · Standing/live contact share + permission model (true synced contacts across users) | R1–2. XL. The closest existing formalization of "true sync," not a one-time copy like the done [P1](31-P1-contact-sharing.md). |
| [P2d](73-P2d-dawarich-geopulse-integration.md) · Dawarich / GeoPulse integration | R1–2. Location-history correlation into life-event/activity suggestions — an L4 idea, not a simple link. |
| [P2e](74-P2e-jellyfin-integration.md) · Jellyfin integration | R1. Least-defined idea in this list — not even scoped enough to say what it would do. |
| [P2f](75-P2f-audiobookshelf-integration.md) · Audiobookshelf integration | R1. Same shape of idea as P2e, for Audiobookshelf. |
| [P3](76-P3-ai-ollama-layer.md) · AI / Ollama layer | R1. Summarization, entity/relationship extraction, memory-curator suggestions. Gated on the propose-then-approve pattern; `90` D1 is explicit this is not an AI-first project. |
| [T61](80-T61-contact-picker-api.md) · W3C Contact Picker API for PWA import | R1. Lets the PWA read device contacts directly (Chrome on Android only) instead of requiring a file export first. Narrow audience — Android + PWA + no native app installed. |
| [M6](85-M6-photo-url-user-prefs-oidc.md) · Backend endpoints the Android app needs (photo URL, user prefs, OIDC native return) | R2. From the M1 Phase-5 review: expose the profile-picture URL in list/detail payloads, `PATCH /users/me`, and a `client=android` OIDC callback that returns the token via a custom-scheme deep link. Was `82-M1-missing-endpoints`; renumbered 2026-08-11 to clear the duplicate `82-`, and its fourth item (the dashboard composite) is now M3. |
| [M5](84-M5-android-polish-and-hardening.md) · Android app: deferred polish, native-endpoint consumers, and the missing test tier | R3. The **Android-client counterpart to M2/M3/M4**, which are all backend-side. The work M1 shipped without: tablet layout + accessibility audit (M1 items 31/32, explicitly deferred), the four recorded UI deviations from the web, the in-overlay quick-capture sheet, the app-side clients for M2 (unblocked now — M2's backend is merged), M3, M4 and the M1-endpoints items, and a decision about the absent instrumented-test tier. A container of independently shippable sections, not an all-or-nothing gate. Filed 2026-08-11 after a full review pass of `android/` — that pass's *defect* fixes landed separately, see M1's review-pass note. |

## Done

Kept for reference/lookup, not ranked — order below is roughly the sequence they shipped in.

| Ticket | Status |
|---|---|
| [N1](01-N1-contact-merge.md) · Contact merge / dedupe | **DONE** |
| [N4](02-N4-notes-capture-inbox.md) · Notes: dead-end journal → capture inbox | **DONE** |
| [T5](03-T5-lifeevent-frontend.md) · LifeEvent frontend + timeline | **DONE** |
| [T5b](04-T5b-lifeevent-reminders.md) · LifeEvent → reminder wiring | **DONE** |
| [T2](05-T2-circle-tag-triage.md) · Circle/Tag triage migration | **DONE** |
| [T3](06-T3-circle-tag-backend.md) · Circle/Tag backend rewiring | **DONE** |
| [T4](07-T4-circle-tag-frontend.md) · Circle/Tag frontend rewiring | **DONE** |
| [T25](08-T25-known-gaps.md) · Known small functional gaps | **DONE** |
| [T26](08b-T26-delete-semantics.md) · Delete semantics — purge job + constraint fixes | **DONE** |
| [T1](09-T1-households.md) · Household CRUD + suggestion trigger | **DONE** |
| [T20a](10-T20a-preferences.md) · Preferences migration | **DONE** |
| [T6](11-T6-custom-fields-api.md) · Custom fields v2 — API | **DONE** |
| [T7](12-T7-custom-fields-frontend.md) · Custom fields v2 — frontend + retire v1 | **DONE** |
| [T9](13-T9-selective-export.md) · Selective field export + sensitivity gating | **DONE** |
| [T12a](14-T12a-etag-primitives.md) · Activity/LifeEvent ETag primitives | **DONE** |
| [T24](15-T24-test-coverage.md) · Non-critical test-coverage expansion | **DONE** |
| [T8](16-T8-openapi.md) · OpenAPI coverage + drift test | **DONE** |
| [T17](17-T17-change-feeds.md) · Change feeds + cursor pagination | **DONE** |
| [T23](18-T23-ui-polish.md) · UI polish | **DONE** |
| [T22](19-T22-legacy-audit.md) · Legacy/dead-code audit + migration squash | **DONE** |
| [T27](20-T27-crm-kind-ui.md) · Contact CRM.Kind UI (individual/pet/animal) | **DONE** |
| [T28](21-T28-mobile-contact-layout.md) · Mobile contact view layout fixes | **DONE** |
| | **→ ALPHA v0.1.0 — shipped** |
| [T19](20-T19-cadence.md) · CadencePolicy + relationship health | **DONE** |
| [T21](21-T21-conversation-agenda.md) · ConversationAgenda | **DONE** |
| [N2](22-N2-prep-view.md) · Prep view / person briefing | **DONE** |
| [T10](23-T10-graph-traversal.md) · Graph traversal + multi-hop | **DONE** |
| [T11](24-T11-search-fts5.md) · Search synonyms, household scope, FTS5 | **DONE** |
| [N5](27-N5-bulk-operations.md) · Bulk operations | **DONE** |
| [T20b](28-T20b-gift-tracking.md) · Gift tracking | **DONE** |
| [P1](31-P1-contact-sharing.md) · Contact sharing — one-time copy | **DONE** |
| [T14](32-T14-external-link-substrate.md) · External-link substrate | **DONE** |
| [T15/T16](33-T15-T16-immich.md) · Immich level 1 + 2 | **DONE** |
| [T29](38-T29-contact-field-gaps.md) · Contact field gaps | **DONE** |
| | **→ v0.2.0-alpha-candidate — real production data begins here** |
| [T34](43-T34-contact-field-linking.md) · Tappable contact fields (tel/sms/mailto/copy/link registry) | **DONE** |
| [T38](47-T38-search-address-fields.md) · Search doesn't index address fields | **DONE** |
| [T30](39-T30-hide-empty-subtitles.md) · Hide empty section subtitles | **DONE** |
| [T31](40-T31-contact-tabs-info-architecture.md) · Contact detail — grouped cards, not tabs | **DONE** |
| [T32](41-T32-remaining-pages-mobile.md) · Mobile layout — Network/Settings/User Mgmt | **DONE** |
| [T33](42-T33-mobile-nav-restructure.md) · Mobile navigation bar restructuring | **DONE** |
| [T36](45-T36-life-event-categories.md) · Life event categories + expanded default types | **DONE** |
| [T35](44-T35-gift-tracking-gaps.md) · Gift tracking gaps (URL, notes, full-form add) | **DONE** |
| [T39](48-T39-user-management-add-user.md) · Add new users from User Management | **DONE** |
| [N9](30-N9-notification-channels.md) · Notification channels beyond email | **DONE** |
| [T41](50-T41-http-url-allowlist.md) · Web-link fields: http(s) allowlist, not a four-scheme blocklist | **DONE** |
| | **→ ALPHA v0.3.0 — shipped** |
| [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) · VCF/CSV import merge silently corrupts and orphans existing contact data ⚠ | **DONE** |
| [T50](59-T50-vcard21-import-blank-fields.md) · vCard 2.1 import produces blank phone/email/photo | **DONE** |
| | **→ ALPHA v0.3.1 — shipped** |
| [T51](60-T51-push-notification-413-payload-too-large.md) · Browser push "Test notification" fails with 413 from the push service | **DONE** |
| [T42](51-T42-immich-link-person-error-misclassification.md) · Immich "link a person" fails with "Could not reach Immich" | **DONE** |
| [T44](53-T44-link-field-type-registry-not-in-editors.md) · Link field type registry doesn't reach the editors | **DONE** |
| [T46](55-T46-gift-add-entry-points-per-status.md) · Gift "add" entry points default to Idea everywhere | **DONE** |
| [T43](52-T43-link-field-type-custom-icons.md) · Custom link field type icons don't render | **DONE** |
| [T45](54-T45-contact-jump-nav-mobile-dropdown.md) · Contact jump nav should collapse to a dropdown on narrow viewports | **DONE** |
| [T47](56-T47-field-action-icons-layout-and-tel-link.md) · Field action icons should sit near the edit button; phone should also be a tel: link | **DONE** |
| [T52](61-T52-simplify-contact-add-flow.md) · Simplify the contact-add flow to name + contact fields | **DONE** |
| [T53](62-T53-contact-detail-delete-action.md) · Delete a contact from its own detail page, not only from the list | **DONE** |
| [T54](63-T54-contact-header-menu-fixed-position.md) · Contact header's actions menu shifts position when the name wraps | **DONE** |
| [T55](64-T55-copy-button-hover-visibility.md) · Copy button should be hidden until hover/tap, matching edit | **REVERTED** |
| [T58](77-T58-preferred-phone-email-ui.md) · No UI to see or set "preferred" on phone/email (and URL/IMPP) | **DONE** |
| [T48](57-T48-migrate-frontend-off-cra-to-vite.md) · Migrate frontend off Create React App to Vite | **DONE** |
| [T37](46-T37-pet-relationship-kind-default.md) · Pet relationship should default to animal kind | **DONE** |
| [T40](49-T40-household-suggestions-shared-address.md) · Suggest households from shared address | **DONE** |
| [T56](65-T56-bulk-contacts-import-flow.md) · Bulk contacts import (Google Takeout / contacts-app export) in Data Settings | **DONE** |
| [T12b](35-T12b-caldav-serve.md) · Serve Interactions/LifeEvents as CalDAV | **DONE** |
| [T13](36-T13-two-way-calendar.md) · Two-way calendar sync ⚠ | **DONE** |
| [T18](34-T18-audit-trail.md) · Event history / audit trail | **DONE** |
| [N7](29-N7-attachments.md) · File / document attachments | **DONE** |
| | **→ ALPHA v0.4.0 — shipped** |
| [T59](78-T59-immich-v041-still-broken.md) · Immich still broken in v0.4.1 testing | **DONE** (2026-08-09 — three root causes found against live Immich v3.1.0: HTTP/2 stale-session reuse, /api/people flat-array response shape, and GET /api/people/:id/assets removed in v3.x. Sync itself was explicitly *not* re-verified — that gap is now [T70](114-T70-immich-sync-400-diagnosis.md)) |
| | **→ v0.4.4 — shipped** |
| [N6](26-N6-backup-restore.md) · Full backup restore | **DONE** (2026-08-09 — tested `VACUUM INTO` online backup via `make backup` + restore procedure; see the ticket's landing note for the two deliberate deviations from its implementation suggestions) |
| [T60](79-T60-audit-trail-ui.md) · Audit trail UI | **DONE** (2026-08-09 — new `/audit` page + API module + hook over T18's shipped backend: event list with server-side entity_type/entity_id filters, contact-only Undo with confirmation dialog, contact uid→detail-page links, all five locales; see the ticket's landing note for the decisions taken) |
| | **→ v0.5.0 — shipped** |
| [M1](67-M1-mobile-android-app.md) · Native Android app (Kotlin, Jetpack Compose) | **Phases 1–5 core DONE** (2026-08-10 — working core client in `android/`: Gradle multi-module build, JWT/API-token auth, contacts list/detail/create/edit, activities/notes/reminders + unified timeline, tappable field actions + link-action enrichment (on-device verified), local FTS search, 349 hand-verified tests, CI workflow. Phase 3 sub-resources, Phase 4 native call/SMS tracking + notifications + quick-capture, Phase 5 T57 device-contacts import + QuickContact + custom link actions + R8. **Review pass 2026-08-11** — see the ticket's review-pass note: Android CI had never been green (two lint errors), `BootReceiver` could never fire, `:app` shadowed `:core:ui`'s resources, and ~80 strings were unlocalized; all fixed, 358 tests. Deferred polish moved to [M5](84-M5-android-polish-and-hardening.md)) |
| | **→ v0.5.1 — shipped** |
| [T62](86-T62-badge-and-button-color-system.md) · Chip/badge color system overloads brand/status colors | **DONE** (2026-08-11 — chips go neutral, "Add X" buttons + interactive icons + gift/agenda links go brand green, dark-mode chip-flattening bug fixed) |
| [T63](87-T63-typography-roles-garamond-mono.md) · EB Garamond/IBM Plex Mono are loaded but essentially unused — apply the role split mobile testing found | **DONE** (2026-08-11 — Garamond on page headings (h5) + persistent nav; Mono on `overline` section subheadings *and*, per follow-up feedback, on per-field labels like "Birthday"/"Phone"/"Address" to contrast against the sans field content) |
| [M2](81-M2-fcm-mobile-push.md) · Mobile push device registration (token+client) + FCM delivery | **DONE** (2026-08-11 — web Settings UI landed: mobile-device list + delete under the existing Web Push section, test button now probes both browser and mobile devices, 5-locale i18n. Backend landed 2026-08-10.) |
| [M3](82-M3-dashboard-overview-endpoint.md) · `GET /dashboard` today/overview composite | **DONE** (2026-08-11 — new composite endpoint aggregating birthdays/random contacts/upcoming reminders (contact name embedded)/overdue cadences; `DashboardPage` rewired off it, dropping the four-request fan-out plus its per-reminder N+1) |
| [M4](83-M4-contact-detail-composite.md) · `GET /contacts/:id/detail` composite | **DONE** (2026-08-11 — new composite aggregating the ~21 endpoints `ContactDetailPage.tsx` fires per contact, incl. relationship-edge and life-event name resolution and the one-config-check Immich block; web `api/contactDetail.ts` module ships as the Android client's target, `ContactDetailPage.tsx` itself deliberately not rewired per the ticket) |
| [T64](90-T64-household-suggestions-null-crash.md) · "Suggest Households" crashes the whole app when there's nothing to suggest | **DONE** (2026-08-11 — nil-slice fix in both flagged backend functions + frontend guard in both named call sites, raw-JSON and null-prop regression tests, hand-verified live; see the ticket's landing note for a second render-loop bug the first guard attempt introduced and how it was found) |
| | **→ v0.5.2 — shipped** |
| [T70](114-T70-immich-sync-400-diagnosis.md) · Immich sync reports a 400 error | **DONE** (2026-08-12 — hypothesis #1 confirmed by a single `LOG_LEVEL=debug` capture: Immich serializes `assets.nextPage` as a JSON *string* but `/api/search/metadata` validates `page` as a *number*, so every page past the first was rejected. Only ever fired for a person with >200 assets, and the fake server always returned `nextPage:""` — so the branch had never been executed by any test. Fake now rejects a string `page` with Immich's real 400 body; two new tests, one asserting the loop actually walked pages [1 2 3]. **Live re-verification still outstanding** — see the landing note) |
| [M8](89-M8-web-android-parity-audit.md) · Web ↔ Android parity audit — screen-by-screen matrix, then tickets from its gaps | **DONE** (2026-08-11 — six parallel research passes covered every web route and user-initiated action against real Android source; target agreed at sign-off was parity-by-default, exclusions decided explicitly rather than inferred. Filed 19 tickets: M9–M26 plus T65. See the ticket's landing note for the full matrix and structural findings — notably that Android's Dashboard never was rewired onto M3, Cadence (T19, rating 5) has zero Android footprint, and a shared entity-list scaffold is missing edit/delete-confirm across four entity types at once) |
| [T68](112-T68-phone-dedup-country-code-normalization.md) · Phone comparison doesn't reconcile country code, so real duplicates go undetected | **DONE** (2026-08-12 — `PhoneKey` last-10-digit canonical key with 7-digit minimum; replaces `normalizePhoneForComparison` at `DetectDuplicate` and `unionPhones` call sites; `+18005551234` and `(800) 555-1234` now dedupe in both import and manual merge; 17 new/updated tests hand-verified to fail pre-fix) |
| [T75](119-T75-plain-save-destroys-card-only-data.md) · A plain `db.Save` on a loaded contact silently destroys all Card-only data ⚠ | **DONE** (2026-08-12 — `BeforeSave` now *merges* the flat-field derivation onto the loaded Card/CRM/Passthrough instead of replacing it (`models/contact_card_merge.go`), so SpeakToAs/pronouns, PersonalInfo, unprojected address components (apartment/PO-box/floor), rich per-entry email/phone metadata, `CRMEnvelope.Kind` and imported Passthrough all survive a plain save. All three confirmed triggers pinned by handler-level regression tests + three Playwright specs driving the real UI. The chosen address rule is **per-entry dirty comparison** — a deliberate deviation from the ticket's whole-array rule, because T49's additive import merge appends to the flat arrays and the whole-array rule would have destroyed existing apartments the first time an import added a different address (see the ticket's landing note). Undo now restores the snapshot's flat state and preserves the Card-only data no snapshot has ever carried, with the partial-restore behavior stated in the undo dialog across all five locales. **Already-lost data is not recoverable** — the audit trail never captured Card (all `json:"-"`); [T82](126-T82-audit-snapshots-miss-nested-contact-data.md) closes that capture gap) |
| [T82](126-T82-audit-snapshots-miss-nested-contact-data.md) · Audit snapshots never capture Card/CRM/Passthrough | **DONE** (2026-08-12 — contact update *and* delete snapshots now capture the nested Card/CRM/Passthrough via a purpose-built `ContactAuditSnapshot` (`models/contact_audit_snapshot.go`); the `json:"-"` tags are untouched. Undo branches on `HasNested()`: T82 events restore the exact before state (`RestoreFullStateFrom`), pre-T82 events keep T75's preserve-what-isn't-in-the-snapshot path. Storage decision, measurement-backed: snapshot the full nested columns always, but strip `Card.Media` photo entries — measured 575 B card without a photo vs 86 KB with one (the base64 photo is flat-owned, so stripping is lossless and undo re-derives it from the restored `Photo` path). Sensitivity decision: include `private`/`secret` data — it's the user's own record, not an export surface. Deny-list reviewed against Card-shaped keys: no additions needed; redaction reaches nested depth. Tests hand-verified both directions — see the landing note) |
| [T69](113-T69-phone-search-tokenization.md) · Phone search misses results because nothing normalizes phone numbers | **DONE** (2026-08-12 — T38-style `contacts.phones_normalized` shadow column, migration `000020` with SQL backfill, both search paths normalized via `PhoneQueryTokens`; non-primary numbers now findable through global search; 10 backend tests + 3 Playwright e2e tests, all hand-verified and green) |
| [T73](117-T73-contacts-list-sort-control.md) · Contacts list can only be sorted by most-recently-edited | **DONE** (2026-08-12 — `?sort=updated_at|name` on `GET /contacts`: a denormalized, pre-lowercased `sort_name` column (migration `000021`, `COALESCE`-guarded SQL backfill, `(user_id, sort_name, id)` index) kept in sync by `BeforeSave`; a second `NameCursor` shape for name-order paging; `sort=name` + `?since=` is an explicit 400 (the feed is sync state, never name-ordered) and cross-shaped cursors fail loudly both ways; unknown `sort` is a 400, not a silent fallback. OpenAPI updated. Controller/model/migration suites + 4 Playwright e2e tests, all hand-verified; unblocks T77) |
| [T83](127-T83-immich-recentassets-walks-every-page.md) · `RecentAssets` walks a person's entire library to return one asset ⚠ | **DONE** (2026-08-12 — ordering verified trustworthy from the deployed v3.1.0 source (`fileCreatedAt` DESC default + explicit `order` param), so the walk can stop early; `RecentAssets` now requests `size: limit`, sends `order: "desc"`, and stops once `limit` assets are in hand — the contact summary drops from up to 100 requests to exactly 1, closing T70's release gate; request count pinned by fake-server + service-level tests and a new Playwright spec; see the ticket's landing note for the accepted approximation and the corrected `ImmichAsset` sort comment) |
| [T66](110-T66-contact-timeline-bounded-view-and-explorer.md) · Contact timeline: bound the M4 composite + paginated filterable endpoint | **DONE** (2026-08-12 — new `GET /contacts/:id/timeline`: cursor-paginated over a normalized `(event_date, type, id)` key, `?type=` + `?bucket=` filters, per-table N+1 bounded merge proven exact; composite's six timeline-eligible blocks capped at 5 each (external activities gained an `occurred_at` order, gifts now order by date) — measured payload for a 200-external-activity contact drops **122,779 B → 8,407 B** (~14.6×). Life events are the one documented bounded-merge exception (PartialDate isn't SQL-orderable — resolved and predicated in Go). Real-schema Go suite + 4 Playwright e2e specs, all hand-verified; OpenAPI updated; unblocks T78 — see the landing note) |
| [T85](129-T85-contacts-list-fts-search.md) · `GET /contacts?search=` should use FTS5, so search composes with the list's filters | **DONE** (2026-08-12 — `applyContactSearch` now ORs an FTS5 `contacts_fts` MATCH subquery onto its existing LIKE clause (gated at two runes), narrowing the same row set `circle`/`archived`/`sort`/cursor already narrow rather than replacing it. `services.ContactFTSMatch` centralizes the phone-vs-plain match-expression choice so `Search` and `applyContactSearch` can't drift. No migration. Unblocks [T86](130-T86-web-fold-search-into-contacts.md) and [T87](131-T87-android-fold-search-into-contact-list.md)) |
| [T79](123-T79-flat-address-projection-too-narrow.md) · The flat address projection has no slot for apartment / PO box / floor | **DONE** (2026-08-12 — `ContactAddress` widened with `POBox`/`Apartment`/`Floor`; all four mapping functions round-trip them, `FormatAddress` renders them between street and city (so the legacy `Address` scalar and the `addresses_flat` search column carry them), migration `000022` backfills stranded VCF-imported card components into the flat JSON for the rows that have them (down is a deliberate no-op — see the landing note), and the T75 merge projection widens with it so a street edit no longer destroys an untouched apartment. Import merge's `unionAddresses` treats the parts as part of an address's identity; T40 suggestion matching deliberately doesn't. Frontend flat type + card⇄flat adapters + `formatAddressLine` carry them, so an imported apartment shows in the contact detail; the editor inputs stay five fields — that is T80, now unblocked. Real-DB round-trip, migration-backfill, merge-semantics, unit tests + a Playwright spec (display / FTS search by apartment / VCF export round trip); live-hand-verified against a scratch backend) |
| [T86](130-T86-web-fold-search-into-contacts.md) · Fold the Search page into Contacts: one search field, one list | **DONE** (2026-08-12 — the Search page is folded into Contacts: a search `TextField` on its own row owns `?search=` (300ms debounce, two-rune minimum), the `/search` notes/activities hits render in a collapsed section below the cards with the `resolved_relation` line preserved, `/search?q=` redirects to `/contacts?search=` and `SearchPage.tsx` is deleted, and the AppBar autocomplete stays as jump-to-contact with its Enter retargeted to `/contacts?search=`. `search.*`/`nav.search` i18n keys removed and moved into `contacts.*` across all five locales. Unit tests (ContactsPage + new `SearchNotesActivities`) plus a rewritten `search.spec.ts` (incl. the redirect and resolved-relation UI paths); [T77](121-T77-web-contacts-list-sort-control.md) landed in the same branch and conforms to this ticket's `?search=` URL-param pattern. See the landing note) |
| [T77](121-T77-web-contacts-list-sort-control.md) · Contacts page has no sort control | **DONE** (2026-08-12, same branch as T86 — a sort `Select` in the filter row (name / recently-edited, both directions, defaulting to name ascending) persisted as `?sort=`+`?order=` via the same `setSearchParams` pattern T86 established; `GetContactsParams`/`useContacts` thread `sort` through so a change resets the cursor rather than appending; the selection is deliberately **not** cleared on a sort change. Five new `contacts.*` keys in all locales; three `ContactsPage.test.tsx` tests + a `contactSortControl.spec.ts` driving the real UI. See the landing note) |
| [T74](118-T74-desktop-field-row-action-distance.md) · Field action buttons sit too far from their field on wide desktop screens | **DONE** (2026-08-12 — two levels, both per the 2026-08-11 design pass. Level 1: `ContactInformation.tsx`'s field list becomes a CSS grid (2 columns at `lg`+), with a `FullSpanField` wrapper for multiline fields/SpeakToAs/card notes/the metadata toggle; every other field narrows for free. Level 2: `SectionGroup` gained an opt-in `twoColumn` prop (only `people`/`timeline`/`cadence`; `overview` deliberately excluded per the design) and `PanelCard` gained `fullWidth` (Connections' graph, the merged timeline). Measured, not eyeballed: the Gender field's action cluster moved **1078.8px → 498.8px** from its value at 1440px (row width 1136px → 556px, a 53.8% reduction) — confirmed via `getBoundingClientRect()` before/after a `git stash`/`pop` cycle. 2-up sections verified at 1440px/1280px, single column confirmed byte-identical at 1024px/390px; jump-nav and no-scroll-jump-mid-edit re-verified) |
| [T71](115-T71-mobile-circles-tags-add-row-overflow.md) · Mobile web: circles/tags "add" row overflows the screen, blocking use | **DONE** (2026-08-12 — `ContactHeader.tsx`'s edit-mode add rows (circles + tags) gain `flexWrap="wrap"`; the `Autocomplete`'s hard `minWidth: 200` floor becomes `{ xs: '100%', sm: 200 }` and the `TextField` gains `minWidth: 0`, so both wrap onto their own lines at phone widths instead of pushing the Add button off-screen. Hand-verified at 390px: pre-fix the Add button sat at x=394 (off-viewport); post-fix it's fully on-screen. Desktop unchanged) |
| [T72](116-T72-gender-edit-suggestion-autocomplete.md) · Gender edit is a bare text field, not the suggestion-autocomplete Add used to have | **DONE** (2026-08-12 — `EditableField` gains an opt-in `options`/`getOptionLabel` pair; when set, edit mode renders a `freeSolo` MUI `Autocomplete` instead of the plain `TextField`. Wired at gender's one call site with `GENDER_OPTIONS` and the same label mapping `genderDisplay` already used, matching the pre-T52 `AddContactDialog` widget verbatim. No other `EditableField` consumer affected — the prop is opt-in) |
| [T65](109-T65-web-circle-tag-rename-delete.md) · Wire up circle/tag rename & delete on web | **DONE** (2026-08-12 — new standalone `/circles` page (Circles/Tags tabs, `?tab=` state) built on the existing `useCircles`/`useTags` hooks, which no page had called before. Fixed a real bug found while wiring this up: both hooks' `handleUpdate` never called `refresh()` (unlike their own `handleDelete` and `useHouseholds`'s `handleUpdate`) — added it, with zero existing consumers affected by the change. Hand-verified end to end: create/rename/delete for both circles and tags, delete confirmed to remove a circle from a contact's membership, not just the circle itself) |
| [T80](124-T80-web-address-editor-line-two.md) · Address editor has no line 2 / PO box / floor field | **DONE** (2026-08-12 — `AddressFields.tsx` gains PO Box/Apartment/Floor inputs, hidden by default behind an "Additional fields" button and auto-revealed per-address whenever any is non-empty, so a VCF-imported address needs no extra tap. Reveal state tracked by `useRowKeys`' stable row key so it's genuinely per-address. T79's landing had already done the flat type/adapters/read-only rendering, so this was purely the editor inputs) |
| | **→ v0.5.3 — shipped** |
| [T78](122-T78-web-timeline-bounded-view-explorer.md) · Contact timeline: 5-item default + "View all" explorer | **DONE** (2026-08-12 — the timeline section preview truncates to the 5 most recent merged events (T66 already bounded the composite blocks at 5), and a "View all" button in the PanelCard's actions slot opens the full timeline explorer: an `AppDialog` driven by the T66 cursor endpoint with a six-type multi-select, a five-bucket recency select, and "Load more" cursor pagination — so "view all" is a second bounded fetch, not the old unbounded render. `api/timeline.ts` + `hooks/useTimeline.ts` follow the `relationshipEdges` shape; `ContactTimeline`'s rows are reused verbatim for both surfaces (plus an `emptyText` prop and aria-labels on its action icons). A page-level revision counter refetches the explorer after a note/activity edit or completion delete lands through the page dialogs. New strings in all five locales. 9 API + 7 component tests, 6 Playwright e2e specs (preview truncation, six-types mixed preview/explorer, type + bucket filters and their combination, cursor paging, empty states) — all hand-verified against the rebuilt test stack; full e2e suite green. See the landing note) |
| [T67](111-T67-android-address-import-parsing.md) · Device-contacts import loses address data | **DONE** (2026-08-12 — fixed both compounding bugs: `DeviceContactsReader` now reads the real `StructuredPostal` layout (`DATA1`=formatted, `DATA4`=street, `DATA7`=city, `DATA8`=region, `DATA9`=postcode, `DATA10`=country) instead of treating postcode as the formatted address; `DeviceContactMapper` emits the real registry kinds `name`/`locality` instead of `street`/`city`. `DeviceContact.addresses` is now a structured `List<DeviceAddress>` rather than a pre-joined string. Home/work TYPE carries through into `Address.contexts` (`private`/`work`). New `DeviceContactsReaderTest` + 3 new `DeviceContactMapperTest` cases, hand-verified to fail against the reintroduced bugs. **Hand-verified on a real device** (Pixel 8a): re-ran device-contacts import against a contact that had previously lost its address to this bug — imported correctly. Also corrected the source of the bug — [67-M1](67-M1-mobile-android-app.md)'s own column-comment table had the same off-by-one mistake the implementation faithfully copied) |
| [T81](125-T81-android-contact-edit-corrupts-phone-email-metadata.md) · Editing any contact relabels every phone "cell" and drops email/phone metadata ⚠ | **DONE** (2026-08-12 — `ContactFormState.emails`/`phones` are now `List<Email>`/`List<Phone>`, the loaded objects, not scalars; saving copies onto the loaded entry (`.copy(address = …)`/`.copy(number = …)`) instead of reconstructing, so `id`/`contexts`/`pref`/`features`/`label` all survive a save. The UI/ViewModel contract moved from a full-list replace to index-based edit/add/remove (`onEmailValueChange`/`onEmailAdd`/`onEmailRemove`, mirrored for phones) so a middle-row delete removes the exact object at that index rather than reindexing metadata onto a neighbor; `ContactFormScreen.kt`'s `StringListEditor` became a generic `ValueListEditor<T>`. The `cell` label default now lives only in `onPhoneAdd()` — the one place a phone is genuinely new — not applied to every phone on every save. Two new regression tests, hand-verified to fail against the reintroduced bug. **Hand-verified on a real device** (Pixel 8a): edited an existing contact's name only, confirmed on web afterward that an unrelated phone's label/preferred/id were unchanged) |
| [T76](120-T76-android-local-fts-phone-search.md) · Offline search can't find a contact by phone number either | **DONE** (2026-08-12 — new `PhoneKey.kt` mirrors backend `PhoneKey`/`NormalizePhoneDigits`/`FlattenPhones`/`PhoneQueryTokens`/`phoneFTSMatch` line-for-line; `CachedContact`/`CachedContactFts` gain `phonesNormalized` (every phone's digits + last-10 key, space-joined), covering all of a contact's numbers once detail has been fetched, not just `primaryPhone`. **Two deviations from the ticket's own suggested approach, both found by following its "verify before relying on" instruction**: (1) not a destructive migration — `pending_interactions` is a real not-yet-synced outbox (device call/SMS tracking), so a hand-written `MIGRATION_13_14` (`ALTER TABLE` + drop/recreate/rebuild the FTS4 mirror, since FTS4 can't `ALTER ADD COLUMN`) replaces `fallbackToDestructiveMigration` for this version bump; (2) FTS4's `column:"term"*` filter syntax silently matches nothing when quoted (confirmed empirically) — `phoneMatchExpr` uses unquoted `column:term*`, safe since the tokens are always pure digits. The ticket's third gap (unsanitized MATCH input) turned out already fixed by prior work — left as-is. `Migration13To14Test` builds a real v13-shaped database (every table, since Room validates the whole schema post-migration) and hand-verified both ways: swapped to `fallbackToDestructiveMigration`, confirmed the outbox-preservation assertion failed; a country-code-prefix test specifically pins the digits-vs-key duality the routing provides, since the ticket's literal scenario alone would still pass without it. **Hand-verified on a real device** (Pixel 8a): the migration ran live against a real on-device database at v13 with no crash and no data loss) |
| [T87](131-T87-android-fold-search-into-contact-list.md) · Fold search into the contact list, retire the placeholder search route | **DONE** (2026-08-12 — new `ApiClient.search(q, limit, householdId)` for `GET /search`; `SearchResult` deliberately has no `contacts` property at all (not just unused) so the response's contact group is structurally unable to leak into the contact list, which stays the sole authority (T85). `ContactListViewModel` gained `apiClient: ApiClient` directly (mirroring `DashboardViewModel`'s precedent for a composite endpoint with no single-entity home) and fires the cross-entity search from the *same* debounced job as the list fetch, gated at two characters client-side, never surfacing its own error (a structural guarantee — the failure branch has no code path that touches `ContactListUiState.error` — not a runtime-tested one, since `loadContacts` unconditionally resets `error` at the start of its own next run in the same tick, which would mask a regression regardless of assertion placement). New `SearchNotesActivitiesSection`: collapsed by default, re-collapses per query, resolved-relation banner independent of hit count, note/activity rows navigate via contact chip or show "Unfiled". The `search` drawer destination + placeholder deleted; `nav_search` removed from all five locales. Two real findings from verifying rather than assuming: the ticket's third listed gap (unsanitized MATCH input) was already fixed by prior work, and an initially-planned `assertNull(error)` test would have passed even with a real regression (traced the actual coroutine ordering before trusting it — see the ticket's landing note). New `ApiClientTest`/`ContactListViewModelTest`/`ContactListScreenTest` coverage; hand-verified per `/CLAUDE.md`. **Hand-verified on a real device** (Pixel 8a) across two installs: no crashes on real production data, drawer entry confirmed gone, T85 contact-list search confirmed working live — the cross-entity section's populated rendering wasn't observed live (no tried query matched this account's real note/activity content) but is covered by dedicated Compose UI tests against the same composable) |
| [T84](128-T84-android-custom-field-values.md) · Custom field values are entirely absent on Android | **DONE** (2026-08-12 — read-only slice, as the ticket's own §3 permits: all three `ApiClient` methods (`listFieldDefinitions`, `listContactFieldValues`, `replaceContactFieldValues`) are implemented and tested, but only the read path is wired into the UI; editing is a future ticket. New `core/model` types follow T87's nullable-list-with-normalized-accessor pattern (`/CLAUDE.md` frontend trap #8) — `FieldDefinitionsResponse`/`ContactFieldValuesResponse` declare their list fields `List<T>? = null` since Moshi rejects an explicit JSON `null` for a non-nullable list even with a Kotlin default, verified with a dedicated MockWebServer test. `ContactDetailViewModel` gained `apiClient: ApiClient` directly (same cross-cutting-read precedent as `DashboardViewModel`/T87), fetching definitions and this contact's values independently; the new "Custom fields" section renders by walking *definitions* and looking up each value by id, so a value whose definition no longer exists is structurally unreachable rather than a runtime skip-check — hand-verified by breaking that design (iterate values instead) and confirming exactly 2 tests failed. A fetch failure on either list never blocks or errors the rest of the contact screen. **Real-device snag resolved by reading source, not guessing**: a 200 response logged at 348 bytes parsed to an empty list, which looked like a possible silent-parse bug; traced to `field_definition_controller.go`'s `ListFieldDefinitions` wrapping every response in a `sync` metadata block that pads the envelope regardless of content — `total: 0` confirmed the account simply has no custom field definitions yet, not a bug. **Hand-verified on a real device** (Pixel 8a): whole-project CI gate green, section correctly absent with no crash for a contact with zero definitions; the populated render wasn't observed live for the same reason as T87's cross-entity section (no live data on this account) but is covered by 5 dedicated `ContactDetailScreenTest` cases with real fixtures) |

### ⚠ A grooming lesson worth keeping visible

The pre-alpha ordering rules (dependency → data/contract risk → rating → size) produced an alpha
with no rating-5 *capability* in it: T19 cadence, T11 search, and N2 prep view — all R5 — landed
*after* alpha, because the risk-based rule places the cut line by risk, and all three were
additive/safe to defer. N1 contact merge was the only R5 shipped inside alpha itself. All three
have since landed anyway (see above). Noted here because it's a real lesson for the next big
milestone cut, not a live concern — rating alone doesn't set a milestone line; risk does.

### Considered and deliberately not ticketed

Reviewed against Monica's feature set and rejected, with reasons, so they don't get re-raised:

- **Tasks** (R2) — deliberately delegated to an external manager: [T19](20-T19-cadence.md) routes
  overdue cadence to Vikunja via webhook rather than building a task system. A legitimate
  architecture choice, not an omission.
- **Debts / money owed** (R1) — Monica has it; niche enough to not belong in this product.
- **Conversation/message log** (R2) — already covered: `Activity.Type` includes `message`, and
  Activities support multiple contacts.
- **Introduction chain / "who introduced us"** (R2) — mostly covered by `HowWeMet`; if wanted, it
  is a registry token in `relationship_type_registry.go`, not a feature.
- **Standalone journal as it exists today** (R1) — superseded by
  [N4](02-N4-notes-capture-inbox.md) rather than kept.

### Open questions

*(None open.)*

- **Keep i18n across 5 locales? — RESOLVED 2026-08-11: keep.** Inherited from the original Meerkat
  fork, and it stays. Every user-facing string costs 5 real translations, enforced by
  `src/i18n/locales.test.ts`; every ticket's "Done when" continues to require them. This was open
  for months on the grounds that it's a permanent drag on a one-or-two-person instance — decided
  deliberately rather than by default, and recorded here so it stops re-surfacing each grooming
  pass. Dropping locales is a one-way door once translations rot; the drag is real but bounded.
