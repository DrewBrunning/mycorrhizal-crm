import { describe, expect, test } from 'vitest';
import de from './locales/de.json';
import en from './locales/en.json';
import es from './locales/es.json';
import fr from './locales/fr.json';
import it from './locales/it.json';

// Locale parity guard.
//
// CLAUDE.md requires all five locale files to carry real translations, but
// nothing enforced it, and three separate defects had accumulated unnoticed:
//
//   1. `contacts.personalInfo.kind` existed only in en, so the four other
//      locales fell back to English on a visible form label.
//   2. `activities.exportVCard` existed in de/es/fr/it but NOT en — an
//      orphan left behind when the key moved to contactDetail.exportVCard.
//   3. The entire `triage.*` block (25 keys) was untranslated English in all
//      four non-English locales, with the longer strings truncated rather
//      than translated.
//
// A key present in en and missing elsewhere leaks English (or the raw key
// path) to that user; a key present elsewhere and missing from en is dead
// weight that will never be read. Both directions are failures.

type Translations = Record<string, unknown>;

const locales: Record<string, Translations> = { de, es, fr, it };

/** Flattens a nested translation object to dotted leaf paths. */
function flatten(obj: Translations, prefix = ''): Map<string, string> {
  const out = new Map<string, string>();
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
      for (const [k, v] of flatten(value as Translations, path)) out.set(k, v);
    } else {
      out.set(path, String(value));
    }
  }
  return out;
}

const enKeys = flatten(en as Translations);

describe.each(Object.entries(locales))('%s locale', (name, locale) => {
  const keys = flatten(locale);

  test('has every key en has', () => {
    const missing = [...enKeys.keys()].filter((k) => !keys.has(k));
    expect(
      missing,
      `${name}.json is missing keys present in en.json — these leak English to the user`,
    ).toEqual([]);
  });

  test('has no keys en lacks', () => {
    const orphaned = [...keys.keys()].filter((k) => !enKeys.has(k));
    expect(
      orphaned,
      `${name}.json has keys absent from en.json — dead entries nothing will ever read`,
    ).toEqual([]);
  });

  // An interpolation placeholder that exists in en but not in the translation
  // renders as a literal gap in the UI ("items —  Circles"). Catching a
  // dropped {{count}} is the single highest-value structural check here.
  test('preserves every interpolation placeholder', () => {
    const placeholderRe = /\{\{(\w+)\}\}/g;
    const mismatches: string[] = [];

    for (const [key, enValue] of enKeys) {
      const translated = keys.get(key);
      if (translated === undefined) continue;

      const expected = [...enValue.matchAll(placeholderRe)].map((m) => m[1]).sort();
      const actual = [...translated.matchAll(placeholderRe)].map((m) => m[1]).sort();

      if (expected.join(',') !== actual.join(',')) {
        mismatches.push(`${key}: en has [${expected}], ${name} has [${actual}]`);
      }
    }

    expect(mismatches, `${name}.json changed interpolation placeholders`).toEqual([]);
  });
});

// A whole block left as English placeholders is the failure mode that actually
// shipped (triage.*). Identical strings are legitimate in bulk — proper nouns,
// cognates ("Contacts" in fr), format strings — so this cannot assert
// per-key difference. Instead it asserts no *namespace* is wholly untranslated,
// which is what a copy-paste-and-forget looks like.
describe('no namespace is left wholly untranslated', () => {
  // Namespaces whose values are legitimately identical across locales.
  const EXEMPT = new Set(['languages']);

  const namespaces = [...new Set([...enKeys.keys()].map((k) => k.split('.')[0]))];

  test.each(Object.entries(locales))('%s', (name, locale) => {
    const keys = flatten(locale);
    const untranslated: string[] = [];

    for (const ns of namespaces) {
      if (EXEMPT.has(ns)) continue;

      // Only consider blocks with enough substantial strings for "all
      // identical" to be meaningful rather than coincidental.
      const substantial = [...enKeys.entries()].filter(
        ([k, v]) => k.startsWith(`${ns}.`) && v.length > 3,
      );
      if (substantial.length < 5) continue;

      const allIdentical = substantial.every(([k, v]) => keys.get(k) === v);
      if (allIdentical) untranslated.push(`${ns} (${substantial.length} keys)`);
    }

    expect(
      untranslated,
      `these ${name} namespaces are byte-identical to English across every substantial key — they look like untranslated placeholders`,
    ).toEqual([]);
  });
});
