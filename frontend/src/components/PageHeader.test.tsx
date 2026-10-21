import Button from '@mui/material/Button';
import { createTheme, ThemeProvider } from '@mui/material/styles';
import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test } from 'vitest';
import PageHeader from './PageHeader';

afterEach(cleanup);

// Issue #1785: jsdom has no layout engine, so these tests pin the *layout
// contract* (wrapping row, shrinkable wrapping buttons) through computed style;
// e2e/mobileLayout.spec.ts proves the real no-horizontal-overflow outcome in a
// browser.
function renderHeader(ui: React.ReactElement) {
  return render(<ThemeProvider theme={createTheme()}>{ui}</ThemeProvider>);
}

test('renders the title as the page h1', () => {
  renderHeader(<PageHeader title="Systemereignisse" />);
  expect(screen.getByRole('heading', { level: 1, name: 'Systemereignisse' })).toBeTruthy();
  expect(screen.queryByTestId('page-header-actions')).toBeNull();
});

test('leading replaces the title block and keeps custom content', () => {
  renderHeader(<PageHeader title="ignored" leading={<h2>Overall status</h2>} />);
  expect(screen.getByRole('heading', { level: 2, name: 'Overall status' })).toBeTruthy();
  expect(screen.queryByRole('heading', { level: 1 })).toBeNull();
});

test('the header row and the actions block wrap instead of overflowing', () => {
  renderHeader(<PageHeader title="Titel" actions={<Button>Aktualisieren</Button>} />);
  const row = screen.getByTestId('page-header');
  expect(getComputedStyle(row).display).toBe('flex');
  expect(getComputedStyle(row).flexWrap).toBe('wrap');
  const actions = screen.getByTestId('page-header-actions');
  expect(getComputedStyle(actions).flexWrap).toBe('wrap');
  expect(getComputedStyle(actions).minWidth).toBe('0px');
});

test('action buttons may shrink and wrap their label', () => {
  renderHeader(
    <PageHeader
      title="Aktivitäten"
      actions={
        <>
          <Button>Aus Standortverlauf erfassen</Button>
          <Button>Aktivität hinzufügen</Button>
        </>
      }
    />,
  );
  for (const name of ['Aus Standortverlauf erfassen', 'Aktivität hinzufügen']) {
    const style = getComputedStyle(screen.getByRole('button', { name }));
    expect(style.whiteSpace).toBe('normal');
    expect(style.minWidth).toBe('0px');
    expect(style.maxWidth).toBe('100%');
  }
});

test('every action stays rendered and reachable', () => {
  renderHeader(
    <PageHeader
      title="Benutzer"
      actions={
        <>
          <Button>Benutzer hinzufügen</Button>
          <Button>Erinnerungs-E-Mails senden</Button>
        </>
      }
    />,
  );
  expect(screen.getAllByRole('button')).toHaveLength(2);
});

test('accepts a custom sx without dropping the wrapping layout', () => {
  renderHeader(<PageHeader title="T" sx={{ mb: 2 }} actions={<Button>A</Button>} />);
  const row = screen.getByTestId('page-header');
  expect(getComputedStyle(row).flexWrap).toBe('wrap');
  expect(getComputedStyle(row).marginBottom).toBe('16px');
});

test('accepts an sx array without dropping the wrapping layout', () => {
  renderHeader(<PageHeader title="T" sx={[{ mb: 3 }]} actions={<Button>A</Button>} />);
  const row = screen.getByTestId('page-header');
  expect(getComputedStyle(row).flexWrap).toBe('wrap');
  expect(getComputedStyle(row).marginBottom).toBe('24px');
});
