import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import '../i18n/config';
import { AppThemeProvider } from '../AppThemeProvider';
import { notifySessionExpired } from '../api/sessionExpiry';
import { login2FA, loginUser, logoutAndRedirect } from '../auth';
import { useOIDCConfig } from '../hooks/useOIDCConfig';
import SessionExpiredGate from './SessionExpiredGate';

vi.mock('../auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../auth')>();
  return {
    ...actual,
    loginUser: vi.fn(),
    login2FA: vi.fn(),
    logoutAndRedirect: vi.fn(),
  };
});

vi.mock('../hooks/useOIDCConfig', () => ({ useOIDCConfig: vi.fn() }));

const loginUserMock = vi.mocked(loginUser);
const login2FAMock = vi.mocked(login2FA);
const logoutAndRedirectMock = vi.mocked(logoutAndRedirect);
const useOIDCConfigMock = vi.mocked(useOIDCConfig);

beforeEach(() => {
  useOIDCConfigMock.mockReturnValue({
    enabled: false,
    provider_name: 'SSO',
    registration_disabled: false,
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderGate() {
  return render(
    <AppThemeProvider>
      <SessionExpiredGate />
    </AppThemeProvider>,
  );
}

// Issue #557: replaces the old `window.location.href = '/login'` hard
// redirect on a 401. These pin the two modes apiFetch can signal and the
// in-place re-authentication flow, none of which should ever navigate away
// or unmount anything.
describe('SessionExpiredGate', () => {
  test('renders nothing until a session-expiry notification arrives', () => {
    renderGate();
    expect(screen.queryByText(/session/i)).not.toBeInTheDocument();
  });

  test('a passive notification shows a dismissible banner, not a modal', async () => {
    renderGate();

    notifySessionExpired('passive');

    await waitFor(() => expect(screen.getByText('Your session has expired.')).toBeInTheDocument());
    // The modal re-auth form must not be open for a passive notification.
    expect(screen.queryByLabelText(/username or email/i)).not.toBeInTheDocument();
  });

  test('a blocking notification opens the re-authentication modal immediately', async () => {
    renderGate();

    notifySessionExpired('blocking');

    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());
  });

  test('clicking "Sign in" on the passive banner opens the modal', async () => {
    renderGate();
    notifySessionExpired('passive');
    await waitFor(() => expect(screen.getByText('Your session has expired.')).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());
  });

  test('a successful sign-in closes the modal and the banner', async () => {
    loginUserMock.mockResolvedValue({});
    renderGate();
    notifySessionExpired('blocking');
    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/^password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() =>
      expect(screen.queryByLabelText(/username or email/i)).not.toBeInTheDocument(),
    );
    expect(screen.queryByText('Your session has expired.')).not.toBeInTheDocument();
  });

  test('a two-factor account steps to the code prompt, then closes on success', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    login2FAMock.mockResolvedValue({});
    renderGate();
    notifySessionExpired('blocking');
    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/^password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }));

    await waitFor(() => expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/verification code/i), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() =>
      expect(screen.queryByLabelText(/verification code/i)).not.toBeInTheDocument(),
    );
  });

  test('"Not now" closes the modal but leaves the banner up', async () => {
    renderGate();
    notifySessionExpired('blocking');
    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Not now' }));

    await waitFor(() =>
      expect(screen.queryByLabelText(/username or email/i)).not.toBeInTheDocument(),
    );
    expect(screen.getByText('Your session has expired.')).toBeInTheDocument();
  });

  test('"Log out" falls back to the documented hard-redirect path', async () => {
    renderGate();
    notifySessionExpired('blocking');
    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());

    fireEvent.click(screen.getByRole('button', { name: 'Log out' }));

    expect(logoutAndRedirectMock).toHaveBeenCalledTimes(1);
  });

  test('a passive notification never escalates an already-open blocking modal away', async () => {
    renderGate();
    notifySessionExpired('blocking');
    await waitFor(() => expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument());

    notifySessionExpired('passive');

    // Still open -- a later passive event must not silently close the modal
    // the user is already looking at.
    expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument();
  });
});
