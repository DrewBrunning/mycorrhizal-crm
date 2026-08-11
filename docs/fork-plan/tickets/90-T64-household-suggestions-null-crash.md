# T64 — "Suggest Households" crashes the whole app when there's nothing to suggest

| | |
|---|---|
| **Rating** | 4 — an `ErrorBoundary`-crashing bug (not a silent failure), and easily reachable: it fires whenever a scan finds zero groups, which includes the empty-account case every new user starts from. |
| **Size** | S |
| **Depends on** | — |
| **Alpha** | Not a data-safety issue — read-only endpoint, nothing persisted or corrupted. Purely a crash/availability bug. |
| **Source** | User's 2026-08-11 T63 follow-up testing: "Suggesting households with this user causes a full crash because of the lack of addresses on any contact." |

## Why this exists — reproduced live, root cause confirmed

Reproduced directly in the running dev app: log in as any user, go to Households, click "Suggest
Households." The page hard-crashes into the app's top-level `ErrorBoundary`:

```
TypeError: Cannot read properties of null (reading 'flatMap')
    at AddressHouseholdSuggestions (frontend/src/components/AddressHouseholdSuggestions.tsx:28)
```

Confirmed reproducible with **zero contacts on the account** — not a large-dataset or malformed-data
edge case, just "nothing to suggest," which is also what happens whenever every contact lacks an
address, or no two contacts share one. This is exactly `/CLAUDE.md`'s own documented Frontend Trap
#8 ("A Go struct field that is `omitempty` and a TS field that is required is a crash waiting to
happen"), just via a nil slice rather than an `omitempty` tag — same root shape, third occurrence.

**The chain, backend to frontend:**

1. `services/household_service.go:343` — `GenerateAddressHouseholdSuggestions` declares
   `var suggestions []AddressSuggestion` and only ever `append`s to it. When the caller's contacts
   have no shared addresses (including "zero contacts," "no contact has an address," and "every
   shared address has only one contact at it"), the loop body never runs and this stays a **nil
   Go slice**, returned as-is (`household_service.go:403`, `return suggestions, nil`).
2. `controllers/household_controller.go:417-420` — `SuggestAddressHouseholds` puts that nil slice
   straight into the JSON response: `c.JSON(http.StatusOK, gin.H{"suggestions": suggestions, ...})`.
   Go's `encoding/json` marshals a nil slice as `null`, not `[]` — confirmed live, the actual response
   body was `{"suggestions":null,"total":0}`.
3. `frontend/src/api/households.ts:196-197` — `SuggestAddressHouseholdsResponse.suggestions` is
   typed `AddressHouseholdSuggestion[]`, non-nullable. TypeScript has no way to know the backend can
   send `null` here, so nothing downstream is guarded.
4. `frontend/src/HouseholdsPage.tsx:122-123` — `setAddressSuggestions(result.suggestions)` stores the
   `null` straight into component state (typed `AddressHouseholdSuggestion[]`, but nothing at runtime
   stops a `null` from landing there).
5. `frontend/src/components/AddressHouseholdSuggestions.tsx:34` —
   `suggestions.flatMap((s) => s.member_vcard_uids)` runs unconditionally in a `useMemo`, on whatever
   `suggestions` prop it was handed. `null.flatMap` throws, and nothing catches it before the
   `ErrorBoundary`.

## What to build

1. **Backend: never return a nil slice for `suggestions`.** Initialize
   `suggestions := []AddressSuggestion{}` in `GenerateAddressHouseholdSuggestions` instead of
   `var suggestions []AddressSuggestion`, so an empty result always marshals as `[]`, matching what
   the TS type already promises. This is the primary fix — same shape as `/CLAUDE.md`'s existing
   guidance for collection response fields.
2. **Frontend: guard against `null`/`undefined` anyway, defense in depth.** The TS type being wrong
   once is exactly how this shipped — don't leave the frontend trusting it unconditionally a second
   time:
   - `AddressHouseholdSuggestions.tsx:34`: default/guard the `suggestions` prop
     (`(suggestions ?? [])`.flatMap(...)`, or normalize once where the prop is destructured).
   - `HouseholdsPage.tsx:123`: normalize `result.suggestions ?? []` before calling
     `setAddressSuggestions`, so a `null` never enters state even if some other code path
     (a future endpoint, a mock, a proxy) sends one again.
3. **Add a backend test asserting the *raw JSON* shape**, not just the decoded Go value — per
   `/CLAUDE.md`'s own note on trap #8: decoding into the Go struct makes "absent"/`null` and `[]`
   indistinguishable, which is exactly why this shipped unnoticed. Assert `GET
   .../households/suggest-addresses` with an account that has zero qualifying groups returns literal
   `"suggestions":[]` in the response body, not `"suggestions":null`.
4. **Add a frontend test** rendering `AddressHouseholdSuggestions` with `suggestions={null as any}` (or
   however the test simulates the malformed API response) and asserting it renders an empty state
   instead of throwing — this is the regression test that would have caught it before it shipped.

## Traps

- Don't fix this by special-casing "zero contacts" — the same nil-slice-to-`null` conversion happens
  for *any* input that produces zero suggestion groups (all contacts share no address, addresses
  don't normalize into groups of 2+, everything's already dismissed/already co-members). Fix the
  general nil-slice-serialization issue, not the specific reproduction case.
- Check `services/household_service.go` for other functions with the same `var x []T` pattern that
  feed a JSON response — this file has more than one suggestion-generating function
  (`SuggestRelationshipsFromHousehold` is the sibling worth a quick look), and this bug class doesn't
  announce itself until something in the frontend actually calls `.flatMap`/`.map`/`.length` on the
  result unguarded.
- This is now the **third** documented instance of the same nil-slice/required-TS-array bug shape in
  this codebase (per `/CLAUDE.md` Frontend Trap #8's own prep-view example). Worth considering, as a
  separate/future decision (not this ticket's scope), whether API responses should go through a
  shared serialization helper that normalizes nil slices to `[]` once, instead of relying on every
  handler remembering to initialize its own.

## Done when

- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- `cd frontend && npx tsc --noEmit && npx vitest run` green.
- The new backend raw-JSON test and the new frontend null-prop test both exist and were hand-verified
  to fail against the current code first (per `/CLAUDE.md`'s testing rule), then pass after the fix.
- Hand-verified live: an account with zero contacts (or any contacts, none sharing an address) can
  click "Suggest Households" without crashing, and sees the existing "no suggestions" empty state
  instead.

## Landing note — 2026-08-11

Done. Both flagged nil-slice call sites fixed: `GenerateAddressHouseholdSuggestions` (the reported
crash) and, per the ticket's own trap note, `GenerateHouseholdSuggestions`'s sibling `created`
slice — `var x []T` → `x := []T{}` in both, so an empty result always marshals as `[]`. New raw-JSON
backend test (`TestAddressHouseholdSuggestions_EmptyResultIsJSONArrayNotNull`) hand-verified to fail
against the pre-fix code (`suggestions:null`) before passing. Frontend guarded in both places the
ticket named — `AddressHouseholdSuggestions.tsx`'s prop and `HouseholdsPage.tsx`'s
`setAddressSuggestions` call — plus a new null-prop regression test, also hand-verified failing first
(reproduced the exact `TypeError: Cannot read properties of null (reading 'flatMap')` from the
report).

**A second, more interesting bug surfaced during verification, not in the ticket's own text.** The
first version of the frontend guard — `const suggestions = suggestionsProp ?? [];` inline in the
component body — passed `tsc`, passed `go test`, and passed the new test *in isolation*, but hung
`npx vitest run`'s full suite indefinitely (confirmed pegged at 100%+ CPU for 24+ minutes before being
killed). Root cause: `?? []` evaluates a **fresh** array literal on every render when the prop is
null/undefined, whereas an explicit `[]` prop keeps a stable reference across a component's own
internal re-renders. That fed a `useMemo`/`useEffect`/`setState` chain further down (member-name
resolution) that depends on referential equality — a new array every render meant the memo never
stabilized, the effect never stopped re-firing, and `setState` never stopped forcing another render.
Explicit `[]` props never triggered it because `??` only evaluates its right side when the left is
null/undefined. Fixed by hoisting a single module-level `EMPTY_SUGGESTIONS` constant and falling back
to that instead of a fresh literal — see the comment at its declaration in
`AddressHouseholdSuggestions.tsx`. Root-caused with a bisection: minimal-repro test files (deleted
after use) narrowed it from "the whole suite hangs" down to "two back-to-back renders with an
undefined/null prop, in a component using this exact `useMemo` → `useEffect` → `setState` shape,
hang; two back-to-back renders with an explicit `[]` prop don't." Worth remembering as its own trap —
a `prop ?? []` fallback is not the same as a stable `[]`, once anything downstream keys a memo or
effect off that value's identity.

Full verification: backend (`go build`/`vet`/`gofmt`/`test`) green; frontend (`tsc --noEmit`, full
`vitest run` — 83 files / 623 tests) green, confirmed not hanging after the `EMPTY_SUGGESTIONS` fix;
hand-verified live against the local dev server (fresh account, zero households) — raw response body
`{"suggestions":[],"total":0}`, empty-state UI renders with no console errors, survived three repeated
clicks of "Suggest Households" (the scenario that would have re-exposed the render-loop bug had the
fix been wrong).
