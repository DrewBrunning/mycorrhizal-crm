---
title: Backend Development
parent: Development
nav_order: 2
---

# Backend Development

## Setup

Requires current [Go version](https://go.dev/doc/install).

```sh
cd backend
cp .env.example .env   # then edit with your values
go mod tidy
```

## Running Locally

```sh
source .env
go run main.go
```

The server starts on `HOST_PORT` (default `8080`). Migrations run automatically.

## Adding a New Endpoint

1. Define an input DTO in `models/` with validation tags.
2. Add a controller function in `controllers/`.
3. Register the route in `routes/routes.go`.
4. Add the validation schema to the middleware registration if needed.

## Database Migrations

### Creating a Migration

```sh
make migrate-create NAME=add_foo_column
```

This creates two files in `database/migrations/`: `NNNNNN_add_foo_column.up.sql` and `.down.sql`.

### Running Migrations

Migrations run automatically on startup. To apply or roll back manually:

```sh
make migrate-up
make migrate-down
make migrate-status
make migrate-force   # operator-only recovery for an interrupted (dirty) migration — prompts first
```

The migration gates are **fail-closed** (issue #439): the server refuses to
start on a dirty database, a database ahead of this binary's schema, or a
sub-floor database, each with a typed error naming the state and its recovery
(see `docs/upgrade-compatibility.md` → Refusal states). `migrate-force` is the
only path that clears a dirty flag, and it requires explicit operator
confirmation — never automatic.

## Models and DTOs

GORM models (`Contact`, `Activity`, etc.) live alongside input DTOs (`ContactInput`, etc.) in `models/`. DTOs are what controllers receive after validation while models are what GORM persists.

All models include `UserID uint` for tenant isolation.

## Services

Complex business logic lives in `services/`. Controllers call services; services own multi-step operations (e.g. sending emails). Services receive a `*gorm.DB` and any needed config, they do not access `*gin.Context`.

## Structured logging

`logger/` wraps zerolog. Operational code (scheduler jobs, sync services, notification/webhook dispatch, migrations, backup/restore) uses the standard field vocabulary in `logger/fields.go` — `event`, `component`, `operation`, `duration_ms`, `result`, `error` — instead of ad-hoc keys, and threads a `context.Context` so a `correlation_id` bound upstream rides along. `logger.Ctx(ctx)` returns the context-bound logger (falling back to the global); `logger.Op(ctx, "<event>")` times an operation and emits one standardized completion line. Significant state transitions also persist a `models.SystemEvent` row (issue #424) that surfaces on the admin `/system-events` timeline. Operator-facing detail is in `docs/operations/observability.md`; the design rationale is [ADR 0005](../adrs/0005-operational-event-model.md).

## Testing

```sh
go test ./...
```

Tests that touch persistence run against the **real migrated schema** — never
GORM `AutoMigrate` (CLAUDE.md backend trap #1). Use `internal/dbtest.New(t)`,
which hands each test an isolated copy of a per-binary migrated template
(issue #632); the `database` package's own tests call
`database.InitDB(filepath.Join(t.TempDir(), "x.db"))` directly. Which layer a
test belongs to is decided in [Testing](testing.md) — the explicit test
pyramid.
