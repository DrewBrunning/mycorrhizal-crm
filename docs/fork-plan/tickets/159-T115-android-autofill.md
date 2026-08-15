# T115 — Android contact form fields don't offer autofill hints

| | |
|---|---|
| **Platform** | Android |
| **Rating** | 3 — real-world convenience for a form a user fills repeatedly; not a correctness bug |
| **Size** | M — touches the contact form's many text fields and the shared multi-value editor |
| **Depends on** | Nothing |
| **Status** | **DONE** (2026-08-15) |
| **Source** | Testing note: *"Android fields don't prompt for auto fill."* |

## Why this exists

`ContactFormScreen.kt` renders every field as a Material3 `OutlinedTextField` with no autofill metadata. None
of the form's `OutlinedTextField`s sets a Compose `AutofillType`, and there is no `AutofillNode`/`Modifier
.autofill` wiring, so the Android Autofill framework (Google Autofill, a password manager, or the on-device
address book) has no signal about what each field *is*, and offers no prompt or fill suggestions. Web has the
browser's native form semantics for free; the native app has to declare them.

The fields that should get hints and their matching `AutofillType` (from `androidx.compose.ui.autofill`):

- prefix → `PersonNamePrefix`
- given name → `PersonGivenName`
- middle name → `PersonMiddleName`
- surname → `PersonFamilyName`
- suffix → `PersonNameSuffix`
- email (in `MultiValueEditor`'s `EmailSpec`) → `EmailAddress`
- phone (in `MultiValueEditor`'s `PhoneSpec`) → `PhoneNumber`
- organization / department → `OrganizationName`
- address fields (in `AddressEditor`) → the `AddressLine`/`AddressCity`/`AddressState`/`AddressCountry`/
  `AddressPostalCode` family
- birthday → no standard person type; leave it (or `NewPassword`-adjacent is wrong — do not force a
  misleading type)

## What to build

Wire autofill through the Compose `Autofill` API so each form field advertises what it holds:

- Create a single `AutofillNode` for the form (or one per field — a single node with `onFill` dispatching by
  the focused field is simpler) and apply `Modifier.autofill(node, onFill = …)` on the fields, with the
  appropriate `AutofillType` list per field.
- `OutlinedTextField` has no built-in `autofill` parameter, so the modifier must be applied via the shared
  text-field wrapper; check whether `MultiValueEditor`/`AddressEditor` (`android/core/ui/…/components/`) need
  an `autofillTypes: List<AutofillType> = emptyList()` parameter added to their spec so the email/phone/
  address editors can pass hints through without every caller knowing about autofill.
- On fill, write the framework's value into the corresponding field's state through the existing
  `onXxxChange` callbacks (never mutate state directly), so a fill round-trips through the same validation/
  save path as a keystroke.

The hints should apply to the create/edit form; the login/registration screens are a separate surface and
only out of scope here unless trivial to include.

## Traps

- The email/phone fields live inside the generic `MultiValueEditor<T>` — adding `AutofillType` there must not
  change its API for the five other specs (titles, links, online services, personal info) that have no
  sensible autofill type. Prefer an optional per-spec field defaulting to none.
- Autofill is only meaningful on API 26+ (and behaves best 28+); the wiring must be a no-op (not a crash) on
  older APIs, which the framework classes already handle if you don't call them unconditionally.
- Compose's autofill requires the `androidx.compose.ui:ui` artifact (already a dependency) — do not add a new
  library.
- There is no device in the build environment for on-device verification (see recent M-series landing notes);
  the CI gate is `./gradlew testDebugUnitTest lintDebug` — unit-test that a fill callback writes the right
  field's state, and note that real prompt behaviour needs a device/manual pass.

## Done when

- The contact form's name/email/phone/organization/address fields advertise the correct `AutofillType`, and a
  fill (or a faked `AutofillNode` fill in a test) writes the value into the matching field's state.
- No other form (notes, activities, cadence, …) is affected, and the multi-value editor's non-email/phone
  specs keep their existing behaviour.
- `./gradlew testDebugUnitTest lintDebug` green, with a test asserting the fill callback routes to the right
  field — hand-verify it fails when the routing is broken.
- On-device autofill prompt verified by hand (a Pixel/emulator with Google Autofill) as the final manual step.

## Landing note (2026-08-15)

New `core/ui/.../components/AutofillOutlinedTextField.kt`: a thin wrapper over M3 `OutlinedTextField` that
registers an `AutofillNode` (Compose's public-but-experimental `LocalAutofill`/`LocalAutofillTree` API) with
the field's `AutofillType`, tracks its bounding box via `onGloballyPositioned`, requests autofill on focus and
cancels on blur, and routes a fill through the field's own `onValueChange`. It is deliberately guarded: no
Autofill service / API < 26 → `LocalAutofill` is null → pure no-op; the `requestAutofillForNode` call is
skipped until the field has been positioned (the framework errors on a null bounding box). Wired into the
contact form's prefix/given/middle/surname/suffix (PersonNamePrefix/FirstName/MiddleName/LastName/Suffix),
`MultiValueSpec.autofillType` (EmailSpec→EmailAddress, PhoneSpec→PhoneNumber, default null for the other
five specs), and the address editor's street/city/region/postal/country
(AddressStreet/AddressLocality/AddressRegion/PostalCode/AddressCountry).

Tests live in the existing `MultiValueEditorTest` (feature:contacts) and assert a field registers exactly one
node with the right `AutofillType` and that a fake `AutofillNode.onFill("Jordan")` writes into the field's
state; the no-type case asserts an empty hint list. Hand-verified: with `autofillTypes = emptyList()` forced,
the type assertion fails (`expected:<[PersonFirstName]> but was:<[]>`). **Two caveats.** (1) Compose 1.7.6's
autofill support is still experimental/incomplete (nothing sets `boundingBox` automatically — we do it
ourselves), so real prompt behaviour needs the on-device Google-Autofill pass the ticket lists as its final
manual step, which couldn't be done here (no device in the build env). (2) The `feature:contacts` module in
this environment does **not discover newly-added test files** (AGP 9.3.1/Gradle 9.5.0 quirk — new files in
`core:ui` and `feature:timeline` are found, existing files' new methods are found, but a brand-new file in
`feature:contacts` is silently never run, even after `clean`), which is why these tests were added to an
existing file rather than a new one; `:feature:contacts:testDebugUnitTest :core:ui:testDebugUnitTest
:feature:timeline:testDebugUnitTest` plus `lintDebug` and `:app:assembleDebug` are all green.

**Review pass (same day)** — `AutofillOutlinedTextField` initially tracked its bounding box in a Compose
`mutableStateOf`, which `onGloballyPositioned` wrote on every scroll/relayout of every autofill field (the
contact form has ~10 of them) — that recomposition churn was a real perf smell. The state is gone; the
modifier now writes `node.boundingBox` directly (a plain mutable field on `AutofillNode`, so no
recomposition), and the focus guard checks `node.boundingBox != null` instead of a state snapshot.


