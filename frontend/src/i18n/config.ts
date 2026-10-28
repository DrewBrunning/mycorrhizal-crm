import i18n from 'i18next';
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
void i18n
  .use(LanguageDetector)
  .use(initReactI18next)
  .init({
    resources: {
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
    },
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
