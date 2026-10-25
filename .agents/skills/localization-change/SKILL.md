---
name: localization-change
description: Use when adding or changing UI text, translations, plural forms, locale behavior, or a language-dependent format.
license: MIT
---

# Localization change

Outcome: the same key set in every locale, real translations (not English
placeholders), correct plural families, and a type-clean frontend. Read
`frontend/AGENTS.md` (trap #5) and `docs/development/frontend.md` first.

All five locale files (`en`, `de`, `es`, `fr`, `it`) under
`frontend/src/i18n/locales/` must carry the key. The test is the spec; do not
add a second key list.

## Steps

1. Add the key to `frontend/src/i18n/locales/en.json` and every other locale
   file in the same change. A key present in only one direction leaks English or
   dead weight, and `frontend/src/i18n/locales.test.ts` fails both ways.
2. Translate the value for each locale. Do not copy English or truncate a longer
   translation to fit; adjust the component layout instead.
3. For a count-bearing string, use a real i18next plural family (`_one`,
   `_other`, plus `_many` for `es`/`fr`/`it` where CLDR defines it). Never write
   `{{count}} contacts` as a bare string.
4. If the key becomes unused, remove it from every locale; the key-usage guard
   (`frontend/src/i18n/keyUsage.test.ts`) fails on an unused key.
5. If the change affects date/number formatting, update
   `frontend/src/i18n/muiLocale.ts` and its test rather than hardcoding a locale.
6. Run the locale test and the type check; both are the DoD.

## DoD

```bash
cd frontend && npx vitest run src/i18n/locales.test.ts
cd frontend && npx tsc --noEmit
```

Report any check you could not run; never imply it passed.

## Stop and ask

- A translation needs product copy the contributor cannot supply.
- A locale is missing from the set or is being added; that changes the
  supported-locale contract and the fact anchor.
- A plural rule needs a CLDR form the test does not model.

Canonical detail: `frontend/AGENTS.md`, `docs/development/frontend.md`,
`frontend/src/i18n/locales.test.ts`.
