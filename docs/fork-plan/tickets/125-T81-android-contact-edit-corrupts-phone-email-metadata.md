# T81 — Editing any contact on Android relabels every phone "cell" and drops email/phone metadata ⚠

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 4 — silent data corruption on the single most-used write path in the app |
| **Size** | S — the form's state shape for two fields, plus its load and save mapping |
| **Depends on** | Nothing. Split out of [M7](88-M7-android-contact-record-coverage.md) during its 2026-08-11 design pass so it can ship without waiting for the whole editor build-out. |
| **Status** | TO BE DONE |
| **Source** | Found in M7's design pass, 2026-08-11. M7 described this as "emails and phones silently discard type/label/preferred on edit"; reading the code showed it is worse than a discard — it actively rewrites values. |

## Why this exists

`ContactFormState` holds emails and phones as **plain strings**
(`feature/contacts/.../ContactFormViewModel.kt:45-46`):

```kotlin
val emails: List<String> = listOf(""),
val phones: List<String> = listOf(""),
```

They are loaded by throwing everything else away (`:262-263`):

```kotlin
val emails = card?.emails?.map { it.address ?: "" } ?: listOf("")
val phones = card?.phones?.map { it.number ?: "" } ?: listOf("")
```

…and rebuilt from scratch on save (`:83-95`):

```kotlin
emails = emails.…map { Email(address = it) }.ifEmpty { null },
phones = phones.…map { Phone(number = it, label = "cell") }.ifEmpty { null },
```

`toInput`'s own doc comment says it merges onto `base` precisely "so fields the form does not model
… survive a save — the backend PUT is a full overwrite, so a rebuild-from-scratch would silently
delete them." That reasoning is correct and is exactly what the emails/phones branches then violate:
they *are* modeled, badly, so the merge protects everything except them.

### What actually happens on every contact edit

Because `Email`/`Phone` are reconstructed rather than copied, each entry loses `id`, `contexts`,
`pref`, and `label`. Concretely, editing a contact's *name* on Android:

- **Rewrites every phone's label to `"cell"`.** A number the user labeled `work` or `home` comes back
  `cell`. This is not a dropped field — it is a wrong value written over a right one, and it is
  invisible until the user next looks at the web UI.
- **Clears `preferred` on every phone and email.** [T58](77-T58-preferred-phone-email-ui.md)'s
  preferred-contact flag is `pref`, which the rebuild never sets.
- **Drops `contexts`** (home/work) on both.
- **Drops each entry's `id`**, the backend's identity handle for that entry.
- **Drops `features`** on phones — which is what [T34](43-T34-contact-field-linking.md)'s
  SMS-vs-call action detection reads.

The hardcoded `label = "cell"` is deliberate and reasonable *for entries the form creates* — the
inline comment explains it mirrors the web form's `defaultType="cell"`, without which the detail
screen can never offer an SMS action. The bug is applying it to entries the form merely **loaded**.

## What to build

1. Change `ContactFormState.emails`/`phones` from `List<String>` to `List<Email>`/`List<Phone>`,
   holding the objects actually loaded from the record.
2. Load without narrowing — keep the entries as-is rather than mapping to their scalar.
3. Save by **copying**, never constructing: `entry.copy(address = newValue)` /
   `entry.copy(number = newValue)`. Every field the form doesn't surface rides along untouched.
4. Keep the `label = "cell"` default **only for newly added rows** — a blank entry the user just
   created has no label to preserve. An entry loaded with an existing label keeps it.
5. Drop trailing blank rows on save as today, but by filtering on the value field rather than by
   rebuilding the list.

This deliberately does **not** add any UI for label/preferred/contexts — that is
[M7](88-M7-android-contact-record-coverage.md)'s `MultiValueEditor`. This ticket only stops the form
from destroying data it never showed. The UI can follow later without re-doing this work.

## Traps

- **Don't "fix" this by adding the missing fields to the form.** That is M7, it is much bigger, and
  it leaves the corruption live in the meantime. Preserve-then-surface, in that order.
- **A blank new row has no `id`.** Sending `id: null` for a new entry and the real `id` for an
  existing one is the intended shape — don't synthesize placeholder ids.
- **`ifEmpty { null }` is load-bearing.** The backend PUT is a full overwrite; sending an empty list
  where the record had entries deletes them. Keep the null-vs-empty distinction exactly as it is.

## Done when

- Editing a contact's name on Android leaves every phone's label, `pref`, `contexts`, `features`,
  and `id` exactly as they were — verified by loading a contact that has a `work`-labeled preferred
  phone, editing only the name, saving, and re-reading the record.
- A newly added phone row still defaults to `cell` so T34's SMS action works.
- A regression test asserts the round-trip preserves `id` and `contexts`, hand-verified to fail
  against the current `toInput` per `/CLAUDE.md`.
- `./gradlew testDebugUnitTest`, `./gradlew lintDebug` and `./gradlew assembleDebug` green —
  the three steps `.github/workflows/android-tests.yml` actually runs.

## Landed 2026-08-12

`ContactFormState.emails`/`phones` are now `List<Email>`/`List<Phone>` — the loaded objects, not
scalars. Loading no longer narrows to `it.address`/`it.number`; saving copies onto the loaded
entry (`email.copy(address = trimmed)` / `phone.copy(number = trimmed)`) instead of reconstructing
a fresh `Email`/`Phone`, so `id`/`contexts`/`pref`/`features`/`label` all ride through untouched.

The UI/ViewModel contract changed from a full-list replace
(`onEmailsChange(List<String>)`/`onPhonesChange(List<String>)`) to index-based
edit/add/remove (`onEmailValueChange(index, value)`/`onEmailAdd()`/`onEmailRemove(index)`, mirrored
for phones) — not just a mechanical rename. A full-list replace re-indexes positionally, so
deleting a *middle* row would have silently reattached a surviving entry's metadata to whatever
value shifted into its old position; index-based operations remove the exact object at the given
index instead. `ContactFormScreen.kt`'s `StringListEditor` became a generic `ValueListEditor<T>`
taking a `valueOf: (T) -> String` extractor, so it renders either list without knowing about
`Email`/`Phone` beyond what's displayed. The `label = "cell"` default now lives only in
`onPhoneAdd()` — the single place a phone entry is genuinely new — instead of being applied
unconditionally to every phone on every save.

Two new `ContactFormViewModelTest` cases, hand-verified per `/CLAUDE.md`: reintroduced the old
reconstruct-from-scratch `toInput`, confirmed exactly those two tests failed (17/19 still passed),
reverted. Existing tests updated to the new index-based API; no behavior they covered changed.

**Hand-verified on a real device** (Pixel 8a): edited an existing contact's name only, confirmed
on web afterward that the phone's label/preferred/id were unchanged.

Landed via [PR #104](https://github.com/DrewBrunning/mycorrhizal-crm/pull/104).
