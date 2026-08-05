import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, within } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import { SnackbarProvider } from '../context/SnackbarContext';
import ContactInformation from './ContactInformation';
import { Card, CRMEnvelope } from '../api/contacts';
import { ContactFieldKey } from '../contactFields';

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

// ContactInformation now fetches the user's LinkFieldType registry on mount
// (T34, for social/other-online-service link resolution) — stub fetch to
// fail fast rather than hitting a real network, mirroring
// BuildVersionCard.test.tsx's own convention.
beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('network down'); }));
});

function renderInformation(
  card: Card = {},
  crm: CRMEnvelope = {},
  opts: { onUpdateCard?: (patch: Partial<Card>) => Promise<void>; enabledFields?: Set<ContactFieldKey> } = {}
) {
  const onUpdateCard = opts.onUpdateCard ?? vi.fn(async () => {});
  const result = render(
    <ThemeProvider theme={defaultTheme}>
      <SnackbarProvider>
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
            onUpdateCard={onUpdateCard}
            enabledFields={opts.enabledFields}
          />
        </DateFormatProvider>
      </SnackbarProvider>
    </ThemeProvider>
  );
  return { ...result, onUpdateCard };
}

// Scopes to the EditableArrayField row whose caption text matches `label`
// (e.g. "Phone") -- every row shares the generic "Edit"/copy aria-labels,
// so tests must not query those globally.
function fieldRow(label: string): HTMLElement {
  const caption = screen.getByText(label);
  const outerBox = caption.parentElement?.parentElement;
  if (!outerBox) throw new Error(`could not locate field row for "${label}"`);
  return outerBox as HTMLElement;
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

// --- T34: tappable fields + copy buttons ---

test('a cell phone gets call, text, and copy actions', () => {
  renderInformation({ phones: [{ number: '+15551234567', features: ['cell'] }] });
  const row = fieldRow('Phone');
  expect(within(row).getByLabelText('Call')).toHaveAttribute('href', 'tel:+15551234567');
  expect(within(row).getByLabelText('Text')).toHaveAttribute('href', 'sms:+15551234567');
  expect(within(row).getByLabelText('Copy Phone')).toBeInTheDocument();
});

test('a landline (home/work/other) phone gets call and copy, but no text action', () => {
  renderInformation({ phones: [{ number: '+15559876543', contexts: ['home'] }] });
  const row = fieldRow('Phone');
  expect(within(row).getByLabelText('Call')).toHaveAttribute('href', 'tel:+15559876543');
  expect(within(row).queryByLabelText('Text')).toBeNull();
  expect(within(row).getByLabelText('Copy Phone')).toBeInTheDocument();
});

test('a fax number gets copy only -- no call or text action', () => {
  renderInformation({ phones: [{ number: '+15550001111', features: ['fax'] }] });
  const row = fieldRow('Phone');
  expect(within(row).queryByLabelText('Call')).toBeNull();
  expect(within(row).queryByLabelText('Text')).toBeNull();
  expect(within(row).getByLabelText('Copy Phone')).toBeInTheDocument();
});

test('an email is itself a mailto: link, plus a copy action', () => {
  renderInformation({ emails: [{ address: 'alice@example.com', contexts: ['home'] }] });
  const row = fieldRow('Email');
  const link = within(row).getByText(/alice@example\.com/);
  expect(link.closest('a')).toHaveAttribute('href', 'mailto:alice@example.com');
  expect(within(row).getByLabelText('Copy Email')).toBeInTheDocument();
});

test('an address with no coordinates links to a map search built from the formatted address', () => {
  renderInformation({
    addresses: [{ components: [{ kind: 'locality', value: 'Springfield' }, { kind: 'region', value: 'IL' }] }],
  });
  const row = fieldRow('Address');
  const link = within(row).getByText(/Springfield, IL/);
  expect(link.closest('a')).toHaveAttribute(
    'href',
    'https://maps.google.com/?q=' + encodeURIComponent('Springfield, IL')
  );
  expect(within(row).getByLabelText('Copy Address')).toBeInTheDocument();
});

test('an address with coordinates links to the geo: URI directly', () => {
  renderInformation({
    addresses: [{ components: [{ kind: 'locality', value: 'Springfield' }], coordinates: 'geo:39.78,-89.65' }],
  });
  const row = fieldRow('Address');
  const link = within(row).getByText(/Springfield/);
  expect(link.closest('a')).toHaveAttribute('href', 'geo:39.78,-89.65');
});

test('a raw link (Card.Links) is directly tappable, plus a copy action', () => {
  renderInformation(
    { links: [{ uri: 'https://example.com/profile', contexts: ['home'] }] },
    {},
    { enabledFields: new Set<ContactFieldKey>(['links']) }
  );
  const row = fieldRow('Websites');
  const link = within(row).getByText(/https:\/\/example\.com\/profile/);
  expect(link.closest('a')).toHaveAttribute('href', 'https://example.com/profile');
  expect(within(row).getByLabelText('Copy Websites')).toBeInTheDocument();
});

test('a social profile with no resolvable link renders as text with a copy action only', () => {
  renderInformation(
    { socialProfiles: [{ service: 'SomeUnknownService', user: 'alice' }] },
    {},
    { enabledFields: new Set<ContactFieldKey>(['socialProfiles']) }
  );
  const row = fieldRow('Social Profiles');
  const text = within(row).getByText(/SomeUnknownService: alice/);
  expect(text.closest('a')).toBeNull();
  expect(within(row).getByLabelText('Copy Social Profiles')).toBeInTheDocument();
});

// --- T34 regression: adding action buttons must not change the save round trip ---

test('editing and saving a phone unchanged still round-trips through the same adapter shape', async () => {
  const { onUpdateCard } = renderInformation({ phones: [{ number: '+15551234567', features: ['cell'] }] });
  const row = fieldRow('Phone');
  fireEvent.click(within(row).getByLabelText('Edit'));
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(onUpdateCard).toHaveBeenCalledTimes(1);
  // contexts is re-derived from type (itself derived from features[0]) by
  // the pre-existing cardPhonesToValues/valuesToCardPhones round trip --
  // unrelated to T34's display changes, just the adapter's own behavior.
  expect(onUpdateCard).toHaveBeenCalledWith({
    phones: [{ number: '+15551234567', features: ['cell'], contexts: ['cell'], pref: undefined, label: undefined }],
  });
});
