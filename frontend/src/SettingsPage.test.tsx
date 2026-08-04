import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import './i18n/config';
import SettingsPage from './SettingsPage';
import { AppThemeProvider } from './AppThemeProvider';
import { DateFormatProvider } from './DateFormatProvider';
import { SnackbarProvider } from './context/SnackbarContext';
import { changePassword } from './api/auth';
import { getApiTokens, createApiToken, revokeApiToken, ApiToken } from './api/apiTokens';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(cleanup);

// WebhooksSettings and ImmichSettings are separately tested (their own
// *.test.tsx files) and fire their own API calls on mount — stub them out
// so this file can focus on password change and API tokens without needing
// to also mock their unrelated endpoints.
vi.mock('./components/WebhooksSettings', () => ({ default: () => null }));
vi.mock('./components/ImmichSettings', () => ({ default: () => null }));

vi.mock('./api/auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/auth')>();
  return { ...actual, changePassword: vi.fn() };
});
vi.mock('./api/apiTokens', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/apiTokens')>();
  return {
    ...actual,
    getApiTokens: vi.fn(),
    createApiToken: vi.fn(),
    revokeApiToken: vi.fn(),
  };
});

// jsdom doesn't implement matchMedia; AppThemeProvider's system-theme
// listener needs it to exist.
beforeEach(() => {
  window.matchMedia = window.matchMedia || (() => ({
    matches: false,
    addEventListener: () => {},
    removeEventListener: () => {},
    addListener: () => {},
    removeListener: () => {},
  }) as unknown as MediaQueryList);

  vi.mocked(changePassword).mockReset();
  vi.mocked(getApiTokens).mockReset();
  vi.mocked(createApiToken).mockReset();
  vi.mocked(revokeApiToken).mockReset();
  vi.mocked(getApiTokens).mockResolvedValue({ tokens: [] });
});

function apiToken(overrides: Partial<ApiToken> = {}): ApiToken {
  return {
    id: 1,
    name: 'CI token',
    created_at: '2026-01-01T00:00:00Z',
    last_used_at: null,
    revoked_at: null,
    expires_at: null,
    scope: 'full',
    ...overrides,
  };
}

function renderPage() {
  return render(
    <AppThemeProvider>
      <DateFormatProvider>
        <SnackbarProvider>
          <SettingsPage />
        </SnackbarProvider>
      </DateFormatProvider>
    </AppThemeProvider>
  );
}

// --- Password change ---------------------------------------------------

test('changing the password submits current/new and shows the success message', async () => {
  vi.mocked(changePassword).mockResolvedValue('Password updated successfully.');
  renderPage();

  fireEvent.change(screen.getByLabelText('Current password *'), { target: { value: 'old-pass' } });
  fireEvent.change(screen.getByLabelText('New password *'), { target: { value: 'new-pass-123' } });
  fireEvent.change(screen.getByLabelText('Confirm new password *'), { target: { value: 'new-pass-123' } });
  fireEvent.click(screen.getByRole('button', { name: /update password/i }));

  await waitFor(() => expect(changePassword).toHaveBeenCalledWith('old-pass', 'new-pass-123'));
  await waitFor(() => expect(screen.getByText('Password updated successfully.')).toBeInTheDocument());

  // The form clears the sensitive fields after a successful change.
  expect((screen.getByLabelText('Current password *') as HTMLInputElement).value).toBe('');
  expect((screen.getByLabelText('New password *') as HTMLInputElement).value).toBe('');
});

test('a mismatched confirmation is rejected before calling the API', async () => {
  renderPage();

  fireEvent.change(screen.getByLabelText('Current password *'), { target: { value: 'old-pass' } });
  fireEvent.change(screen.getByLabelText('New password *'), { target: { value: 'new-pass-123' } });
  fireEvent.change(screen.getByLabelText('Confirm new password *'), { target: { value: 'does-not-match' } });
  fireEvent.click(screen.getByRole('button', { name: /update password/i }));

  await waitFor(() => expect(screen.getByText(/do not match/i)).toBeInTheDocument());
  expect(changePassword).not.toHaveBeenCalled();
});

test('a rejected password change surfaces the server error', async () => {
  vi.mocked(changePassword).mockRejectedValue(new Error('current password is incorrect'));
  renderPage();

  fireEvent.change(screen.getByLabelText('Current password *'), { target: { value: 'wrong' } });
  fireEvent.change(screen.getByLabelText('New password *'), { target: { value: 'new-pass-123' } });
  fireEvent.change(screen.getByLabelText('Confirm new password *'), { target: { value: 'new-pass-123' } });
  fireEvent.click(screen.getByRole('button', { name: /update password/i }));

  await waitFor(() => expect(screen.getByText('current password is incorrect')).toBeInTheDocument());
});

// --- API tokens ----------------------------------------------------------

test('shows the empty state when there are no API tokens', async () => {
  renderPage();
  await waitFor(() => expect(screen.getByText(/no api tokens/i)).toBeInTheDocument());
});

test('lists an existing token with its scope and status', async () => {
  vi.mocked(getApiTokens).mockResolvedValue({ tokens: [apiToken({ name: 'Sync script', scope: 'carddav' })] });
  renderPage();

  await waitFor(() => expect(screen.getByText('Sync script')).toBeInTheDocument());
  expect(screen.getByText('CardDAV only')).toBeInTheDocument();
  expect(screen.getByText('Active')).toBeInTheDocument();
});

test('a revoked token is labeled and has no revoke action', async () => {
  vi.mocked(getApiTokens).mockResolvedValue({
    tokens: [apiToken({ name: 'Old token', revoked_at: '2026-01-02T00:00:00Z' })],
  });
  renderPage();

  await waitFor(() => expect(screen.getByText('Old token')).toBeInTheDocument());
  expect(screen.getByText('Revoked')).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: /revoke/i })).not.toBeInTheDocument();
});

test('creating a token shows the secret exactly once and refreshes the list', async () => {
  const created = { ...apiToken({ id: 5, name: 'New token' }), token: 'mcrh_live_abc123' };
  vi.mocked(createApiToken).mockResolvedValue(created);

  renderPage();
  await waitFor(() => expect(screen.getByText(/no api tokens/i)).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /create token/i }));
  fireEvent.change(screen.getByLabelText(/token name/i), { target: { value: 'New token' } });
  fireEvent.click(screen.getByRole('button', { name: /^create$/i }));

  await waitFor(() =>
    expect(createApiToken).toHaveBeenCalledWith('New token', 90, 'full')
  );
  await waitFor(() => expect(screen.getByDisplayValue('mcrh_live_abc123')).toBeInTheDocument());
  await waitFor(() => expect(getApiTokens).toHaveBeenCalledTimes(2), );
});

test('revoking a token requires confirmation before calling the API', async () => {
  vi.mocked(getApiTokens).mockResolvedValue({ tokens: [apiToken({ id: 3, name: 'To Revoke' })] });
  vi.mocked(revokeApiToken).mockResolvedValue(undefined);

  renderPage();
  await waitFor(() => expect(screen.getByText('To Revoke')).toBeInTheDocument());

  fireEvent.click(screen.getByRole('button', { name: /revoke/i }));
  expect(revokeApiToken).not.toHaveBeenCalled();
  expect(screen.getByText(/revoke token/i)).toBeInTheDocument();

  fireEvent.click(screen.getByRole('button', { name: /^revoke$/i }));

  await waitFor(() => expect(revokeApiToken).toHaveBeenCalledWith(3));
});
