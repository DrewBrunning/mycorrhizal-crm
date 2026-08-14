# T90 — No way to mark a contact as "Me" (the column exists, nothing writes or reads it)

> **Landing note, 2026-08-14.** Implemented and merged as `feature/t90-mark-contact-as-me`.
>
> Backend: `PATCH /users/me/self-contact` (body `{ "vcard_uid": "…" }` or `null`/`""` to clear),
> scoped to the caller and 404ing any uid that doesn't resolve to a non-deleted contact the caller
> owns. The lazy backfill the schema comment promised is now real: `GetCurrentUser` calls
> `EnsureSelfContact` unconditionally — it is idempotent, and a backfill failure is logged and
> served-without, never a 500'd session. `deleteContactAssociations` (shared by `DeleteContact` and
> `CommitContactMerge`) nulls the pointer when the deleted contact was "Me"; `RepointContactAssociations`
> moves it onto the keeper when "Me" is the merge loser. OpenAPI updated; the drift test is green.
>
> Web: a "Your contact" picker card in Settings (debounced `getContacts` autocomplete over the same
> shape `MergeContactsDialog` uses, a clear button, and a cache refresh via `fetchAndCacheUserInfo`
> so the badge updates without a reload), and a neutral `color="default"` "You" chip next to the name
> on `ContactHeader` (per T62) with "This is me / This isn't me" in the compact overflow menu as the
> second entry point. The detail page seeds the badge from the localStorage `user_info` cache and
> corrects it from its own `/users/me` fetch. New strings in all five locales.
>
> Tests, all hand-verified: real-migrated-schema controller tests in
> `backend/controllers/self_contact_test.go` (lazy backfill, idempotence, set / clear-null /
> clear-empty, foreign-uid 404, unknown-uid 404, malformed-uid 400, delete-nulls-pointer, and the
> merge-commit path repointing the pointer to the keeper — the auto-created self contact is an
> ordinary `Contact`, so the delete path is exercised with the pointer pre-set), two merge repoint
> tests in `services/contact_merge_repoint_test.go`, and ContactHeader/SettingsPage unit tests (the
> "You" badge test and the merge-repoint controller test were each verified to fail with the
> feature disabled).
>
> **Review pass, same day** — four hardening fixes plus the extra tests above: `EnsureSelfContact`
> is now transactional, so a failed pointer-write can't leave an orphan contact behind (a real risk
> now that the lazy backfill calls it on every `/users/me`); `GetCurrentUser` restores the
> pre-backfill pointer on failure so the response can't claim a self contact the DB doesn't hold;
> the header toggle updates the badge optimistically and refreshes the cache once (it previously
> made a redundant second `/users/me` call, and a failed refresh could roll the badge back after a
> successful PATCH); and the detail page only lets a *successful* `/users/me` override the cached
> badge, so a transient fetch failure no longer hides it. New `api/users.test.ts` + `auth.test.ts`
> cover the PATCH wrapper (set/clear/error) and the badge cache (unset/corrupt/failure cases), and
> the Settings save-error path is pinned.
>
> Android deliberately out of scope per the ticket — the client half is a separate ticket. Two
> deliberate UI decisions beyond the ticket's letter: the "This is me / This isn't me" action is
> only in the compact overflow menu (the ticket's cited lines), not as a wide-viewport button —
> Settings is the wide-screen path; and a dangling pointer (self contact soft-deleted behind the
> scenes) renders as "not set" everywhere, since `getContactsByUid` can't resolve a soft-deleted
> row and no visible contact will ever match its uid.

| | |
|---|---|
| **Platform** | Backend + Web |
| **Rating** | 4 — the schema already committed to this idea; it is currently a dead column |
| **Size** | M — one endpoint, one lazy-backfill fix, one settings control, one badge |
| **Depends on** | Nothing |
| **Status** | **TO BE DONE** |
| **Source** | Beta testing note, 2026-08-13: *"How can I mark a contact as 'Me'?"* |

## Why this exists

You can't — but the data model already says you should be able to.

Migration `backend/database/migrations/000018_self_contact.up.sql` added
`users.self_contact_vcard_uid TEXT`, modelled at `backend/models/user.go:38-41`, with the comment "every
user gets a contact representing themselves on registration… Null for pre-existing users until they create
or are assigned one."

What actually exists:

- **One writer**, `services.EnsureSelfContact` (`backend/services/user_service.go:63-85`), which creates a
  `Contact{UserID, Firstname: user.Username}` and stores its `VCardUID`. It is idempotent (early return at
  `:70`) and is called from exactly three registration paths:
  `backend/controllers/user_controller.go:72` (self-registration),
  `backend/services/oidc_service.go:342` (OIDC first login),
  `backend/controllers/admin_user_controller.go:248` (admin-created user).
- **One reader**, `CurrentUserResponse.SelfContactVCardUID` (`backend/models/dtos.go:435`), returned by
  `GET /users/me` (`backend/controllers/admin_user_controller.go:44-70`, route
  `backend/routes/routes.go:55`).

What does not exist:

- **No endpoint to set or change it.** The user-mutation routes are `backend/routes/routes.go:50-54`
  (change-password, language, date-format, enabled-contact-fields) and none touches it; admin
  `PATCH /users/:id` (`routes.go:421`, `AdminUserUpdateInput`) doesn't either. So if the auto-created self
  contact is wrong — and for anyone who imported their real contacts it will be, since it holds nothing but
  a username — there is no way to repoint it.
- **No lazy backfill, despite the comment claiming one.** `user.go:39-40` and `user_service.go:67-68` both
  say pre-existing users are "handled lazily when they first hit an endpoint that needs it," but
  `GetCurrentUser` does not call `EnsureSelfContact`, and no other endpoint does either. Every account
  created before migration `000018` has `NULL` forever. **This is a live bug on real production data**, not
  a hypothetical.
- **No UI on either client.** `self_contact_vcard_uid` appears in exactly two frontend lines —
  `frontend/src/auth.ts:30` (the `UserInfo` interface) and `:70` (cached into localStorage from
  `/users/me`) — and **nothing reads it**. Android has zero references.
- **No flag on `Contact`.** The link is stored one-way on `users`, so "is this contact me?" is a `/users/me`
  lookup plus a UID comparison, not a column on the row.

## What to build

1. **`PATCH /users/me/self-contact`** — body `{ "vcard_uid": "<uid>" }`, scoped to the caller. Follow the
   shape of the existing single-purpose user-mutation routes at `backend/routes/routes.go:50-54` and the
   newer controller idiom (`currentUserID(c)`, `middleware.GetValidated[T]`, `apperrors.AbortWithError`).
   - The uid must resolve to a non-deleted `Contact` owned by the caller — otherwise `404 ErrNotFound`.
   - Accept an explicit `null`/empty to **clear** the link, so a user can un-mark without picking a
     replacement.
   - Setting it does not delete or modify the previously linked contact. It is a pointer move, nothing more.
2. **Fix the missing lazy backfill.** Call `services.EnsureSelfContact` from `GetCurrentUser`
   (`admin_user_controller.go:44-70`) before building the response, so a pre-`000018` account gets one on
   its next `/users/me`. It is already idempotent, so this is safe to call unconditionally — but it does
   *create a contact*, so it must be inside the same request's error path (a failure here returns the user
   without a self contact rather than 500ing the whole session).
3. **Web: a picker in Settings.** `frontend/src/SettingsPage.tsx` gains a "Your contact" control — an
   autocomplete over `getContacts({ search })` (the same shape
   `frontend/src/components/MergeContactsDialog.tsx:60-73` uses) that PATCHes the chosen uid, plus a clear
   button. Refresh the cached `UserInfo` in `frontend/src/auth.ts:70` afterwards so the badge below updates
   without a reload.
4. **Web: a "You" badge on the contact.** On `ContactDetailPage`, when `record.uid` equals the cached
   `self_contact_vcard_uid`, render a neutral chip in the header next to the name, and offer "This is me" /
   "This isn't me" in the header's overflow menu (`ContactHeader.tsx:340-400`) as a second entry point.
   Per [T62](86-T62-badge-and-button-color-system.md), the chip is `color="default"` — being yourself is not
   a status condition.
5. New strings translated in all five locale files (`/CLAUDE.md` frontend trap #5).

## Traps

- **`EnsureSelfContact` creates a contact.** Calling it from `GetCurrentUser` means a `GET` now has a
  write side effect for exactly one request per legacy account. That is the intended fix — but it must not
  run inside a read-only transaction, and it must not turn a `/users/me` failure into a login failure.
- **The self contact is an ordinary `Contact`.** It can be deleted, archived, merged away, or imported over
  like any other. Deleting it leaves `self_contact_vcard_uid` pointing at a soft-deleted row — the badge
  logic must tolerate that (treat a dangling uid as "not set"), and `DeleteContact`
  (`backend/controllers/contact_controller.go`) is the natural place to null the pointer. Add it there; it
  is the canonical cascade checklist per `/CLAUDE.md` backend trap #6.
- **Merge repoints, it does not follow.** If the self contact is the *loser* in a merge, its row is deleted
  (`contact_merge_controller.go:186`) and the pointer dangles. `RepointContactAssociations`
  (`backend/services/contact_merge_service.go:362-585`) must gain `users.self_contact_vcard_uid` to its
  list, the same way it handles `household_members.member_vcard_uid` at `:406-418`.
- Android is deliberately out of scope here — file the client half separately once this lands.

## Done when

- A user can pick any of their own contacts as "Me" from Settings, change the choice, and clear it.
- An account created before migration `000018` gets a self contact on its next `/users/me` — verified
  against a real migrated database with the column left `NULL`, not against `AutoMigrate`
  (`/CLAUDE.md` backend trap #1).
- The contact detail page shows a neutral "You" badge on the linked contact and nothing on the others.
- Deleting the linked contact clears the pointer rather than leaving it dangling; merging it away repoints
  the pointer to the keeper. Both pinned by tests.
- Setting a uid the caller doesn't own returns 404, not 200 (`/CLAUDE.md` backend trap #5).
- New strings translated in all five locales.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- `backend/openapi.yaml` covers the new route (the drift test enforces it).
