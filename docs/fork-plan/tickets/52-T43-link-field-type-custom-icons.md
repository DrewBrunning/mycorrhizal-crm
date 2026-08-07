# T43 — Link field type icons silently ignore any name outside a hardcoded 15-icon list

| | |
|---|---|
| **Rating** | 2 — cosmetic; the field still works correctly with the fallback icon |
| **Size** | S |
| **Depends on** | [T34](43-T34-contact-field-linking.md) (done — owns `LinkFieldType.Icon` and this file) |
| **Alpha** | n/a — frontend-only, no schema/API change |
| **Source** | Real-world usage report, 2026-08-06: a custom link field type's `icon` field accepts
  and stores an arbitrary MDI name (e.g. `mdiForumOutline`), but the icon never renders — it always
  falls back to the generic link icon |

## Why this exists

`LinkFieldType.Icon` is backend-validated only for length (`models/dtos.go:538`,
`validate:"max=100"`) — there is no backend allow-list, and the Settings icon picker
(`LinkFieldTypesSettings.tsx:288-296`) is a `freeSolo` `Autocomplete`, so a user can type and save
any MDI icon name. It round-trips through the API fine.

But `resolveLinkFieldTypeIcon` (`frontend/src/linkFieldTypeIcons.ts:9-53`) resolves icons through a
**hand-maintained, fixed lookup table** covering only the 15 icons used by this repo's own seeded
defaults:

```ts
import { mdiMessageLock, mdiFacebookMessenger, mdiWhatsapp, /* … 12 more */, mdiLinkVariant } from '@mdi/js';

export const LINK_FIELD_TYPE_ICONS: Record<string, string> = {
  mdiMessageLock, mdiFacebookMessenger, mdiWhatsapp, /* … 12 more, 15 total */
};
export const LINK_FIELD_TYPE_ICON_FALLBACK = mdiLinkVariant;
export const LINK_FIELD_TYPE_ICON_OPTIONS = Object.keys(LINK_FIELD_TYPE_ICONS);

export function resolveLinkFieldTypeIcon(icon: string | undefined): string {
  if (!icon) return LINK_FIELD_TYPE_ICON_FALLBACK;
  return LINK_FIELD_TYPE_ICONS[icon] || LINK_FIELD_TYPE_ICON_FALLBACK;
}
```

Any icon name that isn't one of those 15 exact keys — including a real, correctly-spelled
`@mdi/js` export like `mdiForumOutline` — silently falls back to `mdiLinkVariant` (line 52). The
Autocomplete's own suggestion list is capped to the same 15 (`LINK_FIELD_TYPE_ICON_OPTIONS`,
line 12), so nothing hints to the user that typing a valid MDI name won't work; it just quietly
doesn't.

Unlike `CLAUDE.md`'s usual frontend-trap-4 pattern (a frontend enum mirroring a backend `oneof`
validator, which *must* be a fixed, hand-synced list because the backend rejects anything else),
this field has **no backend enum to mirror** — the 100-char free-text field was a deliberate
exception (see T34's "Decisions made", frontend-trap-4 called out explicitly there). Capping icon
resolution to 15 names is therefore an unforced frontend limitation, not a consequence of the data
model.

## What to build

Replace the static lookup with a resolution against the *full* `@mdi/js` export set, keeping the
curated 15-name list only as Autocomplete suggestions, not as the sole resolvable set:

```ts
import * as mdiIcons from '@mdi/js';

export function resolveLinkFieldTypeIcon(icon: string | undefined): string {
  if (!icon) return LINK_FIELD_TYPE_ICON_FALLBACK;
  const path = (mdiIcons as Record<string, string>)[icon];
  return path || LINK_FIELD_TYPE_ICON_FALLBACK;
}
```

- Keep `LINK_FIELD_TYPE_ICONS`/`LINK_FIELD_TYPE_ICON_OPTIONS` as-is for the Autocomplete's curated
  suggestion list (typing/selecting one of the 15 should still be the easy path) — just stop using
  `LINK_FIELD_TYPE_ICONS` as the *only* resolvable set in `resolveLinkFieldTypeIcon`.
- Confirm `@mdi/js`'s bundled export surface doesn't get fully pulled into the production bundle by
  a wildcard import — check the built bundle size before/after, and use a named/lazy import
  strategy instead if `import * as mdiIcons from '@mdi/js'` turns out to defeat tree-shaking (it
  likely does, since every consumer of the module now references it dynamically by string key;
  `@mdi/js` is pure data with no side effects per-icon, so this is worth actually measuring rather
  than assuming either way).
- Consider validating the typed value live in the Autocomplete (green check / red x, or just
  rendering the resolved icon preview inline) so a typo is visible immediately instead of only
  discovered after save.

## Traps

- Don't change what happens for an *empty* or genuinely-unknown icon name — the fallback to
  `mdiLinkVariant` must stay for those; the fix only widens what counts as "known."
- `@mdi/js` names are exact-case (`mdiForumOutline`, not `mdiForumoutline` or `mdi-forum-outline`)
  — don't add case-insensitive matching that could resolve a typo to the wrong icon silently;
  fail closed to the fallback the way it already does.

## Done when

- `npx tsc --noEmit` clean, `npx vitest run` green, with a test proving a non-curated but valid
  `@mdi/js` name (e.g. `mdiForumOutline`) now resolves to that icon's actual path, not the
  fallback, alongside the existing curated-name and unknown-name cases.
- Hand-verified in a real browser: add a link field type in Settings with icon `mdiForumOutline`,
  confirm it renders correctly in both the Settings list row and any place the icon is shown on a
  contact's resolved social/messaging link.
- Bundle-size check recorded in the landing note (before/after, or a decision that a lazy/dynamic
  import was needed and why).
