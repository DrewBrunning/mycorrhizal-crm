import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import en from './locales/en.json';
import { pseudoLocalize, pseudoResources } from './pseudo';

// Pseudo-localization guard (issue #2536).
//
// The key-parity tests can see a missing translation, but not a hardcoded
// string (it has no key) or a layout that overflows once German/French run
// 30-40% longer than English. `qps` makes both visible: every translated leaf
// is accented and padded, so any unaccented ASCII text on screen is a literal,
// and the `~` padding reserves the expansion space.
//
// These tests pin the generator's contract; the rendered-UI pass lives in the
// Playwright spec that follows in a separate PR.

type Translations = Record<string, unknown>;

/** Flattens a nested translation object to dotted leaf paths, indexing array elements. */
function flatten(obj: Translations, prefix = ''): Map<string, string> {
  const out = new Map<string, string>();
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    // Arrays recurse by index, matching pseudoResources: an array's elements are
    // leaves, not the array itself, so a shape change would be visible here too.
    if (value !== null && typeof value === 'object') {
      for (const [k, v] of flatten(value as Translations, path)) out.set(k, v);
    } else {
      out.set(path, String(value));
    }
  }
  return out;
}

describe('pseudoLocalize', () => {
  test('leaves i18next interpolation tokens intact', () => {
    const out = pseudoLocalize('You have {{count}} contacts');
    // The token must be byte-identical: i18next replaces it after this runs.
    expect(out).toContain('{{count}}');
    expect(out).not.toContain('{{cóúnt}}');
  });

  test('leaves $t() nesting tokens intact', () => {
    const out = pseudoLocalize('See $t(contacts.title) for details');
    expect(out).toContain('$t(contacts.title)');
  });

  test('wraps the result in pseudo-locale brackets and accents letters', () => {
    const out = pseudoLocalize('Add contact');
    expect(out.startsWith('⟦')).toBe(true);
    expect(out.endsWith('⟧')).toBe(true);
    expect(out).toContain('Ádd cóntáct');
  });

  test('pads output to at least 1.4x the source length', () => {
    const source = 'Save changes';
    expect(pseudoLocalize(source).length).toBeGreaterThanOrEqual(source.length * 1.4);
  });

  test('is a no-op on already pseudo-localized text', () => {
    const once = pseudoLocalize('Save');
    // No-op rather than throwing: pseudoResources recurses over JSON imported
    // by tests/fixtures, and a fixture that already contains pseudo text must
    // not crash the run. Re-padding would also silently inflate lengths.
    expect(pseudoLocalize(once)).toBe(once);
  });
});

describe('pseudoResources', () => {
  const enLeaves = flatten(en as Translations);

  test('transforms every leaf and preserves the leaf count', () => {
    const pseudoLeaves = flatten(pseudoResources(en as object) as Translations);

    expect(pseudoLeaves.size).toBe(enLeaves.size);

    const unchanged = [...enLeaves]
      .filter(([key, value]) => pseudoLeaves.get(key) === value)
      .map(([key]) => key);
    expect(unchanged, 'these leaves were not pseudo-localized').toEqual([]);
  });

  test('transforms plural-suffix keys like any other leaf', () => {
    const pluralKey = [...enLeaves.keys()].find((key) => key.endsWith('_one'));
    expect(pluralKey, 'en.json must contain at least one plural family').toBeDefined();

    const pseudoLeaves = flatten(pseudoResources(en as object) as Translations);
    const value = pseudoLeaves.get(pluralKey as string);
    expect(value).toBe(pseudoLocalize(enLeaves.get(pluralKey as string) as string));
  });

  test('preserves arrays and pseudo-localizes their elements', () => {
    // Synthetic: en.json has no array values today, but i18next supports them
    // for `returnObjects` keys. Treating an array as a keyed object would turn
    // it into `{ "0": ... }`, which renders as "[object Object]".
    const resources = { list: ['Add', 'Save'] };
    const out = pseudoResources(resources) as { list: unknown };

    expect(Array.isArray(out.list)).toBe(true);
    expect(out.list).toHaveLength(2);
    expect(out.list).toEqual([pseudoLocalize('Add'), pseudoLocalize('Save')]);
  });
});

// The runtime wiring is only exercised when the browser already asks for
// `qps`; a normal visitor must not pay to build ~4k pseudo strings.
describe('qps resource wiring', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    localStorage.clear();
    vi.resetModules();
  });

  test('adds a pseudo-localized qps resource when localStorage asks for it', async () => {
    localStorage.setItem('i18nextLng', 'qps');
    vi.resetModules();
    const i18n = (await import('./config')).default;

    expect(i18n.getResource('qps', 'translation', 'app.title')).toBe(pseudoLocalize(en.app.title));
    // Other locales stay real translations.
    expect(i18n.getResource('en', 'translation', 'app.title')).toBe(en.app.title);
  });

  test('does not add a qps resource for a normal locale', async () => {
    localStorage.setItem('i18nextLng', 'de');
    vi.resetModules();
    const i18n = (await import('./config')).default;

    expect(i18n.hasResourceBundle('qps', 'translation')).toBe(false);
  });
});
