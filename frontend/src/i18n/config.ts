import i18n from 'i18next';
import LanguageDetector from 'i18next-browser-languagedetector';
import { initReactI18next } from 'react-i18next';
import deTranslations from './locales/de.json';
import enTranslations from './locales/en.json';
import esTranslations from './locales/es.json';
import frTranslations from './locales/fr.json';
import itTranslations from './locales/it.json';

// #211: keep <html lang> in sync with the active i18next language so
// assistive tech announces the page in the right language. `load:
// 'languageOnly'` below means `resolvedLanguage` is a bare code (`de`, not
// `de-DE`) -- already a valid BCP-47 tag, no further mapping needed.
const syncHtmlLang = (lng: string) => {
  document.documentElement.lang = lng;
};
i18n.on('languageChanged', syncHtmlLang);

// Suppress i18next's promotional console message (hardcoded since v23)
const noop = () => {};
const origLog = console.log;
console.log = noop;
i18n
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
  });

export default i18n;
