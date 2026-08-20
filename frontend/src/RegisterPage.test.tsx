import { describe, test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import './i18n/config';
import RegisterPage from './RegisterPage';
import { AppThemeProvider } from './AppThemeProvider';
import { useOIDCConfig } from './hooks/useOIDCConfig';

vi.mock('./hooks/useOIDCConfig', () => ({ useOIDCConfig: vi.fn() }));

const useOIDCConfigMock = vi.mocked(useOIDCConfig);

beforeEach(() => {
  useOIDCConfigMock.mockReturnValue({ enabled: false, provider_name: 'SSO', registration_disabled: false });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function renderRegister() {
  return render(
    <MemoryRouter>
      <AppThemeProvider>
        <RegisterPage />
      </AppThemeProvider>
    </MemoryRouter>
  );
}

// Issue #192: same fix as LoginPage.tsx -- a failed submission used to drop
// keyboard focus to <body> with no programmatic link between the error and
// the fields it concerns.
describe('RegisterPage failed-registration focus and error association', () => {
  test('moves focus to the error and associates it with all three fields on a rejected submit', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue({
      ok: false,
      json: async () => ({ error: { message: 'Username already taken' } }),
    } as Response);

    renderRegister();

    const usernameInput = screen.getByLabelText(/username/i);
    const emailInput = screen.getByLabelText(/email/i);
    const passwordInput = screen.getByLabelText(/password/i);

    fireEvent.change(usernameInput, { target: { value: 'alice' } });
    fireEvent.change(emailInput, { target: { value: 'alice@example.com' } });
    fireEvent.change(passwordInput, { target: { value: 'secret123' } });
    fireEvent.click(screen.getByRole('button', { name: /register/i }));

    await screen.findByText('Username already taken');
    const alert = screen.getByRole('alert');
    await waitFor(() => expect(document.activeElement).toBe(alert));

    expect(usernameInput).toHaveAttribute('aria-describedby', alert.id);
    expect(emailInput).toHaveAttribute('aria-describedby', alert.id);
    expect(passwordInput).toHaveAttribute('aria-describedby', alert.id);
    expect(usernameInput).toHaveAttribute('aria-invalid', 'true');
  });
});

// DISABLE_REGISTRATION: someone can still land on /register directly (a
// bookmark, a typed URL) even with LoginPage's link hidden — show that
// plainly instead of a form that would always 403 on submit.
describe('RegisterPage registration gate', () => {
  test('shows the form by default', () => {
    renderRegister();
    expect(screen.getByRole('button', { name: /^register$/i })).toBeInTheDocument();
  });

  test('shows a disabled notice instead of the form when the server has registration disabled', () => {
    useOIDCConfigMock.mockReturnValue({ enabled: false, provider_name: 'SSO', registration_disabled: true });
    renderRegister();

    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
    expect(screen.getByText(/registration is currently disabled/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /back to login/i })).toBeInTheDocument();
  });
});
