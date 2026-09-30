import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import TwoFactorSettings from './TwoFactorSettings';

// Issue #1354: recovery-code regeneration is offered whenever ANY second factor
// is enrolled, and a passkey assertion is an accepted proof.

const mocks = vi.hoisted(() => ({
  listPasskeys: vi.fn(),
  proveWithAnyPasskey: vi.fn(),
}));

vi.mock('../api/webauthn', async (importActual) => ({
  ...(await importActual<typeof import('../api/webauthn')>()),
  listPasskeys: mocks.listPasskeys,
  proveWithAnyPasskey: mocks.proveWithAnyPasskey,
}));

vi.mock('../webauthnCeremony', async (importActual) => ({
  ...(await importActual<typeof import('../webauthnCeremony')>()),
  isWebAuthnSupported: () => true,
}));

const freshCodes = ['FRESH-AAAA-BBBB', 'FRESH-CCCC-DDDD'];
const passkey = { id: 'pk1', name: 'Key', created_at: '2026-01-01T00:00:00Z' };

let regenerateBodies: unknown[] = [];
let regenerateResponse: { ok: boolean; body: unknown };

beforeEach(() => {
  regenerateBodies = [];
  regenerateResponse = { ok: true, body: { recovery_codes: freshCodes } };
  mocks.listPasskeys.mockResolvedValue([passkey]);
  mocks.proveWithAnyPasskey.mockResolvedValue({ assertion: { id: 'cred' } });
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (url.includes('/users/2fa/status')) {
        const body = { enabled: false };
        return {
          ok: true,
          json: async () => body,
          text: async () => JSON.stringify(body),
        };
      }
      if (url.includes('/recovery-codes/regenerate')) {
        regenerateBodies.push(JSON.parse(String(init?.body)));
        const { ok, body } = regenerateResponse;
        return {
          ok,
          status: ok ? 200 : 429,
          json: async () => body,
          text: async () => JSON.stringify(body),
        };
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

function renderCard() {
  return render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>,
  );
}

test('a passkey-only account is offered Regenerate and can prove with a passkey', async () => {
  renderCard();

  fireEvent.click(await screen.findByText('Regenerate recovery codes'));
  fireEvent.click(await screen.findByText('Verify with a passkey instead'));

  await waitFor(() => expect(screen.getByText('FRESH-AAAA-BBBB')).toBeInTheDocument());
  expect(mocks.proveWithAnyPasskey).toHaveBeenCalledTimes(1);
  expect(regenerateBodies).toEqual([{ assertion: { id: 'cred' } }]);
});

test('a passkey-only account can also prove with a recovery code', async () => {
  renderCard();

  fireEvent.click(await screen.findByText('Regenerate recovery codes'));
  fireEvent.change(await screen.findByLabelText('Verification code *'), {
    target: { value: 'AAAAA-BBBBB-CCCCC' },
  });
  fireEvent.click(screen.getByText('Verify and continue'));

  await waitFor(() => expect(screen.getByText('FRESH-CCCC-DDDD')).toBeInTheDocument());
  expect(regenerateBodies).toEqual([{ code: 'AAAAA-BBBBB-CCCCC' }]);
});

test('a locked-out proof shows the server message and keeps the dialog open', async () => {
  regenerateResponse = {
    ok: false,
    body: {
      error: 'Account temporarily locked',
      message: 'Too many failed verification attempts. Please try again later.',
      retry_after: 60,
    },
  };
  renderCard();

  fireEvent.click(await screen.findByText('Regenerate recovery codes'));
  fireEvent.click(await screen.findByText('Verify with a passkey instead'));

  expect(
    await screen.findByText('Too many failed verification attempts. Please try again later.'),
  ).toBeInTheDocument();
  expect(screen.queryByText('FRESH-AAAA-BBBB')).not.toBeInTheDocument();
});

test('a cancelled passkey prompt shows the cancelled message', async () => {
  mocks.proveWithAnyPasskey.mockRejectedValue(new DOMException('cancelled', 'NotAllowedError'));
  renderCard();

  fireEvent.click(await screen.findByText('Regenerate recovery codes'));
  fireEvent.click(await screen.findByText('Verify with a passkey instead'));

  await waitFor(() => expect(mocks.proveWithAnyPasskey).toHaveBeenCalled());
  expect(regenerateBodies).toEqual([]);
  expect(screen.queryByText('FRESH-AAAA-BBBB')).not.toBeInTheDocument();
});

test('no factor at all offers no Regenerate action', async () => {
  mocks.listPasskeys.mockResolvedValue([]);
  renderCard();

  await screen.findByText('Enable two-factor authentication');
  expect(screen.queryByText('Regenerate recovery codes')).not.toBeInTheDocument();
});
