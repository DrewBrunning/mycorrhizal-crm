import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import { isAdmin } from './auth';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';

// Mirrors index.tsx's real provider composition -- App itself doesn't
// include these, they're only added at the root render call, so any
// component that reaches into their context (e.g. BrandLogo's
// useThemePreference, or a routed page's useAnnouncer()) needs the same
// wrapping here to render at all.
afterEach(() => {
  cleanup();
  localStorage.clear();
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
