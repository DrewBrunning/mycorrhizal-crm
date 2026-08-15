# ADR 0001: Neutral hub-and-spoke contact model

- **Status:** accepted
- **Date:** 2026-08-15
- **Supersedes:** the Meerkat fork's original flat/`models.Contact`-centric schema as the single
  source of truth for standardized contact data

## Context

The app exchanges contact data in several external formats (JSContact, vCard 4.0, vCard 3.0) and via
CardDAV, plus a CRM-specific envelope (circles, tags, cadence, etc.). Before this decision the codebase
mapped each format to the SQL/`models` layer directly, so every new format meant another bespoke
mapping, and format-to-format conversion required format-specific code paths.

## Decision

Adopt a **hub-and-spoke** architecture:

- **One neutral internal superset model** — `backend/contactmodel` — the hub. It is a pure data type
  set (no parsing, no mapping, no `gorm`), shaped closely after JSContact (the richest registry) but
  our own type set.
- **Independent format adapters** — `jscontact`, `vcard4`, `vcard3` — each implementing import *and*
  export. No adapter depends on another.
- **Conversion is emergent, never coded:** import format → neutral → export another. **RFC 9555 is
  never executed as code**; it is the authoring oracle that fixes what each format's output must be,
  encoded once in the correspondence table (ADR 0002).

### Key structural rules

- **Collections are ordered slices whose elements carry an optional stable `ID`** — this preserves
  JSContact map keys and vCard `PROP-ID`s without forcing a map.
- **Element `ID` fields serialize** (`json:"id,omitempty"`, never `json:"-"`), because `Record.Card` is
  persisted as a JSON database column and the `PROP-ID`/JSContact-map-key round-trip invariant depends
  on the ID surviving a save/reload cycle. Only persistence-layer identity (`UID`, `ETag`, stored as
  separate columns) stays `json:"-"`.

## Consequences

- Adding a format means adding one adapter against the neutral model; adding a concept means one row in
  the correspondence table (ADR 0002), not N format mappings.
- The neutral model is the single source of truth for what is "a contact's standardized data"; the
  `models.Contact` flat columns remain for the list/detail projection only, and are derived from the
  neutral `Card` via `ApplyRecordToContact` (see `/CLAUDE.md`, backend trap #2/#3).
