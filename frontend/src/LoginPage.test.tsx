import { describe, test, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import './i18n/config';
import LoginPage from './LoginPage';
import { AppThemeProvider } from './AppThemeProvider';
import { loginUser, login2FA, isAuthenticated } from './auth';

vi.mock('./auth', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./auth')>();
  return { ...actual, loginUser: vi.fn(), login2FA: vi.fn(), isAuthenticated: vi.fn() };
});

vi.mock('./hooks/useOIDCConfig', () => ({
  useOIDCConfig: () => ({ enabled: false, provider_name: 'SSO' }),
}));

const loginUserMock = vi.mocked(loginUser);
const login2FAMock = vi.mocked(login2FA);
const isAuthenticatedMock = vi.mocked(isAuthenticated);

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

function renderLogin() {
  return render(
    <MemoryRouter>
      <AppThemeProvider>
        <LoginPage setToken={() => {}} />
      </AppThemeProvider>
    </MemoryRouter>
  );
}

// N8 (issue #158): the two-step login flow for 2FA-enabled accounts.
describe('LoginPage two-factor step', () => {
  test('shows the 2FA step when the account requires it', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    renderLogin();

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() =>
      expect(screen.getByText('Two-factor authentication')).toBeInTheDocument()
    );
    expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument();
    // No session must be considered authenticated yet.
    expect(isAuthenticatedMock).not.toHaveBeenCalled();
    // User info must not have been fetched/cached on the password-only step.
    expect(loginUserMock).toHaveBeenCalledTimes(1);
  });

  test('completes login with the code when 2FA is required', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    login2FAMock.mockResolvedValue({ language: 'en', date_format: 'eu' });
    isAuthenticatedMock.mockReturnValue(true);
    renderLogin();

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/verification code/i), { target: { value: '123456' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(login2FAMock).toHaveBeenCalledWith('123456'));
    expect(isAuthenticatedMock).toHaveBeenCalled();
  });

  test('shows an error and stays on the 2FA step for a rejected code', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    login2FAMock.mockRejectedValue(new Error('Invalid code'));
    renderLogin();

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument());

    fireEvent.change(screen.getByLabelText(/verification code/i), { target: { value: '000000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(screen.getByText('Invalid code. Please try again.')).toBeInTheDocument());
    expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument();
  });

  test('"Back to sign in" returns to the credentials form', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    renderLogin();

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument());
    fireEvent.click(screen.getByText('Back to sign in'));

    expect(screen.getByLabelText(/username or email/i)).toBeInTheDocument();
    expect(screen.queryByLabelText('Verification code *')).not.toBeInTheDocument();
  });
});

// Issue #192: a failed login used to drop keyboard focus to <body> with no
// programmatic link between the error and the fields it concerns.
describe('LoginPage failed-login focus and error association', () => {
  test('moves focus to the error and associates it with both fields on a rejected credentials submit', async () => {
    loginUserMock.mockRejectedValue(new Error('Invalid username or password'));
    renderLogin();

    const identifierInput = screen.getByLabelText(/username or email/i);
    const passwordInput = screen.getByLabelText(/^password/i);

    fireEvent.change(identifierInput, { target: { value: 'alice' } });
    fireEvent.change(passwordInput, { target: { value: 'wrong' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await screen.findByText('Invalid username or password');
    const alert = screen.getByRole('alert');
    await waitFor(() => expect(document.activeElement).toBe(alert));

    expect(identifierInput).toHaveAttribute('aria-describedby', alert.id);
    expect(passwordInput).toHaveAttribute('aria-describedby', alert.id);
    expect(identifierInput).toHaveAttribute('aria-invalid', 'true');
    expect(passwordInput).toHaveAttribute('aria-invalid', 'true');
  });

  test('moves focus to the error on a rejected 2FA code', async () => {
    loginUserMock.mockResolvedValue({ two_factor_required: true });
    login2FAMock.mockRejectedValue(new Error('Invalid code'));
    renderLogin();

    fireEvent.change(screen.getByLabelText(/username or email/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/^password/i), { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await waitFor(() => expect(screen.getByLabelText(/verification code/i)).toBeInTheDocument());
    fireEvent.change(screen.getByLabelText(/verification code/i), { target: { value: '000000' } });
    fireEvent.click(screen.getByRole('button', { name: 'Login' }));

    await screen.findByText('Invalid code. Please try again.');
    const alert = screen.getByRole('alert');
    await waitFor(() => expect(document.activeElement).toBe(alert));
  });
});
