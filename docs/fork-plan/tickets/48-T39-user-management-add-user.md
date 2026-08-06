# T39 — Add new users from User Management

| | |
|---|---|
| **Rating** | 3 — a real gap in an admin-facing screen that already does the harder half |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | n/a — real data exists. New endpoint + form; no schema change |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

`UsersPage.tsx` (User Management) already lets an admin edit an existing user, including setting
their password (`editForm.password`) — but there's no way to **create** a new user from this
screen. Confirmed: no add/create affordance exists in the current page. Since the admin flow
already handles setting a password directly (rather than an invite-email flow), adding a user is
the same shape of work already built for editing one, just against a create endpoint instead of
an update.

## What to build

- Backend: an admin-only `POST /admin/users` (or wherever `admin_user_controller.go`'s existing
  routes live) accepting username/email/password/role, following whatever validation the
  self-registration path (if one exists) or `admin_user_controller.go`'s existing update path
  already uses for password strength / uniqueness checks. Reuse existing validation rather than
  writing a parallel set.
- Frontend: an "Add user" button on `UsersPage.tsx` opening a dialog with the same fields as the
  edit dialog (following its own pattern), calling the new create endpoint.

## Traps

- `users.email`/`username` are unique — per `CLAUDE.md`'s `DeleteUser` note, a soft-deleted
  account currently blocks re-registration of the same email forever (flagged as a known bug
  under **T26**, not this ticket's problem to fix, but worth being aware of: creating a user with
  an email that collides with a soft-deleted one will hit that same pre-existing bug). Don't
  silently swallow that error — surface it clearly if this ticket lands before T26 does.
- Follow `currentUserID(c)` / admin-role-check conventions already used elsewhere in
  `admin_user_controller.go` — this must not be reachable by a non-admin user.
- Password entered directly by an admin (not an invite-email flow) is already this app's existing
  pattern (per the edit dialog) — keep create consistent with that, don't introduce email
  invites as a surprise scope expansion.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with a controller test
  covering success, duplicate-email/username rejection, and non-admin rejection.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: an admin creates a new user from User Management, the new user can log in with
  the set password.
- All 5 locale files have real translations for any new strings.

## Landing note — 2026-08-06

Landed on `feature/T39-user-management-add-user` (squash-merged as
[#42](https://github.com/DrewBrunning/mycorrhizal-crm/pull/42)).

- **Backend**: `POST /admin/users` behind `middleware.AdminMiddleware`
  (`routes/routes.go`), handled by `controllers.CreateUser` in
  `admin_user_controller.go`. It mirrors `RegisterUser`'s hashing and validation but,
  being admin-gated, accepts `IsAdmin` directly and is deliberately *not* subject to
  `DISABLE_REGISTRATION` — an admin adding a user is not self-registration.
- **Uniqueness** is surfaced, not swallowed: a `UNIQUE constraint failed` from
  `users.username`/`email` becomes a `409 ErrAlreadyExists` with a
  `field: "username or email"` detail. The ticket's soft-delete-collision trap turned
  out to be moot in practice — T26 made `DeleteUser` hard-delete via `Unscoped()`, so
  an account removed through this app leaves no row to collide with.
- **Frontend**: an "Add user" action on `UsersPage.tsx` opening a dialog built from
  the same fields as the existing edit dialog, so create and edit stay one pattern
  rather than two.
- **Coverage**: controller tests for success, duplicate username/email, and non-admin
  rejection; `e2e/userManagement.spec.ts` pins the end-to-end claim the ticket asked
  to hand-verify — an admin creates a user who then logs in with the set password —
  plus the duplicate-username conflict keeping the dialog open.
- All 5 locale files carry real translations for the new strings.
