import { deDE, enUS, esES, frFR, itIT } from '@mui/material/locale';
import type { ThemeOptions } from '@mui/material/styles';

// MUI ships its built-in component strings (TablePagination, Autocomplete,
// Alert, Pagination, …) as theme options fragments, not through i18next. Map
// the app's supported languages onto those fragments so one theme carries both
// the palette and the component labels. Keep this in sync with the locale files
// in this directory; an unsupported language falls back to English.
const MUI_LOCALES: Record<string, ThemeOptions> = {
  en: enUS,
  de: deDE,
  es: esES,
  fr: frFR,
  it: itIT,
};

export function muiLocaleFor(language: string | undefined): ThemeOptions {
  const base = (language ?? 'en').split('-')[0].toLowerCase();
  return MUI_LOCALES[base] ?? enUS;
}
