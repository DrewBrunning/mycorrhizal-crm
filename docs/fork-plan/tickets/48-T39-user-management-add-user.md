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
