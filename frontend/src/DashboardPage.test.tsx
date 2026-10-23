import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { listCircles } from './api/circles';
import { dismissContactSyncConflict, restoreContactSyncConflict } from './api/contactSyncConflicts';
import { type DashboardResponse, getDashboard } from './api/dashboard';
import { getUpcomingOccasions, type UpcomingOccasion } from './api/occasionObligations';
import { completeReminder, getUpcomingReminders, skipReminder } from './api/reminders';
import DashboardPage from './DashboardPage';
import { DateFormatProvider } from './DateFormatProvider';
import i18n from './i18n/config';

// M3: the page
// now fetches one GET /dashboard composite instead of fanning out to four
// endpoints plus a per-reminder contact lookup. This codebase's vitest has
// no auto-cleanup (CLAUDE.md frontend trap #1).
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/dashboard', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/dashboard')>();
  return { ...actual, getDashboard: vi.fn() };
});
vi.mock('./api/reminders', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/reminders')>();
  return {
    ...actual,
    getUpcomingReminders: vi.fn(),
    completeReminder: vi.fn(),
    skipReminder: vi.fn(),
  };
});
vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return { ...actual, listCircles: vi.fn() };
});
vi.mock('./api/contactSyncConflicts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contactSyncConflicts')>();
  return { ...actual, restoreContactSyncConflict: vi.fn(), dismissContactSyncConflict: vi.fn() };
});
vi.mock('./api/occasionObligations', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/occasionObligations')>();
  return { ...actual, getUpcomingOccasions: vi.fn() };
});

const getDashboardMock = vi.mocked(getDashboard);
const getUpcomingRemindersMock = vi.mocked(getUpcomingReminders);
const completeReminderMock = vi.mocked(completeReminder);
const skipReminderMock = vi.mocked(skipReminder);
const listCirclesMock = vi.mocked(listCircles);
const restoreSyncConflictMock = vi.mocked(restoreContactSyncConflict);
const dismissSyncConflictMock = vi.mocked(dismissContactSyncConflict);
const getUpcomingOccasionsMock = vi.mocked(getUpcomingOccasions);

function emptyDashboard(): DashboardResponse {
  return {
    birthdays: [],
    random_contacts: [],
    upcoming_reminders: [],
    overdue: [],
    favorites: [],
    reach_out_suggestions: [],
    contact_sync_conflicts: [],
    data_decay_overdue: [],
    birthdays_total: 0,
    upcoming_reminders_total: 0,
    overdue_total: 0,
    reach_out_suggestions_total: 0,
    contact_sync_conflicts_total: 0,
    data_decay_overdue_total: 0,
  };
}

beforeEach(() => {
  getDashboardMock.mockReset();
  getUpcomingRemindersMock.mockReset();
  completeReminderMock.mockReset();
  skipReminderMock.mockReset();
  listCirclesMock.mockReset();
  restoreSyncConflictMock.mockReset();
  dismissSyncConflictMock.mockReset();
  getUpcomingOccasionsMock.mockReset();
  getUpcomingOccasionsMock.mockResolvedValue({ occasions: [], days: 30 });
  listCirclesMock.mockResolvedValue({
    circles: [],
    total: 0,
    next_cursor: '',
    limit: 200,
    members: [],
  });
});

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/dashboard']}>
      <DateFormatProvider>
        <Routes>
          <Route path="/dashboard" element={<DashboardPage />} />
          <Route path="/contacts/:id" element={<div>CONTACT DETAIL PAGE</div>} />
        </Routes>
      </DateFormatProvider>
    </MemoryRouter>,
  );
}

test('a failed dashboard fetch shows the error with a Try again that reloads it (issue #1478)', async () => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  getDashboardMock.mockRejectedValueOnce(new Error('Service unavailable'));
  getDashboardMock.mockResolvedValueOnce(emptyDashboard());

  renderPage();

  expect(await screen.findByText('Service unavailable')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Try again' }));

  await waitFor(() => expect(screen.queryByText('Service unavailable')).not.toBeInTheDocument());
  expect(getDashboardMock).toHaveBeenCalledTimes(2);
  expect(screen.getByRole('heading', { name: 'Dashboard' })).toBeInTheDocument();
});

test('fetches the dashboard composite once and renders all four blocks', async () => {
  getDashboardMock.mockResolvedValue({
    birthdays: [{ type: 'contact', name: 'Bea Birthday', birthday: '--08-20', contact_id: 1 }],
    random_contacts: [{ ID: 2, firstname: 'Randy', lastname: 'Contact', archived: false }],
    favorites: [{ ID: 4, firstname: 'Fay', lastname: 'Vorite', is_favorite: true }],
    upcoming_reminders: [
      {
        ID: 9,
        message: 'Call Nicky',
        by_mail: false,
        remind_at: '2026-08-12T00:00:00Z',
        recurrence: 'once',
        reoccur_from_completion: true,
        completed: false,
        email_sent: false,
        contact_id: 3,
        contact_name: 'Nicky Name',
      },
    ],
    overdue: [],
    reach_out_suggestions: [],
    contact_sync_conflicts: [],
    data_decay_overdue: [],
    birthdays_total: 1,
    upcoming_reminders_total: 1,
    overdue_total: 0,
    reach_out_suggestions_total: 0,
    contact_sync_conflicts_total: 0,
    data_decay_overdue_total: 0,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Bea Birthday')).toBeInTheDocument());
  expect(screen.getByText('Randy Contact')).toBeInTheDocument();
  // Issue #173: the favorites block renders its contacts.
  expect(screen.getByText('Fay Vorite')).toBeInTheDocument();
  // #196: the decorative avatar initial must not double into a card link's
  // accessible name ("FFay Vorite") -- favorites, birthdays and stay-in-touch
  // all wrap an <Avatar> inside `component={Link}`.
  expect(screen.getByRole('link', { name: 'Fay Vorite' })).toBeInTheDocument();
  expect(screen.getByRole('link', { name: /^Bea Birthday/ })).toBeInTheDocument();
  expect(screen.getByRole('link', { name: /^Randy Contact/ })).toBeInTheDocument();
  expect(screen.getByText('Call Nicky')).toBeInTheDocument();
  // The reminder's contact_name is embedded server-side (M3 design decision
  // 2) -- no separate per-reminder contact fetch happens.
  expect(screen.getByText('Nicky Name')).toBeInTheDocument();
  expect(getDashboardMock).toHaveBeenCalledTimes(1);
});

test("empty dashboard renders each column's empty state without crashing", async () => {
  getDashboardMock.mockResolvedValue(emptyDashboard());

  renderPage();

  await waitFor(() => expect(screen.getByText('No upcoming birthdays')).toBeInTheDocument());
  expect(screen.getByText('No upcoming reminders')).toBeInTheDocument();
  // Issue #173: the favorites block's empty state.
  expect(screen.getByText('No favorites yet')).toBeInTheDocument();
});

test('completing a reminder refetches via the plain upcoming-reminders endpoint and keeps the known contact name', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    upcoming_reminders: [
      {
        ID: 9,
        message: 'Call Nicky',
        by_mail: false,
        remind_at: '2026-08-12T00:00:00Z',
        recurrence: 'once',
        reoccur_from_completion: true,
        completed: false,
        email_sent: false,
        contact_id: 3,
        contact_name: 'Nicky Name',
      },
    ],
  });
  completeReminderMock.mockResolvedValue({ message: 'ok' });
  getUpcomingRemindersMock.mockResolvedValue([
    {
      ID: 9,
      message: 'Call Nicky',
      by_mail: false,
      remind_at: '2026-08-19T00:00:00Z',
      recurrence: 'weekly',
      reoccur_from_completion: true,
      completed: false,
      email_sent: false,
      contact_id: 3,
    },
  ]);

  renderPage();

  await waitFor(() => expect(screen.getByText('Call Nicky')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Complete' }));

  await waitFor(() => expect(completeReminderMock).toHaveBeenCalledWith(9));
  await waitFor(() => expect(getUpcomingRemindersMock).toHaveBeenCalledTimes(1));
  // The refetched reminder carries no contact_name from the plain endpoint;
  // the page must carry the previously-known name forward.
  await waitFor(() => expect(screen.getByText('Nicky Name')).toBeInTheDocument());
});

test('renders a CardDAV sync conflict and can restore or dismiss it (issue #395)', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    contact_sync_conflicts: [
      {
        id: 'conflict-1',
        created_at: '2026-08-24T00:00:00Z',
        updated_at: '2026-08-24T00:00:00Z',
        subscription_id: 5,
        contact_id: 42,
        field: 'phone',
        local_value: '[{"type":"work","value":"555-0100"}]',
        remote_value: '[]',
        status: 'pending',
        contact_vcard_uid: 'uid-42',
        contact_name: 'Grace Hopper',
        subscription_name: 'Work address book',
      },
    ],
  });
  restoreSyncConflictMock.mockResolvedValue();
  dismissSyncConflictMock.mockResolvedValue();
  vi.spyOn(window, 'confirm').mockReturnValue(true);

  renderPage();

  await waitFor(() => expect(screen.getByText('Grace Hopper')).toBeInTheDocument());
  // The conflict notice names the field and offers the local value back
  // (the caption renders "Phone: 555-0100 → —").
  expect(screen.getByText(/Phone: 555-0100/)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: 'Restore local value' }));
  await waitFor(() => expect(restoreSyncConflictMock).toHaveBeenCalledWith('conflict-1'));
  await waitFor(() => expect(screen.queryByText('Grace Hopper')).not.toBeInTheDocument());
});

test('dismissing a sync conflict removes the notice', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    contact_sync_conflicts: [
      {
        id: 'conflict-2',
        created_at: '2026-08-24T00:00:00Z',
        updated_at: '2026-08-24T00:00:00Z',
        subscription_id: 5,
        contact_id: 43,
        field: 'job_title',
        local_value: 'Local Title',
        remote_value: 'Remote Title',
        status: 'pending',
        contact_vcard_uid: 'uid-43',
        contact_name: 'Ada Lovelace',
        subscription_name: 'Work address book',
      },
    ],
  });
  dismissSyncConflictMock.mockResolvedValue();

  renderPage();

  await waitFor(() => expect(screen.getByText('Ada Lovelace')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));
  await waitFor(() => expect(dismissSyncConflictMock).toHaveBeenCalledWith('conflict-2'));
  await waitFor(() => expect(screen.queryByText('Ada Lovelace')).not.toBeInTheDocument());
});

test('renders the upcoming occasions widget once the fetch succeeds', async () => {
  getDashboardMock.mockResolvedValue(emptyDashboard());
  const occasion: UpcomingOccasion = {
    contact_id: 9,
    contact_name: 'Freddie Mercury',
    source: 'birthday',
    label: 'Freddie Mercury',
    date: '2026-09-05',
    days_until: 3,
  };
  getUpcomingOccasionsMock.mockResolvedValue({ occasions: [occasion], days: 30 });

  renderPage();

  await waitFor(() => expect(getUpcomingOccasionsMock).toHaveBeenCalledWith({ days: 30 }));
  expect(await screen.findByText('Freddie Mercury')).toBeInTheDocument();
});

test('renders an overdue data decay policy and links to the contact (issue #352)', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    data_decay_overdue: [
      {
        policy: {
          id: 'decay-1',
          entity_id: 'uid-stale',
          interval_days: 365,
          active: true,
          created_at: '2025-01-01T00:00:00Z',
          updated_at: '2025-01-01T00:00:00Z',
        },
        health: { next_due: '2026-08-01T00:00:00Z', overdue_by: 40 },
        contact_id: 55,
        contact_name: 'Stale Sam',
      },
    ],
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Stale Sam')).toBeInTheDocument());
  expect(screen.getByText('40 days overdue')).toBeInTheDocument();
  const link = screen.getByRole('link', { name: /^Stale Sam/ });
  expect(link.getAttribute('href')).toBe('/contacts/55');
});

test('an all-clear data decay block is not rendered at all', async () => {
  getDashboardMock.mockResolvedValue(emptyDashboard());

  renderPage();

  await waitFor(() => expect(getDashboardMock).toHaveBeenCalled());
  expect(screen.queryByText('Info Needs a Check')).not.toBeInTheDocument();
});

// Issue #1791: the server caps every list block and reports the uncapped size,
// so a truncated block says so (and an un-truncated one stays quiet).
test('shows "showing N of total" under a block the server truncated', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    birthdays: [{ type: 'contact', name: 'Bea Birthday', birthday: '--08-20', contact_id: 1 }],
    birthdays_total: 40,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Bea Birthday')).toBeInTheDocument());
  expect(screen.getByText('Showing 1 of 40')).toBeInTheDocument();
});

test('shows no truncation note when every row fits', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    birthdays: [{ type: 'contact', name: 'Bea Birthday', birthday: '--08-20', contact_id: 1 }],
    birthdays_total: 1,
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Bea Birthday')).toBeInTheDocument());
  expect(screen.queryByText(/^Showing \d+ of \d+$/)).not.toBeInTheDocument();
});

// Issue #1791: the complete/skip refetch reads the plain, UNCAPPED reminders
// endpoint; the page must re-apply the section cap and refresh the total, or
// one completed reminder on a large account swaps 25 rows for all of them.
test('a reminder refetch after completion keeps the block capped and the total current', async () => {
  const reminder = (id: number) => ({
    ID: id,
    message: `Reminder ${id}`,
    by_mail: false,
    remind_at: '2026-08-12T00:00:00Z',
    recurrence: 'once' as const,
    reoccur_from_completion: true,
    completed: false,
    email_sent: false,
    contact_id: 3,
  });
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    upcoming_reminders: Array.from({ length: 25 }, (_, i) => ({
      ...reminder(i + 1),
      contact_name: 'Nicky Name',
    })),
    upcoming_reminders_total: 60,
  });
  completeReminderMock.mockResolvedValue({ message: 'ok' });
  getUpcomingRemindersMock.mockResolvedValue(Array.from({ length: 59 }, (_, i) => reminder(i + 2)));

  renderPage();

  await waitFor(() => expect(screen.getByText('Showing 25 of 60')).toBeInTheDocument());
  fireEvent.click(screen.getAllByRole('button', { name: 'Complete' })[0]);

  await waitFor(() => expect(screen.getByText('Showing 25 of 59')).toBeInTheDocument());
  expect(screen.getAllByRole('button', { name: 'Complete' })).toHaveLength(25);
  expect(screen.queryByText('Reminder 27')).not.toBeInTheDocument();
});

test('dismissing a sync conflict decrements its block total', async () => {
  const conflict = (id: string) => ({
    id,
    created_at: '2026-08-24T00:00:00Z',
    updated_at: '2026-08-24T00:00:00Z',
    subscription_id: 5,
    contact_id: 42,
    field: 'job_title' as const,
    local_value: 'Local Title',
    remote_value: 'Remote Title',
    status: 'pending' as const,
    contact_vcard_uid: `uid-${id}`,
    contact_name: `Conflict ${id}`,
    subscription_name: 'Work address book',
  });
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    contact_sync_conflicts: [conflict('c1'), conflict('c2')],
    contact_sync_conflicts_total: 30,
  });
  dismissSyncConflictMock.mockResolvedValue();

  renderPage();

  await waitFor(() => expect(screen.getByText('Showing 2 of 30')).toBeInTheDocument());
  fireEvent.click(screen.getAllByRole('button', { name: 'Dismiss' })[0]);
  await waitFor(() => expect(screen.getByText('Showing 1 of 29')).toBeInTheDocument());
});

// Issue #1761: every Stay in Touch card explains why the contact is there,
// from the server's relationship day counts (the same cadence computation the
// cadence view uses).
test('Stay in Touch cards render a reason subtitle for each relationship shape', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    random_contacts: [
      {
        ID: 1,
        firstname: 'Overdue',
        lastname: 'Olive',
        relationship: { has_cadence: true, days_until_due: -5, days_since_last_interaction: 20 },
      },
      {
        ID: 2,
        firstname: 'Due',
        lastname: 'Dana',
        relationship: { has_cadence: true, days_until_due: 4, days_since_last_interaction: 10 },
      },
      {
        ID: 3,
        firstname: 'Today',
        lastname: 'Tara',
        relationship: { has_cadence: true, days_until_due: 0, days_since_last_interaction: 30 },
      },
      {
        ID: 4,
        firstname: 'Recent',
        lastname: 'Rui',
        relationship: { has_cadence: false, days_since_last_interaction: 0 },
      },
      {
        ID: 5,
        firstname: 'Quiet',
        lastname: 'Quinn',
        relationship: { has_cadence: false },
      },
      {
        ID: 6,
        firstname: 'Ago',
        lastname: 'Ada',
        relationship: { has_cadence: false, days_since_last_interaction: 3 },
      },
    ],
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Overdue Olive')).toBeInTheDocument());
  expect(screen.getByText('5 days overdue')).toBeInTheDocument();
  expect(screen.getByText('Due in 4 days')).toBeInTheDocument();
  expect(screen.getByText('Due today')).toBeInTheDocument();
  expect(screen.getByText('Last talked today')).toBeInTheDocument();
  expect(screen.getByText('No interactions yet')).toBeInTheDocument();
  expect(screen.getByText('Last talked 3 days ago')).toBeInTheDocument();
});

test('Stay in Touch pluralizes the last-talked and due-in lines', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    random_contacts: [
      {
        ID: 1,
        firstname: 'One',
        lastname: 'Day',
        relationship: { has_cadence: true, days_until_due: 1, days_since_last_interaction: 1 },
      },
    ],
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('One Day')).toBeInTheDocument());
  expect(screen.getByText('Due in 1 day')).toBeInTheDocument();
});

test('Stay in Touch falls back to a generic reason on an older server', async () => {
  getDashboardMock.mockResolvedValue({
    ...emptyDashboard(),
    random_contacts: [{ ID: 1, firstname: 'Legacy', lastname: 'Lena' }],
  });

  renderPage();

  await waitFor(() => expect(screen.getByText('Legacy Lena')).toBeInTheDocument());
  expect(screen.getByText('Keep in touch')).toBeInTheDocument();
});

// Issue #1915: an offline reload should name the offline condition, never
// surface the raw browser fetch error to the user.
test('an offline dashboard load shows the offline message, not the raw fetch error', async () => {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value: false });
  try {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    getDashboardMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));

    renderPage();

    expect(await screen.findByText(i18n.t('common.offlineMessage'))).toBeInTheDocument();
    expect(screen.queryByText('Failed to fetch')).not.toBeInTheDocument();
  } finally {
    if (original) Object.defineProperty(window.navigator, 'onLine', original);
  }
});
