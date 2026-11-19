import i18n from './config';

// The UI languages that ship a locale file (see ./locales).
const SUPPORTED = ['en', 'de', 'es', 'fr', 'it'];

const base = (tag: string) => tag.toLowerCase().split(/[-_]/)[0];

// Apply the language saved on the account (#2685). The account preference wins
// over the browser's own language guess, so a change made on another device
// reaches this browser on its next load; a value this build cannot render is
// ignored rather than falling back to English, and a language that is already
// active is left alone so nothing re-renders.
export function applyAccountLanguage(language: string | undefined): void {
  if (!language) return;
  const wanted = base(language);
  if (!SUPPORTED.includes(wanted)) return;
  if (base(i18n.resolvedLanguage || i18n.language || '') === wanted) return;
  void i18n.changeLanguage(wanted);
}
