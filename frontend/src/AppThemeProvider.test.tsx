import { Alert, Autocomplete, TablePagination, TextField } from '@mui/material';
import { useTheme } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import { AppThemeProvider } from './AppThemeProvider';
import i18n from './i18n/config';

// #1787: MUI's component strings are theme options fragments, not i18next
// keys. These tests render real MUI components under AppThemeProvider so a
// regression in the locale wiring (or a theme memo that ignores the language)
// fails on the visible label, not on an internal object shape.
afterEach(async () => {
  cleanup();
  await i18n.changeLanguage('en');
});

function ThemeProbe() {
  const theme = useTheme();
  return (
    <>
      {/* Autocomplete's clear button only renders while the input is
          hovered/focused, so pin the theme value that feeds its aria-label. */}
      <span data-testid="mui-clear-text">
        {theme.components?.MuiAutocomplete?.defaultProps?.clearText as string | undefined}
      </span>
      <TablePagination
        component="div"
        count={10}
        page={0}
        rowsPerPage={10}
        onPageChange={() => {}}
        onRowsPerPageChange={() => {}}
      />
      <Autocomplete
        options={['Ada']}
        value="Ada"
        renderInput={(params) => <TextField {...params} label="Contact" />}
      />
      <Alert onClose={() => {}}>Message</Alert>
    </>
  );
}

const LABELS = [
  ['de', 'Zeilen pro Seite:', 'Leeren', 'Öffnen', 'Schließen'],
  ['es', 'Filas por página:', 'Limpiar', 'Abierto', 'Cerrar'],
  ['fr', 'Lignes par page :', 'Vider', 'Ouvrir', 'Fermer'],
  ['it', 'Righe per pagina:', 'Svuota', 'Apri', 'Chiudi'],
] as const;

test.each(LABELS)(
  'renders MUI built-in labels in %s while the app language is active',
  async (language, rowsPerPage, clear, open, close) => {
    await i18n.changeLanguage(language);
    render(
      <AppThemeProvider>
        <ThemeProbe />
      </AppThemeProvider>,
    );

    expect(await screen.findByText(rowsPerPage)).toBeInTheDocument();
    expect(screen.getByTestId('mui-clear-text')).toHaveTextContent(clear);
    expect(screen.getByRole('button', { name: open })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: close })).toBeInTheDocument();
  },
);

test('recreates the theme and MUI labels when the language changes', async () => {
  await i18n.changeLanguage('en');
  render(
    <AppThemeProvider>
      <ThemeProbe />
    </AppThemeProvider>,
  );
  expect(await screen.findByText('Rows per page:')).toBeInTheDocument();

  await i18n.changeLanguage('fr');

  expect(await screen.findByText('Lignes par page :')).toBeInTheDocument();
  expect(screen.queryByText('Rows per page:')).not.toBeInTheDocument();
});

test('an unsupported language falls back to English MUI labels', async () => {
  await i18n.changeLanguage('nl');
  render(
    <AppThemeProvider>
      <ThemeProbe />
    </AppThemeProvider>,
  );

  expect(await screen.findByText('Rows per page:')).toBeInTheDocument();
});
