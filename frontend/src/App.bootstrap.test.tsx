import { cleanup, render, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import App from './App';
import { AppThemeProvider } from './AppThemeProvider';
import { fetchAndCacheUserInfo } from './auth';
import { AnnouncerProvider } from './context/AnnouncerContext';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import i18n from './i18n/config';

vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, fetchAndCacheUserInfo: vi.fn() };
});
vi.mock('./api/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/client')>();
  return { ...actual, apiFetch: vi.fn().mockRejectedValue(new Error('test: network disabled')) };
});

const fetchMock = vi.mocked(fetchAndCacheUserInfo);
const user = { user_id: 1, username: 'alice', is_admin: false, self_contact_vcard_uid: null };

beforeEach(async () => {
  fetchMock.mockReset();
  await i18n.changeLanguage('en');
});

afterEach(async () => {
  cleanup();
  localStorage.clear();
  sessionStorage.clear();
  await i18n.changeLanguage('en');
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

// #2718
test('a signed-out visit with empty storage never probes /users/me', async () => {
  renderApp();
  await new Promise((r) => setTimeout(r, 20));
  expect(fetchMock).not.toHaveBeenCalled();
});

test('returning from the SSO redirect probes /users/me once to restore the session', async () => {
  sessionStorage.setItem('oidc_pending', '1');
  fetchMock.mockResolvedValue(null);
  renderApp();
  await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  expect(sessionStorage.getItem('oidc_pending')).toBeNull();
});

// #2685
test('a signed-in browser applies the account language from /users/me on load', async () => {
  localStorage.setItem('user_info', JSON.stringify(user));
  await i18n.changeLanguage('en-US');
  fetchMock.mockResolvedValue({ ...user, language: 'de' });
  renderApp();
  await waitFor(() => expect(i18n.resolvedLanguage).toBe('de'));
  expect(document.documentElement.lang).toBe('de');
});

test('a failed refresh leaves the stored language alone', async () => {
  localStorage.setItem('user_info', JSON.stringify(user));
  await i18n.changeLanguage('fr');
  fetchMock.mockResolvedValue(null);
  renderApp();
  await waitFor(() => expect(fetchMock).toHaveBeenCalled());
  expect(i18n.resolvedLanguage).toBe('fr');
});
