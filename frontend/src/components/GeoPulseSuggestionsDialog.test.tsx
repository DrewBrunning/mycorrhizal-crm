import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { createActivity } from '../api/activities';
import { type Contact, getContacts } from '../api/contacts';
import {
  type GeoPulseStaySuggestion,
  type GeoPulseSuggestionsResponse,
  getGeoPulseSuggestions,
} from '../api/geopulse';
import GeoPulseSuggestionsDialog from './GeoPulseSuggestionsDialog';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(() => {
  cleanup();
  sessionStorage.clear();
});

vi.mock('../api/geopulse', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/geopulse')>();
  return { ...actual, getGeoPulseSuggestions: vi.fn() };
});
vi.mock('../api/activities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/activities')>();
  return { ...actual, createActivity: vi.fn() };
});
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContacts: vi.fn() };
});

function stay(overrides: Partial<GeoPulseStaySuggestion> = {}): GeoPulseStaySuggestion {
  return {
    stay_id: 7,
    external_ref: 'geopulse:stay:7',
    location: 'Cafe Nero',
    city: 'Leeds',
    country: 'UK',
    latitude: 53.8,
    longitude: -1.55,
    timestamp: '2026-09-20T14:00:00Z',
    duration_seconds: 45 * 60,
    photos: [],
    photos_unavailable: false,
    ...overrides,
  };
}

function response(...suggestions: GeoPulseStaySuggestion[]): GeoPulseSuggestionsResponse {
  return { date: '2026-09-20', suggestions };
}

function contact(): Contact {
  return { ID: 1, firstname: 'Alice', lastname: 'Johnson' };
}

beforeEach(() => {
  vi.mocked(getGeoPulseSuggestions).mockReset();
  vi.mocked(createActivity).mockReset();
  vi.mocked(createActivity).mockResolvedValue({} as never);
  vi.mocked(getContacts).mockReset();
  vi.mocked(getContacts).mockResolvedValue({ contacts: [contact()], next_cursor: '' } as never);
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

function renderDialog(props: Partial<React.ComponentProps<typeof GeoPulseSuggestionsDialog>> = {}) {
  const defaults = { open: true, onClose: vi.fn(), onLogged: vi.fn(), ...props };
  const utils = render(
    <MemoryRouter>
      <GeoPulseSuggestionsDialog {...defaults} />
    </MemoryRouter>,
  );
  return { ...utils, props: defaults };
}

async function lookUp(date = '2026-09-20') {
  fireEvent.change(screen.getByLabelText('Date'), { target: { value: date } });
  fireEvent.click(screen.getByRole('button', { name: 'Look up' }));
}

function localToday(): string {
  const d = new Date();
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

// The stay's own local calendar day, as the dialog derives it.
function stayDay(timestamp: string): string {
  const d = new Date(timestamp);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

test('opens with the explanation, today (local) as the date, and nothing looked up yet', () => {
  renderDialog();

  expect(screen.getByText('Log activity from location history')).toBeInTheDocument();
  expect(screen.getByText(/Nothing is saved until you log one/)).toBeInTheDocument();
  expect(screen.getByLabelText('Date')).toHaveValue(localToday());
  expect(getGeoPulseSuggestions).not.toHaveBeenCalled();
});

test('Look up is disabled without a date', () => {
  renderDialog();
  fireEvent.change(screen.getByLabelText('Date'), { target: { value: '' } });
  expect(screen.getByRole('button', { name: 'Look up' })).toBeDisabled();
});

test('looking up sends the chosen date and the browser timezone', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response());
  renderDialog();

  await lookUp('2026-09-20');

  await waitFor(() => expect(getGeoPulseSuggestions).toHaveBeenCalledTimes(1));
  const [date, timezone] = vi.mocked(getGeoPulseSuggestions).mock.calls[0];
  expect(date).toBe('2026-09-20');
  expect(timezone).toBe(Intl.DateTimeFormat().resolvedOptions().timeZone);
});

test('shows a loading state while the lookup is in flight', async () => {
  let resolve: (v: GeoPulseSuggestionsResponse) => void = () => {};
  vi.mocked(getGeoPulseSuggestions).mockImplementation(() => new Promise((r) => (resolve = r)));
  renderDialog();

  await lookUp();

  const button = await screen.findByRole('button', { name: 'Looking up…' });
  expect(button).toBeDisabled();
  expect(screen.getByRole('progressbar')).toBeInTheDocument();

  resolve(response());
  await waitFor(() => expect(screen.queryByRole('progressbar')).not.toBeInTheDocument());
});

test('lists each stay with its place, duration and region', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(
    response(
      stay(),
      stay({
        stay_id: 8,
        external_ref: 'geopulse:stay:8',
        location: 'The Pub',
        city: '',
        country: '',
        duration_seconds: 60 * 60,
      }),
      stay({
        stay_id: 9,
        external_ref: 'geopulse:stay:9',
        location: '',
        duration_seconds: 95 * 60,
      }),
      stay({
        stay_id: 10,
        external_ref: 'geopulse:stay:10',
        location: 'Blink',
        duration_seconds: 10,
      }),
    ),
  );
  renderDialog();
  await lookUp();

  expect(await screen.findByText('Cafe Nero')).toBeInTheDocument();
  expect(screen.getByText(/45 min · Leeds, UK/)).toBeInTheDocument();
  expect(screen.getByText('The Pub')).toBeInTheDocument();
  expect(screen.getByText(/1 h$/)).toBeInTheDocument();
  expect(screen.getByText('Unnamed place')).toBeInTheDocument();
  expect(screen.getByText(/1 h 35 min/)).toBeInTheDocument();
  // A sub-minute stay is shown as at least a minute, never "0 min".
  expect(screen.getByText(/1 min/)).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: /^Log activity at / })).toHaveLength(4);
});

test('shows photo file names, and says so when photos could not be checked', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(
    response(
      stay({
        photos: [
          { id: 'p1', file_name: 'IMG_1.jpg', taken_at: '2026-09-20T14:05:00Z' },
          { id: 'p2', file_name: '', taken_at: '2026-09-20T14:06:00Z' },
        ],
      }),
      stay({
        stay_id: 8,
        external_ref: 'geopulse:stay:8',
        location: 'Elsewhere',
        photos_unavailable: true,
      }),
      stay({
        stay_id: 9,
        external_ref: 'geopulse:stay:9',
        location: 'Quiet',
        photos_unavailable: false,
      }),
    ),
  );
  renderDialog();
  await lookUp();

  expect(await screen.findByText('Photos nearby (2)')).toBeInTheDocument();
  expect(screen.getByText('IMG_1.jpg')).toBeInTheDocument();
  expect(screen.getByText('p2')).toBeInTheDocument(); // falls back to the id when GeoPulse sends no name
  // Exactly one stay was un-checkable; the quiet one simply has no photos.
  expect(screen.getAllByText('Photos could not be checked.')).toHaveLength(1);
});

test('a stay that is already an Activity is marked logged and cannot be logged again', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(
    response(
      stay({ existing_activity_id: 99 }),
      stay({ stay_id: 8, external_ref: 'geopulse:stay:8', location: 'Fresh' }),
    ),
  );
  renderDialog();
  await lookUp();

  expect(await screen.findByText('Already logged')).toBeInTheDocument();
  expect(screen.getAllByRole('button', { name: /^Log activity at / })).toHaveLength(1);
});

test('a date with no stays says so', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response());
  renderDialog();
  await lookUp();

  expect(
    await screen.findByText('GeoPulse has no recorded stays for this date.'),
  ).toBeInTheDocument();
});

test('a failed lookup shows the reason and a way to the settings page', async () => {
  vi.mocked(getGeoPulseSuggestions).mockRejectedValue(
    new Error('GeoPulse API token is invalid, expired, or not configured'),
  );
  const { props } = renderDialog();
  await lookUp();

  expect(
    await screen.findByText(/GeoPulse API token is invalid, expired, or not configured/),
  ).toBeInTheDocument();
  const link = screen.getByRole('link', { name: 'Open settings' });
  expect(link).toHaveAttribute('href', '/settings');
  fireEvent.click(link);
  expect(props.onClose).toHaveBeenCalled();
});

test('logging a stay opens the activity form pre-filled, and confirming creates the activity with the stay reference', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  const { props } = renderDialog();
  await lookUp('2026-09-20');
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));

  // The activity form is pre-filled from the stay: place and date, nothing about contacts.
  const form = await screen.findByRole('dialog', { name: 'Add Activity' });
  expect(within(form).getByLabelText('Location')).toHaveValue('Cafe Nero');
  expect(within(form).getByLabelText('Date *')).toHaveValue(stayDay('2026-09-20T14:00:00Z'));
  expect(within(form).getByLabelText('Title *')).toHaveValue('');
  await waitFor(() => expect(getContacts).toHaveBeenCalled());

  fireEvent.change(within(form).getByLabelText('Title *'), {
    target: { value: 'Coffee with Alice' },
  });
  fireEvent.click(within(form).getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(createActivity).toHaveBeenCalledTimes(1));
  expect(createActivity).toHaveBeenCalledWith({
    title: 'Coffee with Alice',
    description: '',
    location: 'Cafe Nero',
    date: new Date(stayDay('2026-09-20T14:00:00Z')).toISOString(),
    contact_ids: [],
    external_ref: 'geopulse:stay:7',
  });
  expect(props.onLogged).toHaveBeenCalledTimes(1);

  // The form closes and the stay now reads as logged — no second lookup needed.
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Add Activity' })).not.toBeInTheDocument(),
  );
  expect(screen.getByText('Already logged')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /^Log activity at / })).not.toBeInTheDocument();
  expect(getGeoPulseSuggestions).toHaveBeenCalledTimes(1);
});

test('the contacts are only what the user picks', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  renderDialog();
  await lookUp();
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));

  const form = await screen.findByRole('dialog', { name: 'Add Activity' });
  await waitFor(() => expect(getContacts).toHaveBeenCalled());
  fireEvent.change(within(form).getByLabelText('Title *'), { target: { value: 'Coffee' } });
  fireEvent.mouseDown(within(form).getByLabelText('Contacts'));
  fireEvent.click(await screen.findByRole('option', { name: /Alice Johnson/ }));
  fireEvent.click(within(form).getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(createActivity).toHaveBeenCalled());
  expect(vi.mocked(createActivity).mock.calls[0][0].contact_ids).toEqual([1]);
});

test('a failed save keeps the form open and does not mark the stay logged', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  vi.mocked(createActivity).mockRejectedValue(new Error('boom'));
  const { props } = renderDialog();
  await lookUp();
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));

  const form = await screen.findByRole('dialog', { name: 'Add Activity' });
  fireEvent.change(within(form).getByLabelText('Title *'), { target: { value: 'Coffee' } });
  fireEvent.click(within(form).getByRole('button', { name: /^save$/i }));

  await waitFor(() => expect(createActivity).toHaveBeenCalled());
  expect(await within(form).findByText('Failed to save activity')).toBeInTheDocument();
  expect(props.onLogged).not.toHaveBeenCalled();
  expect(screen.queryByText('Already logged')).not.toBeInTheDocument();
});

test('cancelling the activity form returns to the list untouched', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  renderDialog();
  await lookUp();
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));

  const form = await screen.findByRole('dialog', { name: 'Add Activity' });
  fireEvent.click(within(form).getByRole('button', { name: 'Cancel' }));

  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Add Activity' })).not.toBeInTheDocument(),
  );
  expect(createActivity).not.toHaveBeenCalled();
  expect(screen.getByRole('button', { name: /^Log activity at / })).toBeInTheDocument();
});

test('closing the dialog forgets the lookup, the logged set and any open form', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  const onClose = vi.fn();
  const { rerender } = render(
    <MemoryRouter>
      <GeoPulseSuggestionsDialog open onClose={onClose} />
    </MemoryRouter>,
  );
  await lookUp();
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));
  await screen.findByRole('dialog', { name: 'Add Activity' });

  rerender(
    <MemoryRouter>
      <GeoPulseSuggestionsDialog open={false} onClose={onClose} />
    </MemoryRouter>,
  );
  await waitFor(() =>
    expect(screen.queryByRole('dialog', { name: 'Add Activity' })).not.toBeInTheDocument(),
  );

  rerender(
    <MemoryRouter>
      <GeoPulseSuggestionsDialog open onClose={onClose} />
    </MemoryRouter>,
  );
  expect(screen.queryByText('Cafe Nero')).not.toBeInTheDocument();
  expect(screen.queryByText('Already logged')).not.toBeInTheDocument();
});

test('Close calls onClose', () => {
  const { props } = renderDialog();
  fireEvent.click(screen.getByRole('button', { name: 'Close' }));
  expect(props.onClose).toHaveBeenCalled();
});

test('the activity form is pre-filled from the stay, not from a date typed after the lookup', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay()));
  renderDialog();
  await lookUp('2026-09-20');
  const logButton = await screen.findByRole('button', { name: /^Log activity at / });

  // The user fiddles with the date input but does not press Look up again.
  fireEvent.change(screen.getByLabelText('Date'), { target: { value: '2027-01-02' } });
  fireEvent.click(logButton);

  const form = await screen.findByRole('dialog', { name: 'Add Activity' });
  expect(within(form).getByLabelText('Date *')).toHaveValue(stayDay('2026-09-20T14:00:00Z'));
  expect(within(form).getByLabelText('Date *')).not.toHaveValue('2027-01-02');
});

test('each Log activity button names its place and time', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(
    response(
      stay(),
      stay({
        stay_id: 8,
        external_ref: 'geopulse:stay:8',
        location: '',
        timestamp: '2026-09-20T18:30:00Z',
      }),
    ),
  );
  renderDialog();
  await lookUp();

  const buttons = await screen.findAllByRole('button', { name: /^Log activity at / });
  expect(buttons).toHaveLength(2);
  expect(buttons[0]).toHaveAccessibleName(/^Log activity at Cafe Nero, .*\d/);
  expect(buttons[1]).toHaveAccessibleName(/^Log activity at Unnamed place, .*\d/);
  expect(buttons[0].getAttribute('aria-label')).not.toBe(buttons[1].getAttribute('aria-label'));
});

test('lookup outcomes are announced through a polite status region', async () => {
  let resolve: (v: GeoPulseSuggestionsResponse) => void = () => {};
  vi.mocked(getGeoPulseSuggestions).mockImplementation(() => new Promise((r) => (resolve = r)));
  renderDialog();

  const status = screen.getByRole('status');
  expect(status).toHaveAttribute('aria-live', 'polite');
  expect(status).toHaveTextContent('');

  await lookUp();
  await waitFor(() => expect(status).toHaveTextContent('Looking up…'));
  // The spinner carries an accessible name too.
  expect(screen.getByRole('progressbar')).toHaveAccessibleName('Looking up…');

  resolve(response(stay(), stay({ stay_id: 8, external_ref: 'geopulse:stay:8' })));
  await waitFor(() => expect(status).toHaveTextContent('Places found: 2'));
});

test('an empty lookup is announced as no places', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response());
  renderDialog();
  await lookUp();
  await waitFor(() => expect(screen.getByRole('status')).toHaveTextContent('No places found'));
});

test('a failed lookup stays announced by the error alert', async () => {
  vi.mocked(getGeoPulseSuggestions).mockRejectedValue(new Error('nope'));
  renderDialog();
  await lookUp();
  expect(await screen.findByRole('alert')).toHaveTextContent(/nope/);
  expect(screen.getByRole('status')).toHaveTextContent('');
});

test('an already-logged stay is perceivable as text, not just colour', async () => {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(stay({ existing_activity_id: 5 })));
  renderDialog();
  await lookUp();
  expect(await screen.findByText('Already logged')).toBeVisible();
});

// The Add Activity form falls back to the lookup's own date when the stay's day
// cannot be derived (unparseable timestamp, or Intl yielding no usable parts).
async function openFormFor(s: GeoPulseStaySuggestion) {
  vi.mocked(getGeoPulseSuggestions).mockResolvedValue(response(s));
  renderDialog();
  await lookUp();
  fireEvent.click(await screen.findByRole('button', { name: /^Log activity at / }));
  return within(await screen.findByRole('dialog', { name: 'Add Activity' }));
}

test('an unparseable stay timestamp pre-fills the lookup date', async () => {
  const form = await openFormFor(stay({ timestamp: 'not-a-time' }));
  expect(form.getByLabelText('Date *')).toHaveValue('2026-09-20');
});

test('a date formatter that yields no parts pre-fills the lookup date', async () => {
  const original = Intl.DateTimeFormat.prototype.formatToParts;
  const spy = vi.spyOn(Intl.DateTimeFormat.prototype, 'formatToParts').mockImplementation(() => []);
  try {
    const form = await openFormFor(stay());
    expect(form.getByLabelText('Date *')).toHaveValue('2026-09-20');
  } finally {
    spy.mockRestore();
    expect(Intl.DateTimeFormat.prototype.formatToParts).toBe(original);
  }
});

test('a date formatter that throws pre-fills the lookup date', async () => {
  const spy = vi.spyOn(Intl.DateTimeFormat.prototype, 'formatToParts').mockImplementation(() => {
    throw new RangeError('bad zone');
  });
  try {
    const form = await openFormFor(stay());
    expect(form.getByLabelText('Date *')).toHaveValue('2026-09-20');
  } finally {
    spy.mockRestore();
  }
});
