import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor, within } from '@testing-library/react';
import { ThemeProvider, createTheme } from '@mui/material/styles';
import '../i18n/config';
import { DateFormatProvider } from '../DateFormatProvider';
import { SnackbarProvider } from '../context/SnackbarContext';
import ContactInformation from './ContactInformation';
import { Card, CRMEnvelope } from '../api/contacts';
import { ContactFieldKey } from '../contactFields';
import { getLinkFieldTypes } from '../api/linkFieldTypes';

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

// ContactInformation fetches the user's LinkFieldType registry on mount
// (T34, gated on socialProfiles/otherOnlineServices being enabled) to
// resolve social/other-online-service handles to links. Mocking the API
// module directly (rather than stubbing global fetch to fail) lets tests
// control what the registry actually contains, so the resolution path
// itself gets exercised, not just its "nothing matched" fallback.
vi.mock('../api/linkFieldTypes', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/linkFieldTypes')>();
  return { ...actual, getLinkFieldTypes: vi.fn() };
});

beforeEach(() => {
  vi.mocked(getLinkFieldTypes).mockReset().mockResolvedValue([]);
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
  expect(within(row).getByLabelText(/^Call /)).toHaveAttribute('href', 'tel:+15551234567');
  expect(within(row).getByLabelText(/^Text /)).toHaveAttribute('href', 'sms:+15551234567');
  expect(within(row).getByLabelText(/^Copy Phone /)).toBeInTheDocument();
});

test('a landline (home/work/other) phone gets call and copy, but no text action', () => {
  renderInformation({ phones: [{ number: '+15559876543', contexts: ['home'] }] });
  const row = fieldRow('Phone');
  expect(within(row).getByLabelText(/^Call /)).toHaveAttribute('href', 'tel:+15559876543');
  expect(within(row).queryByLabelText(/^Text /)).toBeNull();
  expect(within(row).getByLabelText(/^Copy Phone /)).toBeInTheDocument();
});

test('a fax number gets copy only -- no call or text action', () => {
  renderInformation({ phones: [{ number: '+15550001111', features: ['fax'] }] });
  const row = fieldRow('Phone');
  expect(within(row).queryByLabelText(/^Call /)).toBeNull();
  expect(within(row).queryByLabelText(/^Text /)).toBeNull();
  expect(within(row).getByLabelText(/^Copy Phone /)).toBeInTheDocument();
});

// A combo device carrying both the "fax" and "cell" features -- the fax
// flag must win over both actions (no vendor actually texts a fax line).
test('a phone flagged both fax and cell suppresses call and text, per the fax override', () => {
  renderInformation({ phones: [{ number: '+15550009999', features: ['fax', 'cell'] }] });
  const row = fieldRow('Phone');
  expect(within(row).queryByLabelText(/^Call /)).toBeNull();
  expect(within(row).queryByLabelText(/^Text /)).toBeNull();
});

// Real-world CardDAV/vCard imports commonly carry TYPE=VOICE,CELL or
// TYPE=VOICE,FAX (multiple tokens) rather than a single bare "cell"/"fax" --
// cardPhonesToValues' derived `type` is only the FIRST token, so the button
// logic must check the full features/contexts arrays, not just `r.type`.
test('a multi-token cell number (features: [voice, cell]) still gets a text action', () => {
  renderInformation({ phones: [{ number: '+15551112222', features: ['voice', 'cell'] }] });
  const row = fieldRow('Phone');
  expect(within(row).getByLabelText(/^Text /)).toHaveAttribute('href', 'sms:+15551112222');
});

test('a multi-token fax number (features: [voice, fax]) does not get a call action', () => {
  renderInformation({ phones: [{ number: '+15553334444', features: ['voice', 'fax'] }] });
  const row = fieldRow('Phone');
  expect(within(row).queryByLabelText(/^Call /)).toBeNull();
});

test('an email is itself a mailto: link, plus a copy action', () => {
  renderInformation({ emails: [{ address: 'alice@example.com', contexts: ['home'] }] });
  const row = fieldRow('Email');
  const link = within(row).getByText(/alice@example\.com/);
  expect(link.closest('a')).toHaveAttribute('href', 'mailto:alice@example.com');
  expect(within(row).getByLabelText(/^Copy Email /)).toBeInTheDocument();
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
  expect(within(row).getByLabelText(/^Copy Address /)).toBeInTheDocument();
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
  expect(within(row).getByLabelText(/^Copy Websites /)).toBeInTheDocument();
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
  expect(within(row).getByLabelText(/^Copy Social Profiles /)).toBeInTheDocument();
});

// The "happy path" the registry exists for at all: a handle actually
// resolves to a working link once the user's LinkFieldType registry has a
// matching entry -- everything above this point only exercises the "no
// match" fallback.
test('a social profile resolves through the LinkFieldType registry to a working link', async () => {
  vi.mocked(getLinkFieldTypes).mockResolvedValue([
    {
      id: 'whatsapp-id', name: 'WhatsApp', protocol: 'https://wa.me/{value}', category: 'messaging',
      is_default: true, position: 0, created_at: '', updated_at: '',
    },
  ]);
  renderInformation(
    { socialProfiles: [{ service: 'WhatsApp', user: '15551234567' }] },
    {},
    { enabledFields: new Set<ContactFieldKey>(['socialProfiles']) }
  );
  expect(getLinkFieldTypes).toHaveBeenCalled();
  const row = fieldRow('Social Profiles');
  await waitFor(() => {
    const link = within(row).getByText(/WhatsApp: 15551234567/);
    expect(link.closest('a')).toHaveAttribute('href', 'https://wa.me/15551234567');
  });
});

// T34 finding: the registry fetch must not fire for accounts that never
// enable the fields it resolves -- socialProfiles/otherOnlineServices are
// both off by default (DEFAULT_ENABLED_CONTACT_FIELDS), and
// ListLinkFieldTypes lazily seeds 19 rows on a user's first call, so an
// unconditional fetch would be a write on every contact-page view for the
// common case.
test('does not fetch the LinkFieldType registry when neither social field is enabled', () => {
  renderInformation({ phones: [{ number: '+15551234567', features: ['cell'] }] });
  expect(getLinkFieldTypes).not.toHaveBeenCalled();
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
