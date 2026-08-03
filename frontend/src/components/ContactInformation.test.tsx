import { test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import ContactInformation from './ContactInformation';
import { Card, CRMEnvelope } from '../api/contacts';

// This codebase's vitest setup does not auto-cleanup between tests (no
// `globals: true`, setupTests.ts doesn't register it) -- without this,
// each test's render accumulates in the DOM and later tests see duplicate
// elements from earlier ones.
afterEach(cleanup);

const defaultTheme = createTheme();

function mockMatchMedia(matches: boolean) {
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
}

function renderInformation(card: Card = {}, crm: CRMEnvelope = {}) {
  return render(
    <ThemeProvider theme={defaultTheme}>
      <DateFormatProvider>
        <ContactInformation
          card={card}
          crm={crm}
          editingField={null}
          editValue=""
          validationError=""
          onEditStart={vi.fn()}
          onEditCancel={vi.fn()}
          onEditSave={vi.fn()}
          onEditValueChange={vi.fn()}
          onUpdateCard={vi.fn()}
        />
      </DateFormatProvider>
    </ThemeProvider>
  );
}

test('renders all four section tabs (T28)', () => {
  renderInformation();
  expect(screen.getByText('General Information')).toBeInTheDocument();
  expect(screen.getByText('Relationships')).toBeInTheDocument();
  expect(screen.getByText('Life Events')).toBeInTheDocument();
  expect(screen.getByText('Preferences')).toBeInTheDocument();
});

test('shows a dropdown Select for section navigation on mobile viewports (T28)', () => {
  mockMatchMedia(true);
  renderInformation();
  expect(screen.getByLabelText('contact information sections')).toBeInTheDocument();
  expect(screen.queryByRole('tablist')).toBeNull();
});

test('long unbroken email addresses wrap instead of overflowing (T28)', () => {
  const email = 'averyveryveryverylongunbrokenword@example.com';
  renderInformation({ emails: [{ address: email, contexts: ['home'] }] });
  // The row renders `value (type)`, so match the address as a substring.
  expect(screen.getByText(new RegExp(email))).toBeInTheDocument();
  // The email row's wrapping container carries the overflow-wrap:anywhere
  // rule so a single unbroken word cannot force horizontal scroll.
  const styles = Array.from(document.head.querySelectorAll('style'))
    .map((s) => s.textContent)
    .join('\n');
  expect(styles).toContain('overflow-wrap:anywhere');
});
