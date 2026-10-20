// Issue #1779: with every data API answering 500, no page may present that as
// "you have no data". Each page/section here must show the shared LoadError (an
// announced role="alert" with Retry and the request ref), must NOT show its
// empty-state copy, and Retry must refetch in place. The fetch stub is the
// only seam, so the real hook -> API client -> error-message path is exercised.
import { ThemeProvider } from '@mui/material';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import type { ReactElement } from 'react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import ActivitiesPage from './ActivitiesPage';
import AuditPage from './AuditPage';
import CirclesTagsPage from './CirclesTagsPage';
import CircleTagTriagePage from './CircleTagTriagePage';
import ContactSharesPage from './ContactSharesPage';
import CalendarSyncSettings from './components/CalendarSyncSettings';
import CustomFieldsSettings from './components/CustomFieldsSettings';
import LinkFieldTypesSettings from './components/LinkFieldTypesSettings';
import PasskeySettings from './components/PasskeySettings';
import WebhooksSettings from './components/WebhooksSettings';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import HouseholdsPage from './HouseholdsPage';
import i18n from './i18n/config';
import NotesPage from './NotesPage';
import OccasionsPage from './OccasionsPage';
import SystemEventsPage from './SystemEventsPage';
import { lightTheme } from './theme';
import UsersPage from './UsersPage';

vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAdmin: vi.fn(() => true) };
});

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: true }),
  );
  // MUI's useMediaQuery needs matchMedia, which jsdom lacks.
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches: false,
    media: '',
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }));
  vi.spyOn(console, 'error').mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

function stubAllRequests500() {
  const fetchMock = vi.fn(async () => {
    const body = {
      error: { code: 'INTERNAL_ERROR', message: 'Simulated outage' },
      request_id: 'req-1779',
    };
    return {
      ok: false,
      status: 500,
      statusText: 'Internal Server Error',
      headers: new Headers({ 'content-type': 'application/json' }),
      json: async () => body,
      text: async () => JSON.stringify(body),
    };
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function renderWithProviders(ui: ReactElement) {
  return render(
    <ThemeProvider theme={lightTheme}>
      <SnackbarProvider>
        <AnnouncerProvider>
          <DateFormatProvider>
            <MemoryRouter>{ui}</MemoryRouter>
          </DateFormatProvider>
        </AnnouncerProvider>
      </SnackbarProvider>
    </ThemeProvider>,
  );
}

interface Case {
  name: string;
  ui: ReactElement;
  // i18n keys of the empty-state copy that must not appear on a failed load.
  empty: string[];
  // Whether the page's API module throws an ApiError, so a 5xx carries "(ref: <request id>)".
  // The handleResponse-based modules (admin, webauthn, webhooks, calendars) throw a plain
  // Error and have no request id to show.
  ref?: boolean;
}

const cases: Case[] = [
  { name: '/notes', ui: <NotesPage />, empty: ['notes.noNotes', 'notes.noResults'], ref: true },
  {
    name: '/activities',
    ui: <ActivitiesPage />,
    empty: ['activities.noActivities', 'activities.noResults'],
    ref: true,
  },
  {
    name: '/occasions',
    ui: <OccasionsPage />,
    empty: ['occasions.events.empty', 'occasions.giftShoppingList.empty'],
    ref: true,
  },
  { name: '/households', ui: <HouseholdsPage />, empty: ['household.empty'], ref: true },
  { name: '/circles', ui: <CirclesTagsPage />, empty: ['circlesTags.noCircles'], ref: true },
  {
    name: '/shares',
    ui: <ContactSharesPage />,
    empty: ['contactShares.incoming.empty', 'contactShares.outgoing.empty'],
    ref: true,
  },
  { name: '/audit', ui: <AuditPage />, empty: ['audit.emptyNoFilters', 'audit.empty'], ref: true },
  { name: '/users', ui: <UsersPage />, empty: ['users.noUsers'] },
  {
    name: '/system-events',
    ui: <SystemEventsPage />,
    empty: ['systemEvents.emptyNoFilters', 'systemEvents.empty'],
    ref: true,
  },
  {
    name: '/circle-tag-triage',
    ui: <CircleTagTriagePage />,
    empty: ['triage.noLegacyCircles'],
    ref: true,
  },
  { name: '/settings passkeys', ui: <PasskeySettings />, empty: ['settings.passkeys.empty'] },
  {
    name: '/settings webhooks',
    ui: <WebhooksSettings />,
    empty: ['settings.webhooks.noWebhooks'],
  },
  {
    name: '/settings/data custom fields',
    ui: <CustomFieldsSettings />,
    empty: ['settings.customFields.noFields'],
    ref: true,
  },
  {
    name: '/settings/data calendars',
    ui: <CalendarSyncSettings />,
    empty: ['settings.calendarSync.empty'],
  },
  {
    name: '/settings/data link field types',
    ui: <LinkFieldTypesSettings />,
    empty: ['settings.linkFieldTypes.empty'],
    ref: true,
  },
];

test.each(cases)(
  '$name: a 500 shows the shared error with Retry, not the empty state',
  async ({ ui, empty, ref }) => {
    const fetchMock = stubAllRequests500();
    renderWithProviders(ui);

    // The shared LoadError: announced (role=alert), titled, with the request ref.
    const alerts = await screen.findAllByText(i18n.t('common.loadFailed'));
    expect(alerts.length).toBeGreaterThan(0);
    if (ref) expect(screen.getAllByRole('alert')[0]).toHaveTextContent('req-1779');

    for (const key of empty) {
      expect(screen.queryByText(i18n.t(key))).not.toBeInTheDocument();
    }

    // Retry refetches in place (no reload): the request count goes up.
    const retry = screen.getAllByRole('button', { name: i18n.t('common.retry') })[0];
    const before = fetchMock.mock.calls.length;
    fireEvent.click(retry);
    await waitFor(() => expect(fetchMock.mock.calls.length).toBeGreaterThan(before));
  },
);

test('/circles?tab=tags: a 500 shows the shared error, not the empty tags copy', async () => {
  stubAllRequests500();
  render(
    <ThemeProvider theme={lightTheme}>
      <SnackbarProvider>
        <MemoryRouter initialEntries={['/circles?tab=tags']}>
          <CirclesTagsPage />
        </MemoryRouter>
      </SnackbarProvider>
    </ThemeProvider>,
  );

  expect((await screen.findAllByText(i18n.t('common.loadFailed'))).length).toBeGreaterThan(0);
  expect(screen.queryByText(i18n.t('circlesTags.noTags'))).not.toBeInTheDocument();
});

test('Notes: Retry recovers once the API is back, and the empty state is then legitimate', async () => {
  let healthy = false;
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => {
      if (!healthy) {
        const body = { error: { code: 'INTERNAL_ERROR', message: 'down' }, request_id: 'r1' };
        return {
          ok: false,
          status: 500,
          statusText: 'x',
          headers: new Headers(),
          json: async () => body,
          text: async () => JSON.stringify(body),
        };
      }
      const body = { notes: [], next_cursor: '', limit: 25, total: 0 };
      return {
        ok: true,
        status: 200,
        headers: new Headers({ 'content-type': 'application/json' }),
        json: async () => body,
        text: async () => JSON.stringify(body),
      };
    }),
  );
  renderWithProviders(<NotesPage />);

  await screen.findByRole('alert');
  expect(screen.queryByText(i18n.t('notes.noNotes'))).not.toBeInTheDocument();

  healthy = true;
  fireEvent.click(screen.getByRole('button', { name: i18n.t('common.retry') }));

  // A successful empty load is the one time the empty copy is true.
  expect(await screen.findByText(i18n.t('notes.noNotes'))).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});
