import i18next from 'i18next';
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
const allLocales: Record<string, Translations> = { en, de, es, fr, it };

// CLDR's "many" category only exists for these three of the app's languages;
// German and English use one/other. i18next resolves a missing many form by
// falling back, but that means a translator's carefully written "many" form
// can silently disappear, so require it explicitly where CLDR defines it.
const MANY_LOCALES = new Set(['es', 'fr', 'it']);

const PLURAL_SUFFIX = /_(zero|one|two|few|many|other)$/;

describe.each(Object.entries(allLocales))('%s plural forms', (name, locale) => {
  const keys = flatten(locale);

  // #1786: every count-bearing string must be a real i18next plural family,
  // not a bare "{{count}} contacts" (rendered as "1 contacts") or a "(s)".
  test('every {{count}} string has _one and _other siblings', () => {
    const missing: string[] = [];

    for (const [key, value] of keys) {
      if (!value.includes('{{count}}')) continue;
      const base = key.replace(PLURAL_SUFFIX, '');
      if (!keys.has(`${base}_one`)) missing.push(`${key}: missing ${base}_one`);
      if (!keys.has(`${base}_other`)) missing.push(`${key}: missing ${base}_other`);
    }

    expect(missing, `${name}.json has count strings that are not pluralised`).toEqual([]);
  });

  // A real plural family must actually use {{count}}; `lost_a_loved_one` is
  // just an enum value ending in "_one", not a half-migrated plural.
  const hasCount = (base: string) =>
    ['_one', '_other', '_many'].some((suffix) =>
      (keys.get(`${base}${suffix}`) ?? '').includes('{{count}}'),
    );

  test('plural families are complete (_one and _other)', () => {
    const incomplete: string[] = [];
    for (const key of keys.keys()) {
      if (key.endsWith('_one')) {
        const base = key.slice(0, -'_one'.length);
        if (hasCount(base) && !keys.has(`${base}_other`)) {
          incomplete.push(`${base}: missing _other`);
        }
      }
      if (key.endsWith('_other')) {
        const base = key.slice(0, -'_other'.length);
        if (hasCount(base) && !keys.has(`${base}_one`)) {
          incomplete.push(`${base}: missing _one`);
        }
      }
    }
    expect(incomplete, `${name}.json has half-migrated plural families`).toEqual([]);
  });

  test('has the CLDR many form where the language needs it', () => {
    if (!MANY_LOCALES.has(name)) return;
    const missing: string[] = [];
    for (const key of keys.keys()) {
      if (!key.endsWith('_one')) continue;
      const base = key.slice(0, -'_one'.length);
      if (hasCount(base) && keys.has(`${base}_other`) && !keys.has(`${base}_many`)) {
        missing.push(`${base}: missing _many`);
      }
    }
    expect(missing, `${name}.json is missing CLDR _many forms`).toEqual([]);
  });

  // "1 item(s)" and "activit(y/ies)" are the shorthand this issue exists to
  // remove; they cannot be translated correctly in any language.
  test('uses no parenthetical plural shorthand', () => {
    const hacks = [...keys].filter(([, value]) => /\((?:s|es|y\/ies)\)/i.test(value));
    expect(
      hacks.map(([key, value]) => `${key} = ${value}`),
      `${name}.json still fakes plurals with parentheses`,
    ).toEqual([]);
  });
});

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
    // _many is CLDR-only: en/de do not define the category, so it is a real,
    // used key in es/fr/it rather than an orphan.
    const orphaned = [...keys.keys()].filter(
      (k) => !enKeys.has(k) && !(MANY_LOCALES.has(name) && k.endsWith('_many')),
    );
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

// The structural checks above cannot tell whether i18next's runtime plural
// resolver actually selects the form we wrote. Exercise a representative
// singular, plural, and CLDR "many" lookup for every locale so a wrong suffix
// name (e.g. `_plural` or a missing `_many`) fails here, not in the UI.
describe.each(Object.entries(allLocales))('%s plural resolution', (name, locale) => {
  const keys = flatten(locale);

  test('selects singular, plural, and many forms at runtime', async () => {
    const instance = i18next.createInstance();
    await instance.init({
      lng: name,
      fallbackLng: 'en',
      resources: {
        [name]: { translation: locale },
        en: { translation: en },
      },
      interpolation: { escapeValue: false },
    });

    for (const base of [
      'duplicates.count',
      'prep.upcomingDates.inDays',
      'contactMerge.associationSummaryNotes',
    ]) {
      const one = instance.t(base, { count: 1 });
      const other = instance.t(base, { count: 2 });
      expect(one, `${name} ${base} did not select _one`).toBe(
        keys.get(`${base}_one`)?.replaceAll('{{count}}', '1'),
      );
      expect(other, `${name} ${base} did not select _other`).toBe(
        keys.get(`${base}_other`)?.replaceAll('{{count}}', '2'),
      );
      expect(one).not.toBe(other);
    }

    const million = instance.t('duplicates.count', { count: 1_000_000 });
    const expectedSuffix = MANY_LOCALES.has(name) ? '_many' : '_other';
    expect(million, `${name} did not select ${expectedSuffix} for 1,000,000`).toBe(
      keys.get(`duplicates.count${expectedSuffix}`)?.replaceAll('{{count}}', '1000000'),
    );
  });
});
