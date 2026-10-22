import { deDE, enUS, esES, frFR, itIT } from '@mui/material/locale';
import { expect, test } from 'vitest';
import { muiLocaleFor } from './muiLocale';

test.each([
  ['en', enUS],
  ['de', deDE],
  ['es', esES],
  ['fr', frFR],
  ['it', itIT],
] as const)('maps %s to its MUI locale fragment', (language, expected) => {
  expect(muiLocaleFor(language)).toBe(expected);
});

test('reduces a regional tag to its base language', () => {
  expect(muiLocaleFor('de-AT')).toBe(deDE);
  expect(muiLocaleFor('FR-ca')).toBe(frFR);
});

test('treats an absent language as English', () => {
  expect(muiLocaleFor(undefined)).toBe(enUS);
});

test('falls back to English for an unsupported language', () => {
  expect(muiLocaleFor('nl')).toBe(enUS);
});
