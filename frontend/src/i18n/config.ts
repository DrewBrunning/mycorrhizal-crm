import i18n, { type Resource } from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';
// The `with` attribute keeps these JSON imports loadable by Playwright's native
// Node ESM loader (e2e fixtures reach this module via api/client.ts), which
// rejects a JSON module without it. Vite and tsc accept it unchanged.
import deTranslations from './locales/de.json' with { type: 'json' };
import enTranslations from './locales/en.json' with { type: 'json' };
import esTranslations from './locales/es.json' with { type: 'json' };
import frTranslations from './locales/fr.json' with { type: 'json' };
import itTranslations from './locales/it.json' with { type: 'json' };
import { pseudoResources } from './pseudo';

// #211: keep <html lang> in sync with the active i18next language so
// assistive tech announces the page in the right language. `load:
// 'languageOnly'` below means `resolvedLanguage` is a bare code (`de`, not
// `de-DE`) -- already a valid BCP-47 tag, no further mapping needed.
// Guarded: this module is also loaded by Node (Playwright's e2e fixtures reach
// it via api/client.ts), where there is no document to sync.
const syncHtmlLang = (lng: string) => {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = lng;
};
i18n.on('languageChanged', syncHtmlLang);

// Suppress i18next's promotional console message (hardcoded since v23)
const noop = () => {};
const origLog = console.log;
console.log = noop;

// #2536: `qps` is a runtime-only pseudo-locale, not a shipped language: it is
// added only when the browser already asks for it, so a normal visitor does
// not pay to generate ~4k pseudo strings. `load: 'languageOnly'` leaves the
// bare `qps` code intact, so the same shipped bundle can be tested as-is.
const resources: Resource = {
  en: {
    translation: enTranslations,
  },
  de: {
    translation: deTranslations,
  },
  it: {
    translation: itTranslations,
  },
  es: {
    translation: esTranslations,
  },
  fr: {
    translation: frTranslations,
  },
};

const wantsPseudoLocale = (() => {
  try {
    return typeof localStorage !== 'undefined' && localStorage.getItem('i18nextLng') === 'qps';
  } catch {
    // Storage can throw when it is disabled or partitioned; treat that as
    // "not requested" rather than failing app startup.
    return false;
  }
})();
if (wantsPseudoLocale) {
  resources.qps = { translation: pseudoResources(enTranslations) };
}

void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources,
    fallbackLng: 'en',
    load: 'languageOnly',
    debug: false,
    interpolation: {
      escapeValue: false,
    },
    detection: {
      order: ['localStorage', 'navigator'],
      caches: ['localStorage'],
    },
  })
  .then(() => {
    console.log = origLog;
    syncHtmlLang(i18n.resolvedLanguage || 'en');
  })
  .catch((err: unknown) => {
    // Restore the patched console.log even on init failure, or the
    // suppression above would silence every later console.log call.
    console.log = origLog;
    console.error('i18next init failed:', err);
  });

export default i18n;
