import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import TwoFactorSettings from './TwoFactorSettings';
import { SnackbarProvider } from '../context/SnackbarContext';

beforeEach(() => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1, username: 'test', is_admin: false }));
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

type MockResponse = unknown | { body: unknown; ok?: boolean };

function mockFetchByUrl(handlers: Record<string, (init?: RequestInit) => MockResponse>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          const result = respond(init);
          const body =
            result && typeof result === 'object' && 'body' in result ? result.body : result;
          const ok = result && typeof result === 'object' && 'ok' in result ? !!result.ok : true;
          const text = JSON.stringify(body);
          return { ok, json: async () => body, text: async () => text };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

const tenCodes = Array.from({ length: 10 }, (_, i) => `CODE-${i}-XXXX-YYYY`);

test('shows the enable button when 2FA is disabled', async () => {
  mockFetchByUrl({ '/users/2fa/status': () => ({ enabled: false }) });

  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText('Two-factor authentication')).toBeInTheDocument());
  expect(screen.getByText('Enable two-factor authentication')).toBeInTheDocument();
});

test('runs the enrollment wizard: setup → QR/key → confirm → recovery codes', async () => {
  mockFetchByUrl({
    '/users/2fa/status': () => ({ enabled: false }),
    '/users/2fa/setup': () => ({ secret: 'JBSWY3DPEHPK3PXP', otpauth_url: 'otpauth://totp/mycorrhizal:test?secret=JBSWY3DPEHPK3PXP' }),
    '/users/2fa/confirm': (init?: RequestInit) => {
      const body = JSON.parse(String(init?.body));
      // The backend rejects a wrong code; only a "valid" one enables 2FA.
      if (body.code !== '123456') {
        return {
          ok: false,
          body: { error: { code: 'INVALID_INPUT', message: 'x', details: { reason: 'Invalid code. Please try again.' } } },
        };
      }
      return { body: { message: 'enabled', recovery_codes: tenCodes } };
    },
  });

  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>
  );

  const enable = await screen.findByText('Enable two-factor authentication');
  fireEvent.click(enable);

  // Setup dialog: QR + manual key visible.
  await waitFor(() => expect(screen.getByDisplayValue('JBSWY3DPEHPK3PXP')).toBeInTheDocument());
  // The QR SVG rendered.
  expect(document.querySelector('svg')).not.toBeNull();

  // A wrong code shows an inline error inside the dialog.
  fireEvent.change(screen.getByLabelText('Verification code *'), { target: { value: '000000' } });
  fireEvent.click(screen.getByText('Enable and continue'));
  await waitFor(() => expect(screen.getByText('Invalid code. Please try again.')).toBeInTheDocument());

  // The correct code confirms and shows the one-time recovery codes.
  fireEvent.change(screen.getByLabelText('Verification code *'), { target: { value: '123456' } });
  fireEvent.click(screen.getByText('Enable and continue'));

  await waitFor(() => expect(screen.getByText('Two-factor authentication enabled.')).toBeInTheDocument());

  // Recovery codes are shown exactly once.
  await waitFor(() => expect(screen.getByText('Recovery codes')).toBeInTheDocument());
  for (const code of tenCodes) {
    expect(screen.getByText(code)).toBeInTheDocument();
  }
  fireEvent.click(screen.getByText('Done'));
  await waitFor(() => expect(screen.queryByText('Recovery codes')).not.toBeInTheDocument());
});

test('shows the enabled state with disable and regenerate buttons', async () => {
  mockFetchByUrl({ '/users/2fa/status': () => ({ enabled: true }) });

  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText(/Two-factor authentication is enabled/)).toBeInTheDocument());
  expect(screen.getByText('Disable')).toBeInTheDocument();
  expect(screen.getByText('Regenerate recovery codes')).toBeInTheDocument();
});

test('disables 2FA after confirming with a code', async () => {
  let status: { enabled: boolean } = { enabled: true };
  mockFetchByUrl({
    '/users/2fa/status': () => status,
    '/users/2fa/disable': () => {
      status = { enabled: false };
      return { message: 'disabled' };
    },
  });

  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>
  );

  fireEvent.click(await screen.findByText('Disable'));

  await waitFor(() => expect(screen.getByText(/Enter a current code/)).toBeInTheDocument());
  fireEvent.change(screen.getByLabelText('Verification code *'), { target: { value: '123456' } });
  fireEvent.click(screen.getByText('Confirm'));

  await waitFor(() => expect(screen.getByText('Two-factor authentication disabled.')).toBeInTheDocument());
  await waitFor(() => expect(screen.getByText('Enable two-factor authentication')).toBeInTheDocument());
});

test('regenerates recovery codes after confirming with a code', async () => {
  const freshCodes = ['FRESH-AAAA-BBBB', 'FRESH-CCCC-DDDD'];
  mockFetchByUrl({
    '/users/2fa/status': () => ({ enabled: true }),
    '/users/2fa/recovery-codes/regenerate': () => ({ recovery_codes: freshCodes }),
  });

  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>
  );

  fireEvent.click(await screen.findByText('Regenerate recovery codes'));

  await waitFor(() => expect(screen.getByText(/Enter a current code/)).toBeInTheDocument());
  fireEvent.change(screen.getByLabelText('Verification code *'), { target: { value: '654321' } });
  fireEvent.click(screen.getByText('Confirm'));

  await waitFor(() => expect(screen.getByText('Recovery codes')).toBeInTheDocument());
  for (const code of freshCodes) {
    expect(screen.getByText(code)).toBeInTheDocument();
  }
});
