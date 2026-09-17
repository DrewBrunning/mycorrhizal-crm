# ADR 0019: Self-service account deletion

- **Status:** accepted
- **Date:** 2026-09-17
- **Depends on:** ADR 0004 (soft vs hard delete semantics — `DeleteUser`'s hard-delete exception)
- **Implements:** issue #972

## Context

The only account-deletion route was admin-only `DELETE /admin/users/:id`, and it explicitly refused
self-deletion. On the common single-user self-hosted deployment the sole user *is* the admin, so
there was no in-product erasure path at all — contradicting `docs/privacy.md`'s "Delete your account
and all its data" claim, and a real GDPR gap for any non-admin user on a multi-user instance who has
no host access to reach for a manual fix.

Two things complicate a naive "let anyone delete themselves":

1. **The sole-admin problem.** If the only admin self-deletes while other user rows remain,
   nobody can ever become admin again — `RegisterUser` only re-grants admin when the user table is
   completely empty (`userCount == 0`), which holds regardless of whether registration is disabled.
   A hard block defeats the actual goal (the single-user case is exactly what needed unblocking), but
   no block at all permanently strands real people.
2. **The audit-trail FK.** `audit_events.user_id` is `NOT NULL` with `ON DELETE CASCADE` to
   `users.id`. For an admin deleting someone else, actor ≠ target, so recording the delete after the
   transaction commits is safe. For self-deletion, actor *is* the target, so no `audit_events` row can
   durably record the operation: recording it before the delete gets cascade-deleted with the user
   row; recording it after hits a dangling FK on a row that no longer exists.

## Decision

- **Self-deletion is allowed unconditionally, except promote-then-delete when it would strand other
  users.** If the caller is the only admin and other user rows exist, they must name another existing
  user to promote to admin in the same request (`promote_user_id`); that user is promoted, then the
  caller's account and data are deleted, in one transaction. A genuinely single-user instance (no
  other rows at all) deletes directly — nobody is left to strand, and this is equivalent to a fresh
  install. A caller who is not the only admin also deletes directly.

  A hard block was considered and rejected: it would leave the single-user case — the actual point of
  this feature — exactly as unreachable as before. Requiring promotion instead of silently refusing
  keeps the guard's cost on the one case where it protects a real person (a multi-user instance's
  remaining users), not on the common case it would otherwise punish for no reason.

- **The self-delete and its promotion side-effect are not recorded in `audit_events`.** Rather than
  fight the FK (a schema change to make `user_id` nullable would let a "this account existed and
  deleted itself" row outlive the very erasure it is supposed to be a record of — in tension with the
  feature's own purpose), a structured server-log line is the operational record instead. This mirrors
  the general principle that this table's rows die with their actor by design (see the FK above); it is
  not a new exception invented for this feature.

- **Re-proof mirrors the codebase's existing convention for its most sensitive self-service actions**:
  current password (`ChangePassword`'s convention) plus a live TOTP/recovery-code proof if 2FA is
  enabled (`DisableTwoFactor`'s convention) — both, since this is the single most destructive endpoint
  in the app.

- **The cascade itself is shared, not duplicated.** `DeleteUser`'s delete-everything-owned-by-this-
  user-id transaction body was extracted into `deleteUserCascade` (`user_delete_cascade.go`) so the
  admin path and the self-service path can never drift into two different definitions of "every table
  a user owns" (ADR 0004's "cascade deletes are manual" consequence — one canonical checklist, not
  two). `controllers/delete_cascade_coverage_test.go`'s completeness sweep runs against both entry
  points from the same fixture-seeding helper.

## Consequences

- A multi-user instance can never end up with zero admins through self-service deletion; a
  single-user instance can always reach zero users through it.
- `docs/security/asvs-l2.md` control 8.3.2 drops "accepted for single-user self-host" — the gap it
  used to document is closed.
- The mutation-testing scope for delete-cascade coverage (`internal/mutationscope.Scopes`,
  `controllers-delete-cascade`) grew by one target file (`user_delete_cascade.go`); `DeleteOwnAccount`
  and its guard in `user_controller.go` are covered by line/branch tests here but not (yet) by
  dedicated mutation testing, since that file is far larger and mostly unrelated to delete cascades.
