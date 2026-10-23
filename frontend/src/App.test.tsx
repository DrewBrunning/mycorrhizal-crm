import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import type { Contact, ContactsResponse } from './api/contacts';
import { getContacts } from './api/contacts';
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
  return { ...actual, isAdmin: vi.fn() };
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
// The AppBar search test below drives getContacts's resolution order
// directly, so it needs a controllable mock rather than a real fetch.
vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return { ...actual, getContacts: vi.fn().mockResolvedValue({ contacts: [] }) };
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

  const searchBox = screen.getByPlaceholderText('Search Contacts');
  fireEvent.change(searchBox, { target: { value: 'alice' } });

  const clearButton = screen.getByRole('button', { name: 'Clear' });
  expect(clearButton).toBeInTheDocument();

  fireEvent.click(clearButton);

  expect((searchBox as HTMLInputElement).value).toBe('');
  expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();
});

function contact(id: number, firstname: string, lastname: string): Contact {
  return { ID: id, firstname, lastname };
}

// Mirrors useContacts.test.ts's overlapping-request helper (issue #556): two
// getContacts calls return promises this test resolves in whichever order it
// chooses, to simulate a slow response for an abandoned query landing after
// a faster response for the query that replaced it.
function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('a stale AppBar search response does not clobber a newer query (#960)', async () => {
  setLoggedInUser(false);
  vi.useFakeTimers();
  try {
    // Simulates typing "jo" then "john" before "jo"'s (slower) response
    // arrives: the second, newer request resolves first.
    const slowJo = deferred<ContactsResponse>();
    const fastJohn = deferred<ContactsResponse>();
    vi.mocked(getContacts)
      .mockReturnValueOnce(slowJo.promise)
      .mockReturnValueOnce(fastJohn.promise);

    renderApp();
    const searchBox = screen.getByPlaceholderText('Search Contacts');

    fireEvent.focus(searchBox);
    fireEvent.change(searchBox, { target: { value: 'jo' } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });

    fireEvent.change(searchBox, { target: { value: 'john' } });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(300);
    });

    await act(async () => {
      fastJohn.resolve({
        contacts: [contact(1, 'John', 'Smith')],
        next_cursor: '',
        limit: 10,
      });
    });
    expect(screen.getByText('John Smith')).toBeInTheDocument();

    // The stale "jo" response settling afterwards must be dropped, not
    // overwrite the list "john" already produced.
    await act(async () => {
      slowJo.resolve({
        contacts: [contact(2, 'Joanna', 'Doe')],
        next_cursor: '',
        limit: 10,
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
