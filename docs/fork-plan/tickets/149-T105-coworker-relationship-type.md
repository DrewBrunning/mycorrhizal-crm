# T105 — Add `coworker_of` as a relationship type

| | |
|---|---|
| **Platform** | Backend + Web + Android |
| **Rating** | 3 |
| **Size** | XS — one registry entry, four hand-synced mirrors, five locale files |
| **Depends on** | Nothing |
| **Status** | **DONE**, 2026-08-13. `coworker_of` added to the backend registry (symmetric, `VCardTypeTag: "co-worker"`, synonyms coworker/co-worker/colleague/workmate) and to both frontend mirrors, `RelationshipEdgeTypes` on Android, and all five locale files. **`Registries.kt`'s `RelationshipType` object was deleted rather than updated** -- the ticket authorised this conditional on confirming zero consumers, and a grep across `android/**/*.kt` found none; a second hand-synced mirror of the same registry with nothing reading it is precisely the drift hazard trap 4 warns about, so keeping it in sync would have preserved the hazard. `relation_type` validation needed no change (it delegates to `IsKnownRelationType`). New `TestCoworkerRelationType` pins the backend half -- known, symmetric, self-inverse, the vCard tag, and all four synonyms through `MatchLegacyRelationType`. The Android-renders-the-raw-token parity gap (`"coworker of"` vs web's translated "Coworker") is pre-existing and left alone, as the ticket specified. |
| **Source** | Beta testing note, 2026-08-13: *"Add 'coworker' as a relationship."* |

## Why this exists

`backend/models/relationship_type_registry.go:30-129` registers 15 types — `parent_of`, `child_of`,
`spouse_of`, `sibling_of`, `friend_of`, `roommate_of`, `partner_of`, `co_parent_of`, `mentor_of`,
`mentee_of`, `owned_by`, `owns`, `gets_along_with`, `conflicts_with`, `related_to`. Professional
relationships are represented only by `mentor_of`/`mentee_of`, which are hierarchical. There is no peer
work relationship.

## What to build

Add one entry to `relationTypeRegistry` (`backend/models/relationship_type_registry.go:30-129`):

- **Token**: `coworker_of` — matching the existing `<role>_of` naming convention.
- **Inverse**: `coworker_of` (itself), **Symmetric**: `true` — the same shape as `friend_of` (`:53`) and
  `roommate_of` (`:59`).
- **VCardTypeTag**: `co-worker`. RFC 6350 §6.6.6 defines this token, so unlike `partner_of` and
  `co_parent_of` (which deliberately carry none, see the comment at `:65-68`) this one exports cleanly.
  `backend/models/contact_record.go:110-190` reads `RelationVCardTypeTag` for both edge directions, so
  adding it makes the type project as `RELATED;TYPE=co-worker`.
- **Synonyms**: `["coworker", "co-worker", "colleague", "workmate"]` — consumed by `MatchLegacyRelationType`
  (`:173`) for free-text legacy matching.

Then update **every** hand-synced mirror. Per `/CLAUDE.md` frontend trap #4 there is no dynamic type-list
endpoint by design, so each of these is a manual edit:

| # | File | What |
|---|---|---|
| 1 | `backend/models/relationship_type_registry.go:30-129` | the registry entry (source of truth) |
| 2 | `frontend/src/api/relationshipEdges.ts:11-14` | the `RelationshipEdgeType` union |
| 3 | `frontend/src/api/relationshipEdges.ts:25-41` | the `RELATIONSHIP_EDGE_TYPES` record (`inverse`/`symmetric`); `RELATIONSHIP_EDGE_TYPE_TOKENS` at `:43` derives from it and feeds the dropdown at `components/RelationshipEdgeDialog.tsx:316-320` |
| 4 | `android/core/model/.../network/RelationshipEdge.kt:12-26` and `:30-48` | the constant plus the `ALL` map of `Meta(inverse, symmetric)`; `TYPE_TOKENS` at `:48` derives from it. This is the mirror the Android UI actually consumes (`RelationshipsScreen.kt:495`, `RelationshipsViewModel.kt:298`/`:317`) |
| 5 | `android/core/model/.../registry/Registries.kt:12-33` | a **second, duplicate** Android mirror — see Traps |
| 6 | `frontend/src/i18n/locales/{en,de,es,fr,it}.json` | `relationships.types.coworker_of`. The `en` block is at `:1156-1172`; all five files carry the same block |

**No validator change.** `backend/middleware/validation.go:30` registers the `relation_type` tag and
`validateRelationType` delegates to `models.IsKnownRelationType` (`:133`), so it picks up the new token
automatically. `backend/openapi.yaml` has no hard enum of tokens either — only prose examples at `:1598`,
`:4806`, `:4810`, `:4861`, `:8464` — so at most a doc touch-up.

## Traps

- **`Registries.kt:12-33` is dead weight that must still be kept in sync.** Grep shows no production
  consumer of `RelationshipType.ALL`; the UI reads `RelationshipEdge.kt`'s copy instead. Either update both
  or delete `Registries.kt`'s relationship block outright — leaving one stale is how two mirrors silently
  disagree. Deleting it is the better outcome; confirm the zero-consumer finding before doing so.
- **Android renders the raw token, not a translated label.** `relationshipLabel`
  (`android/feature/relationships/.../RelationshipsScreen.kt:328-330`) and the dropdown at `:481`/`:497`
  just do `token.replace('_', ' ')`, so `coworker_of` shows as "coworker of" on Android while web shows a
  proper translated label. That is a pre-existing parity gap, not something this ticket introduces — note
  it in the landing note rather than fixing it here.
- **A missing i18n key renders the raw key path** on web, not a fallback label. All five locale files, not
  just `en` — `src/i18n/locales.test.ts` enforces identical key sets in both directions.
- The registry has no `category` field and no gendered variants; gender is handled purely as synonyms for
  legacy matching (`mother_of`/`father_of` at `:33`). Do not add a gendered `coworker` variant.

## Done when

- A relationship edge can be created with `type: "coworker_of"` from web and Android, and rejected with a
  400 for an unknown token as before.
- The type is symmetric: creating A→B surfaces on B's page as coworker too, with no second row stored.
- Exporting a contact with a coworker edge produces `RELATED;TYPE=co-worker` in vCard 4.
- A legacy free-text relation of "colleague" matches the new token through `MatchLegacyRelationType`.
- The label is translated in all five locales.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- `cd android && ./gradlew testDebugUnitTest lintDebug assembleDebug` green.
