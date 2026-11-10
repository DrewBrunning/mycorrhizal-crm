import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import { isAdmin } from './auth';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import './i18n/config';

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
    searchAll: vi.fn().mockResolvedValue({ query: '', contacts: [], notes: [], activities: [] }),
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
