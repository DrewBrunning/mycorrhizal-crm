# T36 — Life event categories + expanded default types

| | |
|---|---|
| **Rating** | 3 — nice-to-have breadth on an already-shipped feature |
| **Size** | M |
| **Depends on** | [T5](03-T5-lifeevent-frontend.md) (done) |
| **Alpha** | n/a — real data exists. New `Category` column on `life_events` is additive/nullable; no data loss risk |
| **Source** | v0.2.0-alpha real-world testing |

## Why this exists

Monica-style categorized life events — 5 named categories (Home & Living, Health & Wellness, Work
& Education, Travel & Experiences, Family & Relationships), each with its own default list plus a
per-category "Add a new life event type" affordance for custom ones.

**Correction from an earlier draft of this ticket:** an initial pass tried to verify this against
the current `monicahq/monica` GitHub source and found a different, much smaller seed
(`CreateVault.php`: 4 categories, ~20 low-stakes entries like "Watched a movie") — that turned out
to be the wrong version/build of Monica to check against, not evidence the 43-item list didn't
exist. The user supplied the actual list directly (reproduced below in full) — that supersedes
the source-check finding entirely. It matches exactly: 5 categories, 43 items.

`LifeEvent.Type` already exists as an open, unvalidated string with 7 predefined constants
(`married`, `graduated`, `job_change`, `had_child`, `adopted_pet`, `retired`, `moved`). Six of
Monica's 43 map directly onto six of those seven (see table below); `graduated` has no Monica
counterpart and is kept as a pre-existing extra rather than dropped.

## The full default list (43 items, user-supplied, authoritative)

Existing `LifeEventType*` constants are marked **(existing: `token`)** — same underlying stored
value, only the category grouping and (for two of them) the display label are new/changed.

**Home & Living** (6)
- Moved **(existing: `moved`)**
- Bought a home
- Made a home improvement
- Went on holidays
- Got a new vehicle
- Got a roommate

**Health & Wellness** (9)
- Overcame an illness
- Quit a habit
- Started new eating habits
- Lost weight
- Started to wear glasses or contact lenses
- Broke a bone
- Removed braces
- Had surgery
- Went to the dentist

**Work & Education** (7, +1 existing extra not in Monica's list)
- Started a new job **(existing: `job_change`** — display label updates from whatever it is today
  to "Started a new job" to match; stored value unchanged**)**
- Retired **(existing: `retired`)**
- Started school
- Studied abroad
- Started volunteering
- Published a paper
- Started military service
- Graduated **(existing: `graduated`, no Monica equivalent — kept, not dropped)**

**Travel & Experiences** (11)
- Started a sport
- Started a hobby
- Learned a new instrument
- Learned a new language
- Got a tattoo or piercing
- Got a license
- Traveled
- Got an achievement or award
- Changed beliefs
- Spoke for the first time
- Kissed for the first time

**Family & Relationships** (10)
- Started a relationship
- Got engaged
- Got married **(existing: `married`)**
- Anniversary
- Expects a baby
- Had a child **(existing: `had_child`)**
- Added a family member
- Got a pet **(existing: `adopted_pet`** — display label updates from whatever it is today to "Got
  a pet" to match; stored value unchanged**)**
- Ended a relationship
- Lost a loved one

Every category also gets an **"Add a new life event type"** affordance — see item 4 below.

## What to build

1. **Category as a real, closed concept.** Add a `Category` field to `LifeEvent`
   (`oneof=home_living health_wellness work_education travel_experiences family_relationships`,
   nullable — a custom event still needs a category to file under, but this stays a plain column
   rather than a new table, consistent with `Type`'s existing "conventional, unvalidated open
   string" treatment). Migration: additive nullable column on `life_events`.
2. **Extend `LifeEventType*` constants** in `models/life_event.go` with the 37 new tokens above
   (e.g. `bought_a_home`, `overcame_an_illness`, `got_a_tattoo_or_piercing`, …), grouped by a
   `LifeEventTypeCategories map[string]string` (type → category) the same way
   `relationship_type_registry.go` maps type → metadata, so the mapping lives in one place
   backend-side. Update the two display-label changes noted above (`job_change` → "Started a new
   job", `adopted_pet` → "Got a pet") in the i18n strings only — do not rename the stored constant
   values, since existing rows already use them.
3. **Frontend registry mirror** (per `CLAUDE.md`'s frontend-trap-4 convention — hardcoded, kept in
   sync by hand, commented as such): a `LIFE_EVENT_CATEGORIES` structure driving a cascading
   category → type picker in the life-event create/edit form, replacing the current flat type list
   if there is one. Order within each category should match the list above (it's already the
   order Monica settled on through real use).
4. **Custom events within a category ("Add a new life event type").** Per-category, an "Add a new
   life event type" option in the type picker that lets the user type a free-text value —
   `Type` is already unvalidated free text, so this is mostly a UI affordance (an extra option at
   the bottom of each category's type list) rather than new backend capability; the category is
   still required and comes from whichever category's picker the user opened it from. This reuses
   `Type`'s existing open-string design rather than building a second user-editable type registry
   — the category structure is the customization surface, not a new CRUD entity (that heavier
   pattern already exists as [T34](43-T34-contact-field-linking.md)'s link-type registry; no need
   to duplicate it here for a lower-stakes field).

## Traps

- Existing `LifeEvent` rows have no `Category` — backfill isn't feasible in general (the type→
  category map only covers the 7 existing constants, and free-text `Type` values used pre-ticket
  have no reliable category to infer). Backfill what maps cleanly via the 7 existing constants;
  leave the rest `NULL`/uncategorized rather than guessing, and make sure the UI handles an
  uncategorized existing event gracefully (e.g. an "Other" bucket in the picker).
- `TableName()`/`gorm:"column:..."` conventions per `CLAUDE.md` — don't let `Category` get a
  GORM-derived column name mismatch.
- Renaming `job_change`'s and `adopted_pet`'s *display* labels must not touch their stored `Type`
  values — existing rows, exports, and anything filtering on the literal string must keep working
  unchanged. This is a translation-file change only.
- i18n: 37 new type labels + 5 category labels + "Add a new life event type" across all 5 locales
  — this is the single largest new-string surface of any ticket in this batch; budget for it.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with a real-DB test for
  the migration and the backfill of existing constant-mapped rows.
- `npx tsc --noEmit` clean, `npx vitest run` green.
- Hand-verified: create a life event via the cascading category→type picker for each of the 5
  categories, including using "Add a new life event type" to create a custom one; confirm an
  existing pre-migration life event (e.g. one with `type: married`) shows under "Family &
  Relationships" after backfill, and that `job_change`/`adopted_pet` rows now display under their
  updated labels without their stored `type` changing.
- All 5 locale files have real translations for every new category, type, and affordance label.
