import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import './i18n/config';
import { AppThemeProvider } from './AppThemeProvider';
import { useOIDCConfig } from './hooks/useOIDCConfig';
import RegisterPage from './RegisterPage';

vi.mock('./hooks/useOIDCConfig', () => ({ useOIDCConfig: vi.fn() }));

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
  vi.restoreAllMocks();
});

function renderRegister() {
  return render(
    <MemoryRouter>
      <AppThemeProvider>
        <RegisterPage />
      </AppThemeProvider>
    </MemoryRouter>,
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
    useOIDCConfigMock.mockReturnValue({
      enabled: false,
      provider_name: 'SSO',
      registration_disabled: true,
    });
    renderRegister();

    expect(screen.queryByLabelText(/username/i)).not.toBeInTheDocument();
    expect(screen.getByText(/registration is currently disabled/i)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /back to login/i })).toBeInTheDocument();
  });
});

// Issue #1790: the registration form was a dead end -- no logo, no way back
// to Login, and no password-requirement hint until the server rejected the
// submission.
describe('RegisterPage navigation and password guidance', () => {
  test('shows the brand logo and a link back to Login', () => {
    renderRegister();

    expect(screen.getByRole('img', { name: 'Mycorrhizal CRM' })).toBeInTheDocument();
    const loginLink = screen.getByRole('link', { name: /already have an account\? log in/i });
    expect(loginLink).toHaveAttribute('href', '/login');
  });

  test('shows the password requirement as helper text before submit', () => {
    renderRegister();

    expect(screen.getByText(/use at least 8 characters/i)).toBeInTheDocument();
    expect(screen.getByText(/15\+ characters or a 20\+ character passphrase/i)).toBeInTheDocument();
  });

  test('the registration-disabled state still offers the logo and a way back', () => {
    useOIDCConfigMock.mockReturnValue({
      enabled: false,
      provider_name: 'SSO',
      registration_disabled: true,
    });
    renderRegister();

    expect(screen.getByRole('img', { name: 'Mycorrhizal CRM' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /back to login/i })).toHaveAttribute('href', '/login');
  });
});

// Issue #1793: registration used to dump the user back on the Login form to
// retype the credentials they had just chosen.
describe('RegisterPage post-registration sign-in', () => {
  function renderWithRoutes(setToken: (token: string | null) => void) {
    return render(
      <MemoryRouter initialEntries={['/register']}>
        <AppThemeProvider>
          <Routes>
            <Route path="/register" element={<RegisterPage setToken={setToken} />} />
            <Route path="/" element={<div>DASHBOARD PAGE</div>} />
            <Route path="/login" element={<div>LOGIN PAGE</div>} />
          </Routes>
        </AppThemeProvider>
      </MemoryRouter>,
    );
  }

  async function fillAndSubmit() {
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: 'alice' } });
    fireEvent.change(screen.getByLabelText(/email/i), { target: { value: 'alice@example.com' } });
    fireEvent.change(screen.getByLabelText(/password/i), {
      target: { value: 'correct horse battery staple' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^register$/i }));
  }

  test('signs the new account in and lands on the dashboard', async () => {
    const setToken = vi.fn();
    const fetchMock = vi.spyOn(global, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/register')) {
        return { ok: true, status: 201, json: async () => ({ message: 'ok' }) } as Response;
      }
      if (url.endsWith('/login')) {
        return { ok: true, status: 200, json: async () => ({}) } as Response;
      }
      if (url.endsWith('/users/me')) {
        return {
          ok: true,
          status: 200,
          json: async () => ({ id: 1, username: 'alice', is_admin: false }),
        } as Response;
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithRoutes(setToken);
    await fillAndSubmit();

    expect(await screen.findByText('DASHBOARD PAGE')).toBeInTheDocument();
    expect(setToken).toHaveBeenCalledWith('authenticated');
    expect(fetchMock.mock.calls.map(([url]) => String(url))).toEqual(
      expect.arrayContaining(['/api/v1/register', '/api/v1/login', '/api/v1/users/me']),
    );
    // The credentials just chosen were reused -- the user never saw Login.
    expect(screen.queryByText('LOGIN PAGE')).not.toBeInTheDocument();
  });

  test('falls back to a success message and the login form when auto sign-in fails', async () => {
    const setToken = vi.fn();
    vi.spyOn(global, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.endsWith('/register')) {
        return { ok: true, status: 201, json: async () => ({ message: 'ok' }) } as Response;
      }
      if (url.endsWith('/login')) {
        return { ok: false, status: 500, json: async () => ({}) } as Response;
      }
      throw new Error(`unexpected fetch: ${url}`);
    });

    renderWithRoutes(setToken);
    await fillAndSubmit();

    expect(
      await screen.findByText('Registration successful! You can now log in.'),
    ).toBeInTheDocument();
    expect(setToken).not.toHaveBeenCalled();
    // The fallback redirect still lands on Login (it takes 1.5s by design).
    expect(await screen.findByText('LOGIN PAGE', {}, { timeout: 3000 })).toBeInTheDocument();
  });
});
