# T91 — Every imported address comes back with type "private" instead of "home"

| | |
|---|---|
| **Platform** | Backend (fixes web VCF import and Android device import at once) |
| **Rating** | 4 — visible on real imported production data, on both clients |
| **Size** | XS — one translation in one projection function |
| **Depends on** | Nothing. [T67](111-T67-android-address-import-parsing.md) fixed the *content*; this is the *type*. |
| **Status** | **DONE**, 2026-08-13. The ctx2type pair moved from `vcard4` to `contactmodel` (which both `vcard4` and `models` already import, and which owns the neutral model the table describes) as exported `TypeTokenToContext`/`ContextToTypeToken`; `vcard4`'s two package-private vars now alias them, so its call sites are untouched. `contactAddressFromNeutral` translates through the map with an explicit fall-through for unmapped contexts. Two tests, both hand-verified against the reintroduced `out.Type = a.Contexts[0]`: a table test over the projection (private->home, work/billing identity, an already-legacy `home` passing through, an arbitrary `cabin` preserved verbatim, plus a flat->Card->flat round trip), and an end-to-end `TestVCardImportAddressType` that runs a real `ADR;TYPE=HOME` vCard through `vcard4.Adapter{}.Import` -> `ApplyRecordToContact` -- the span that hid the bug, since each side was correct alone. **No data migration**, per the ticket's own recommendation: existing rows keep the stored `"private"` string, which displays as raw text but is not lost and is editable; a blanket UPDATE over a JSON column on real production data is the larger risk. `DeviceContactMapperTest` untouched -- Android was always right. **Live verification found the fix was half done:** the backend now persisted `type: "home"`, but the contact detail page still rendered "(private)" because it reads the nested Card, not the flat column -- and `cardAddressesToValues` (`frontend/src/api/contacts.ts`) had the identical untranslated `a.contexts?.[0]` line. Added the mirror map there with the same fall-through, plus a unit test hand-verified against the reintroduced line. Confirmed end to end against a live import: the Card correctly keeps `contexts: ["private"]`, the flat column stores `home`, and the page renders "(Home)" through the i18n key. |
| **Source** | Beta testing note, 2026-08-13: *"Android imported addresses are correct content, but always 'private' for type."* Investigation found the bug is not Android's. |

## Why this exists

The report blames Android. Android is correct; the backend projection is not.

The neutral model (RFC 9553) uses `private`/`work` for address contexts. The flat legacy model uses
`home`/`work`. There is a translation table for exactly this — `ctx2type`, at
`backend/vcard4/adapter.go:241-246`:

```
typeTokenToContext = { "home": "private", "work": "work", ... }
contextToTypeToken = { "private": "home", "work": "work", ... }
```

**It is only applied inside the vCard adapter.** The Card→flat reverse projection copies the context
straight through with no translation — `backend/models/contact_record_reverse.go:306-308`:

```go
if len(a.Contexts) > 0 {
    out.Type = a.Contexts[0]
}
```

So the flat `ContactAddress.Type` literally becomes the string `"private"`, which no other part of the app
understands. `frontend/src/components/AddressFields.tsx:96-100` renders the type through
`t('contacts.types.' + opt, opt)`, and `private` has no such key, so it falls back to displaying the raw
token.

**Two import paths hit this, not one:**

1. **Web VCF/CSV import.** A vCard `ADR;TYPE=home` goes through `splitTypeTokens`
   (`vcard4/adapter.go:250`) → `typeTokenToContext` → `Contexts: ["private"]` → reverse projection →
   flat `Type: "private"`. Every VCF-imported home address on web has this bug.
2. **Android device import.** `DeviceContactMapper.toAddress` (`android/feature/import/.../DeviceContactMapper.kt:87-105`)
   maps `StructuredPostal.TYPE_HOME` → `listOf("private")` — which is *right*, it's the neutral vocabulary —
   and then hits the same untranslated projection. `TYPE_HOME` is the Android Contacts app's default for a
   new address, which is why in practice nearly every imported address shows it.

The forward direction confirms the asymmetry: `backend/models/contact_record.go:643-647` pushes the flat
`a.Type` into `Contexts` *unchanged*, with a comment explaining that the free-text `Type` isn't guaranteed
to be a valid context. So a web-entered `home` round-trips as `contexts:["home"]` and displays fine — only
the adapter-fed and Android-fed paths produce `private`.

## What to build

Translate through the existing map in `contactAddressFromNeutral`
(`backend/models/contact_record_reverse.go:306-308`):

```go
if len(a.Contexts) > 0 {
    if tok, ok := contextToTypeToken[a.Contexts[0]]; ok {
        out.Type = tok
    } else {
        out.Type = a.Contexts[0]
    }
}
```

The unmapped fallback is load-bearing: `contact_record.go:643-647` deliberately allows arbitrary free text
in `Contexts`, so an unrecognised value must pass through unchanged rather than being blanked.

`contextToTypeToken` currently lives in `package vcard4`. Move it (and `typeTokenToContext` with it, so the
pair stays together) to `backend/models` and have the adapter reference it from there, or duplicate it in
`models` with a comment cross-referencing the adapter. Prefer moving — a second copy is exactly the
hardcoded-mirror problem `/CLAUDE.md` frontend trap #4 warns about, and there's no reason to import it here.

**Do not "fix" this in `DeviceContactMapper.kt`.** Emitting `"home"` from Android would put a non-neutral
token into `Contexts`, which is wrong for RFC 9553, would leave the web VCF path broken, and would
contradict the mapper's own comment at `:83-85`. `DeviceContactMapperTest.kt:185-211` pins the current
`"private"` output and is **correct as written** — leave it alone.

## Traps

- **This is a projection change on real production data.** Existing rows already have `Type: "private"`
  stored in the flat `addresses` JSON. The fix corrects new imports and any row that gets re-derived by a
  save, but does not retroactively rewrite stored rows. Decide explicitly whether to add a data migration;
  the recommendation is **no** — `private` displays as raw text but is not lost, users can edit it, and a
  blanket UPDATE over a JSON column on real data is a worse risk than the cosmetic problem it fixes. State
  this in the landing note.
- `/CLAUDE.md` backend trap #3: test through `RecordForContact`, not `RecordFromContact`.
- `/CLAUDE.md` backend trap #1: any persistence test uses
  `database.InitDB(filepath.Join(t.TempDir(), "x.db"))`, not `AutoMigrate`.

## Done when

- A VCF containing `ADR;TYPE=home:;;123 Fake St;Townesville;MO;55555;USA` imports with flat
  `Type: "home"`, and `ADR;TYPE=work` with `Type: "work"`.
- An address whose `Contexts[0]` is arbitrary free text (e.g. `"cabin"`) still projects that text verbatim.
- A full round trip — flat `home` → Card → flat — still yields `home`.
- Existing `vcard4` adapter tests still pass unchanged after the map moves packages.
- `cd backend && go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- The new test is hand-verified: reintroduce `out.Type = a.Contexts[0]` and confirm it fails
  (`/CLAUDE.md`, "Hand-verify your tests").
