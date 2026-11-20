// Pseudo-locale generator (issue #2536).
//
// `qps` is the conventional pseudo-locale code. Accenting the vowels makes a
// translated string visibly different from a hardcoded one, and the 40% `~`
// padding reserves the space German/French expansion needs, so a layout that
// overflows is obvious without maintaining real translations of every key.
//
// The brackets are the delimiter the Playwright spec uses to tell translated
// text from literals. They are outside the padding so "starts with ⟦ and ends
// with ⟧" is also the cheap "already pseudo-localized" check below.

const OPEN = '⟦';
const CLOSE = '⟧';
const PAD_RATIO = 0.4;

const ACCENTS: Record<string, string> = {
  a: 'á',
  e: 'é',
  i: 'í',
  o: 'ó',
  u: 'ú',
  A: 'Á',
  E: 'É',
  I: 'Í',
  O: 'Ó',
  U: 'Ú',
};

// i18next resolves both after the initial translation: interpolation
// (`{{name}}`) and nesting (`$t(key)`). Accenting either would break the
// lookup or leave a literal placeholder on screen.
const TOKEN = /(\{\{[^{}]*\}\}|\$t\([^)]*\))/g;

/** Accents ASCII vowels; everything else (consonants, digits, punctuation) is left alone. */
function accent(text: string): string {
  return text.replace(/[aeiouAEIOU]/g, (letter) => ACCENTS[letter] ?? letter);
}

export function pseudoLocalize(value: string): string {
  // No-op rather than throwing on already-pseudo text: pseudoResources runs
  // over whole imported JSON trees, and a fixture that happens to contain
  // pseudo text must not crash the run. Re-padding would also silently inflate
  // lengths every time the generator is applied.
  if (value.startsWith(OPEN) && value.endsWith(CLOSE)) return value;

  // split() with a capturing group puts tokens at the odd indices, so only the
  // even segments are accented.
  const body = value
    .split(TOKEN)
    .map((part, index) => (index % 2 === 1 ? part : accent(part)))
    .join('');

  return `${OPEN}${body}${'~'.repeat(Math.ceil(body.length * PAD_RATIO))}${CLOSE}`;
}

/** Recursively transforms strings, arrays (preserved as arrays) and keyed objects. */
function pseudoValue(value: unknown): unknown {
  if (typeof value === 'string') return pseudoLocalize(value);
  if (Array.isArray(value)) return value.map(pseudoValue);
  if (value !== null && typeof value === 'object') return pseudoResources(value);
  return value;
}

/** Applies pseudoLocalize recursively to every leaf string, plural-suffix keys included. */
export function pseudoResources(en: object): object {
  const out: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(en)) out[key] = pseudoValue(value);
  return out;
}
