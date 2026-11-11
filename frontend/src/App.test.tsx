import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import { searchAll } from './api/search';
import { isAdmin } from './auth';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import i18n from './i18n/config';

// Mirrors index.tsx's real provider composition -- App itself doesn't
// include these, they're only added at the root render call, so any
// component that reaches into their context (e.g. BrandLogo's
// useThemePreference, or a routed page's useAnnouncer()) needs the same
// wrapping here to render at all.
afterEach(() => {
  cleanup();
  localStorage.clear();
  window.history.replaceState({}, '', '/');
  // Keep the module-scope data router in sync with the reset URL, so this
  // file's not-found test cannot leak its location into later App tests.
  window.dispatchEvent(new PopStateEvent('popstate'));
  vi.clearAllMocks();
});

vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, isAdmin: vi.fn(), fetchAndCacheUserInfo: vi.fn().mockResolvedValue(null) };
});

// This file asserts the shell (routing, nav, layout), not page data. Some
// routes it renders still mount data-loading components (e.g. /settings/data
// mounts ContactFieldSettings + CustomFieldsSettings), and a real fetch that
// settles after the test environment has been torn down makes React dispatch a
// state update with no `window` -- Vitest then fails the whole file on the
// unhandled rejection even though every test passed (#2283 nightly). Reject
// immediately instead: callers take their normal error path within the test,
// nothing reaches the network, and no request can outlive the environment.
vi.mock('./api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/client')>();
  return {
    ...actual,
    apiFetch: vi.fn().mockRejectedValue(new Error('test: network disabled')),
  };
});

// The logged-in shell lands on the dashboard, which fires its own fetches on
// mount; stub them so the nav-entry tests are deterministic and quiet.
vi.mock('./api/dashboard', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/dashboard')>();
  return { ...actual, getDashboard: vi.fn().mockResolvedValue({}) };
});
// Standing shares (#2292): the nav badge reads the pending count; default none.
vi.mock('./api/shareGrants', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/shareGrants')>();
  return {
    ...actual,
    getShareGrantNotifications: vi.fn().mockResolvedValue({ pending_count: 0, notifications: [] }),
  };
});
vi.mock('./api/circles', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/circles')>();
  return { ...actual, listCircles: vi.fn().mockResolvedValue({ circles: [] }) };
});
// The AppBar search is covered in components/GlobalSearch.test.tsx; here
// searchAll only needs to be quiet.
vi.mock('./api/search', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/search')>();
  return {
    ...actual,
    searchAll: vi
      .fn()
      .mockResolvedValue({ query: '', contacts: [], notes: [], activities: [], entities: [] }),
  };
});

function renderApp() {
  return render(
    <AppThemeProvider>
      <DateFormatProvider>
        <SnackbarProvider>
          <AnnouncerProvider>
            <App />
          </AnnouncerProvider>
        </SnackbarProvider>
      </DateFormatProvider>
    </AppThemeProvider>,
  );
}

// AppContent only renders the drawer (and the admin nav items inside it) once
// a token exists, so plant a cached user_info row for the logged-in shell.
function setLoggedInUser(isAdminValue: boolean) {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'alice', is_admin: isAdminValue }),
  );
}

test('an unknown URL renders the not-found page instead of silently landing on the dashboard', () => {
  setLoggedInUser(false);
  window.history.pushState({}, '', '/definitely-not-a-route');
  // The data router is created at module scope; notify it of the new URL the
  // same way a browser Back/Forward would.
  window.dispatchEvent(new PopStateEvent('popstate'));

  renderApp();

  expect(screen.getByRole('heading', { level: 1, name: 'Page not found' })).toBeInTheDocument();
  expect(screen.getByRole('link', { name: 'Go to Contacts' })).toHaveAttribute('href', '/contacts');
  expect(screen.getByRole('link', { name: 'Go to Dashboard' })).toHaveAttribute('href', '/');
  expect(document.title).toBe('Page not found · Mycorrhizal CRM');
});

test('renders app component', () => {
  const { container } = renderApp();
  expect(container).toBeTruthy();
});

test('a non-admin user does not see the System status nav entry', () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  setLoggedInUser(false);

  renderApp();

  // Admin-only destinations are all hidden from non-admins.
  expect(screen.queryByText('System status')).not.toBeInTheDocument();
  expect(screen.queryByText('System events')).not.toBeInTheDocument();
  expect(screen.queryByText('Users')).not.toBeInTheDocument();
  // A non-admin destination is still there.
  expect(screen.getAllByText('Contacts').length).toBeGreaterThan(0);
});

test('an admin sees the System status nav entry', () => {
  vi.mocked(isAdmin).mockReturnValue(true);
  setLoggedInUser(true);

  renderApp();

  expect(screen.getByText('System status')).toBeInTheDocument();
  expect(screen.getByText('System events')).toBeInTheDocument();
});

test('typing a search query shows the clear button, which resets the query', () => {
  setLoggedInUser(false);
  renderApp();

  const searchBox = screen.getByPlaceholderText('Search contacts, notes, activities…');
  fireEvent.change(searchBox, { target: { value: 'alice' } });

  const clearButton = screen.getByRole('button', { name: 'Clear' });
  expect(clearButton).toBeInTheDocument();

  fireEvent.click(clearButton);

  expect((searchBox as HTMLInputElement).value).toBe('');
  expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();
});

// #960 / #2174: the AppBar search, mounted in the real shell. A slow response
// for an abandoned query must not overwrite the newer query's grouped results.
test('a stale AppBar search response does not clobber a newer query (#960)', async () => {
  setLoggedInUser(false);
  vi.useFakeTimers();
  try {
    let resolveSlow!: (v: Awaited<ReturnType<typeof searchAll>>) => void;
    const slowJo = new Promise<Awaited<ReturnType<typeof searchAll>>>((r) => {
      resolveSlow = r;
    });
    vi.mocked(searchAll)
      .mockReturnValueOnce(slowJo)
      .mockResolvedValueOnce({
        query: 'john',
        contacts: [{ id: 1, uid: 'u1', firstname: 'John', lastname: 'Smith' }],
        notes: [],
        activities: [],
        entities: [],
      });

    renderApp();
    const searchBox = screen.getByPlaceholderText('Search contacts, notes, activities…');

    fireEvent.focus(searchBox);
    fireEvent.change(searchBox, { target: { value: 'jo' } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    fireEvent.change(searchBox, { target: { value: 'john' } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });
    expect(screen.getByText('John Smith')).toBeInTheDocument();

    await act(async () => {
      resolveSlow({
        query: 'jo',
        contacts: [{ id: 2, uid: 'u2', firstname: 'Joanna', lastname: 'Doe' }],
        notes: [],
        activities: [],
        entities: [],
      });
    });
    expect(screen.queryByText('Joanna Doe')).not.toBeInTheDocument();
    expect(screen.getByText('John Smith')).toBeInTheDocument();
  } finally {
    vi.useRealTimers();
  }
});

// #1792: the brand logo used to sit after </nav>, outside every landmark, so
// axe's `region` rule flagged it on every route. It must live inside the
// navigation landmark.
test('the sidebar brand logo is inside the main navigation landmark (#1792)', () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  setLoggedInUser(false);

  renderApp();

  const logo = screen.getByAltText('Mycorrhizal CRM');
  const nav = logo.closest('nav');
  expect(nav).not.toBeNull();
  expect(nav).toHaveAttribute('aria-label', 'Main navigation');
});

// Issue #1915: with the shell served from the service worker cache but the API
// unreachable, the app must say it is offline.
test('shows the offline banner when the browser is offline', () => {
  const original = Object.getOwnPropertyDescriptor(window.navigator, 'onLine');
  Object.defineProperty(window.navigator, 'onLine', { configurable: true, value: false });
  try {
    setLoggedInUser(false);
    renderApp();

    expect(screen.getByText(i18n.t('app.offline.banner'))).toBeInTheDocument();
  } finally {
    if (original) Object.defineProperty(window.navigator, 'onLine', original);
  }
});

// Issue #1917: a pushState navigation must move focus off the activated link
// to the new page's heading, so assistive technology announces the new screen.
test('a client-side navigation moves focus to the new page heading', async () => {
  setLoggedInUser(false);
  renderApp();

  const contactsLinks = screen.getAllByRole('link', { name: 'Contacts' });
  fireEvent.click(contactsLinks[0]);

  await waitFor(() =>
    expect(document.activeElement).toBe(
      screen.getByRole('heading', { level: 1, name: 'Contacts' }),
    ),
  );
});

// #2154: Settings (/settings) is a path prefix of Data (/settings/data); the
// drawer must mark only the most specific matching destination as current.
function currentNavPaths(): string[] {
  const nav = screen.getByRole('navigation', { name: 'Main navigation' });
  return Array.from(nav.querySelectorAll('a[aria-current="page"]')).map(
    (a) => a.getAttribute('href') ?? '',
  );
}

function visit(path: string) {
  setLoggedInUser(false);
  window.history.pushState({}, '', path);
  window.dispatchEvent(new PopStateEvent('popstate'));
  renderApp();
}

test('at /settings/data only the Data nav item is current', () => {
  visit('/settings/data');
  expect(currentNavPaths()).toEqual(['/settings/data']);
});

test('at /settings only the Settings nav item is current', () => {
  visit('/settings');
  expect(currentNavPaths()).toEqual(['/settings']);
});

test('a /settings/<other> route with no nav item of its own keeps Settings current', () => {
  visit('/settings/security');
  expect(currentNavPaths()).toEqual(['/settings']);
});

test('a path that merely shares a string prefix with a nav item is not current for it', () => {
  visit('/contacts-archive');
  expect(currentNavPaths()).toEqual([]);
});

// #2175: Shares, Audit log, Data, Users, System events and System status are
// sub-items of Settings. URLs are unchanged; the group highlights with its child.
const SETTINGS_CHILD_ROUTES: { path: string; label: string; adminOnly: boolean }[] = [
  { path: '/shares', label: 'Shares', adminOnly: false },
  { path: '/shared-with-me', label: 'Shared with me', adminOnly: false },
  { path: '/shared-by-me', label: 'Shared by me', adminOnly: false },
  { path: '/audit', label: 'Audit log', adminOnly: false },
  { path: '/settings/data', label: 'Data', adminOnly: false },
  { path: '/users', label: 'Users', adminOnly: true },
  { path: '/system-events', label: 'System events', adminOnly: true },
  { path: '/system-status', label: 'System status', adminOnly: true },
];

function navLink(label: string): HTMLElement {
  const nav = screen.getByRole('navigation', { name: 'Main navigation' });
  const link = Array.from(nav.querySelectorAll('a')).find((a) => a.textContent === label);
  if (!link) throw new Error(`no nav link ${label}`);
  return link;
}

test.each(SETTINGS_CHILD_ROUTES)(
  '$path highlights Settings and its own sub-item, only one as current page',
  ({ path, label }) => {
    vi.mocked(isAdmin).mockReturnValue(true);
    setLoggedInUser(true);
    window.history.pushState({}, '', path);
    window.dispatchEvent(new PopStateEvent('popstate'));
    renderApp();

    expect(currentNavPaths()).toEqual([path]);
    expect(navLink(label)).toHaveClass('Mui-selected');
    expect(navLink('Settings')).toHaveClass('Mui-selected');
    // The group is open on a child route so the user sees where they are.
    expect(screen.getByRole('button', { name: 'Settings sub-pages' })).toHaveAttribute(
      'aria-expanded',
      'true',
    );
  },
);

test('Settings does not highlight on an unrelated route and its group starts collapsed', () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  visit('/contacts');
  expect(navLink('Settings')).not.toHaveClass('Mui-selected');
  expect(screen.getByRole('button', { name: 'Settings sub-pages' })).toHaveAttribute(
    'aria-expanded',
    'false',
  );
});

test('the Settings toggle expands and collapses the sub-items', () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  visit('/contacts');
  const toggle = screen.getByRole('button', { name: 'Settings sub-pages' });
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'true');
  expect(navLink('Audit log')).toHaveAttribute('href', '/audit');
  fireEvent.click(toggle);
  expect(toggle).toHaveAttribute('aria-expanded', 'false');
});

test('admin-only sub-items are absent for non-admins and present for admins', () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  visit('/settings');
  for (const { label, adminOnly } of SETTINGS_CHILD_ROUTES) {
    const nav = screen.getByRole('navigation', { name: 'Main navigation' });
    const present = Array.from(nav.querySelectorAll('a')).some((a) => a.textContent === label);
    expect(present).toBe(!adminOnly);
  }
  cleanup();
  vi.mocked(isAdmin).mockReturnValue(true);
  setLoggedInUser(true);
  renderApp();
  expect(navLink('Users')).toHaveAttribute('href', '/users');
  expect(navLink('System status')).toHaveAttribute('href', '/system-status');
});

test('browser Back and Forward keep the highlight in step with the URL', async () => {
  vi.mocked(isAdmin).mockReturnValue(false);
  visit('/audit');
  expect(currentNavPaths()).toEqual(['/audit']);
  fireEvent.click(navLink('Data'));
  await waitFor(() => expect(currentNavPaths()).toEqual(['/settings/data']));
  await act(async () => {
    window.history.back();
    await new Promise((r) => setTimeout(r, 50));
  });
  await waitFor(() => expect(currentNavPaths()).toEqual(['/audit']));
  expect(navLink('Settings')).toHaveClass('Mui-selected');
});

// Standing shares (#2292, ADR 0042 decision 11): the recipient's pending count
// is a badge on the "Shared with me" entry, with an accessible name.
test('the Shared with me nav entry carries the pending-share badge', async () => {
  const { getShareGrantNotifications } = await import('./api/shareGrants');
  vi.mocked(getShareGrantNotifications).mockResolvedValue({ pending_count: 3, notifications: [] });
  setLoggedInUser(false);
  window.history.pushState({}, '', '/settings');
  window.dispatchEvent(new PopStateEvent('popstate'));
  renderApp();
  expect(await screen.findByLabelText('3 shares waiting for your answer')).toBeInTheDocument();
});

test('no badge when nothing is pending', async () => {
  const { getShareGrantNotifications } = await import('./api/shareGrants');
  vi.mocked(getShareGrantNotifications).mockResolvedValue({ pending_count: 0, notifications: [] });
  setLoggedInUser(false);
  window.history.pushState({}, '', '/settings');
  window.dispatchEvent(new PopStateEvent('popstate'));
  renderApp();
  await screen.findByRole('navigation', { name: 'Main navigation' });
  expect(screen.queryByLabelText(/waiting for your answer/)).toBeNull();
});
