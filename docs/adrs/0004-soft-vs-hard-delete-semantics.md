# ADR 0004: Soft vs hard delete semantics

- **Status:** accepted
- **Date:** 2026-08-15
- **Supersedes:** the earlier operation-based variance ("cascades hard, single deletes soft") that was
  considered and rejected

## Context

The app deletes user data of many shapes. Some of it is content the user authored and may want back;
some of it is edge/join rows that are meaningless without their endpoints. A uniform delete policy
produces either unbounded garbage (everything soft-deleted) or data loss (everything hard-deleted).

## Decision

**Soft vs hard delete is a property of the model, not of the call site.** It is decided once when an
entity is created, and `Delete` always means the same thing for that entity.

- **Content the user authored** (`Contact`, `Note`, `Activity`, `Reminder`, `LifeEvent`) → **soft
  delete**. Gives sync a free tombstone and undo something to work with.
- **Edge- and join-shaped rows** (`RelationshipEdge`, `CircleMember`, `ContactTag`,
  `HouseholdMember`, `ContactSyncLink`, `CalendarEventLink`, `FieldValue`) → **hard delete**. Small and
  bounded, so a client re-pulls them rather than tracking their deaths.

### Why it is a model property, not an operation

A soft-deleted row still occupies every unique index it is in, so a lingering dead row blocks
re-creating the same key. Every table with a natural-key composite unique index hard-deletes for
exactly this reason (a join row *is* its endpoints). Where an entity must soft-delete *and* carry a
unique key, the index is made partial (`... WHERE deleted_at IS NULL`), the way
`idx_contacts_vcard_uid_user` does.

Operation-based variance ("cascades hard, single deletes soft") was rejected: it makes every future
cascade site a chance to forget an `Unscoped()`, and the failure is silent.

### Exceptions (deliberate)

- **`DeleteUser`** hard-deletes via `Unscoped()`, because `users.email`/`username` are unique and a
  soft-deleted account would block re-registration forever.

## Consequences

- **Cascade deletes are manual.** Soft delete does not fire SQL `CASCADE`; `DeleteContact` and
  `DeleteUser` enumerate every dependent table explicitly. Adding an entity requires adding it there
  (`contact_controller.go`'s `DeleteContact` is the canonical checklist).
- `gorm.Model` gives soft delete for free on uint-PK entities. UUID-string-PK entities add
  `DeletedAt gorm.DeletedAt` explicitly (embedding `gorm.Model` there would add a conflicting `ID uint`).
