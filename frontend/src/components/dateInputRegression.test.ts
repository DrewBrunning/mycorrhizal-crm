import { expect, test } from 'vitest';

// Issue #1916: a browser-native `<input type="date">` renders `mm/dd/yyyy` on
// WebKit regardless of the app's Date format preference, so it must never
// appear in the UI. DateField exists to replace it; this source-level guard
// catches a future dialog (or an edited one) quietly reintroducing the native
// control, which no preference-format assertion in one dialog's test would
// notice.
const sources = import.meta.glob(['./**/*.tsx', '!./**/*.test.tsx'], {
  query: '?raw',
  eager: true,
  import: 'default',
}) as Record<string, string>;

// DateField's own header comment quotes the native markup it replaces.
const DATE_FIELD = './DateField.tsx';
const NATIVE_DATE_INPUT = /type\s*=\s*(?:\{[^}]*['"]date['"][^}]*\}|["']date["'])/;

test('no component renders a native date input (issue #1916)', () => {
  const offenders = Object.entries(sources)
    .filter(([path, source]) => path !== DATE_FIELD && NATIVE_DATE_INPUT.test(source))
    .map(([path]) => path);

  expect(offenders).toEqual([]);
});
