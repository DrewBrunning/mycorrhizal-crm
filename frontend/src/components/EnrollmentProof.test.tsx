import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import {
  cancelledError,
  creationOptionsWire,
  requestOptionsWire,
  stubWebAuthn,
  unstubWebAuthn,
} from '../webauthnTestUtils';
import PasskeySettings from './PasskeySettings';
import TwoFactorSettings from './TwoFactorSettings';

// Issue #1337: adding a further second factor to an account that already holds
// one needs a live proof — a code, or an assertion from an existing passkey.
// The first factor stays proof-free (covered by PasskeySettings.test.tsx /
// TwoFactorSettings.test.tsx, whose accounts have no factor).

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  unstubWebAuthn();
  vi.unstubAllGlobals();
});

type Handler = (init?: RequestInit) => unknown;

// Routes by "METHOD /path-fragment"; a handler may return `{ ok: false, body }`.
function mockFetch(handlers: Record<string, Handler>) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    for (const [pattern, respond] of Object.entries(handlers)) {
      const [method, fragment] = pattern.split(' ');
      if ((init?.method ?? 'GET') === method && url.includes(fragment)) {
        const result = respond(init);
        const wrapped =
          result && typeof result === 'object' && 'body' in result
            ? (result as { body: unknown; ok?: boolean })
            : { body: result, ok: true };
        const ok = wrapped.ok !== false;
        const text = JSON.stringify(wrapped.body);
        return {
          ok,
          status: ok ? 200 : 400,
          json: async () => wrapped.body,
          text: async () => text,
        };
      }
    }
    throw new Error(`unexpected fetch: ${init?.method ?? 'GET'} ${url}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const laptop = { id: 'p1', name: 'Laptop', created_at: '2026-01-05T12:00:00Z' };

const bodyOf = (fetchMock: ReturnType<typeof mockFetch>, fragment: string) => {
  const call = fetchMock.mock.calls.find(([u]) => String(u).includes(fragment));
  return call?.[1]?.body ? JSON.parse(String(call[1].body)) : undefined;
};
const called = (fetchMock: ReturnType<typeof mockFetch>, fragment: string) =>
  fetchMock.mock.calls.some(([u]) => String(u).includes(fragment));

const renderPasskeys = () =>
  render(
    <SnackbarProvider>
      <PasskeySettings />
    </SnackbarProvider>,
  );
const renderTotp = () =>
  render(
    <SnackbarProvider>
      <TwoFactorSettings />
    </SnackbarProvider>,
  );

const finishOk = () => ({ id: 'p2', name: 'Phone', created_at: 'x', recovery_codes: [] });

// ---------------------------------------------------------------------------
// PasskeySettings
// ---------------------------------------------------------------------------

test('passkey account: Add asks for a proof first and sends the code with begin', async () => {
  const { create } = stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'GET /users/2fa/status': () => ({ enabled: false }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
    'POST /webauthn/register/finish': finishOk,
  });
  renderPasskeys();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  const dialog = await screen.findByRole('dialog');
  // Nothing was sent, and no authenticator prompt was raised, before the proof.
  expect(called(fetchMock, 'register/begin')).toBe(false);
  expect(create).not.toHaveBeenCalled();
  const confirm = within(dialog).getByRole('button', { name: 'Verify and continue' });
  expect(confirm).toBeDisabled();

  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '  AAAAA-BBBBB-CCCCC ' },
  });
  fireEvent.click(confirm);

  expect(await screen.findByText('Passkey added.')).toBeInTheDocument();
  expect(bodyOf(fetchMock, 'register/begin')).toEqual({ code: 'AAAAA-BBBBB-CCCCC' });
  expect(create).toHaveBeenCalledTimes(1);
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

test('passkey account: the proof can be an assertion from an existing passkey', async () => {
  const { get, create } = stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'GET /users/2fa/status': () => ({ enabled: false }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
    'POST /webauthn/register/begin': () => creationOptionsWire,
    'POST /webauthn/register/finish': finishOk,
  });
  renderPasskeys();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify with a passkey instead' }));

  expect(await screen.findByText('Passkey added.')).toBeInTheDocument();
  expect(get).toHaveBeenCalledTimes(1);
  expect(create).toHaveBeenCalledTimes(1);
  // The proof ceremony is over all passkeys (nothing is being removed).
  expect(bodyOf(fetchMock, 'assert/begin')).toEqual({});
  expect(bodyOf(fetchMock, 'register/begin')).toMatchObject({ assertion: { id: 'cred-id' } });
});

test('TOTP-only account: proof is required for the first passkey, by code only', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'GET /users/2fa/status': () => ({ enabled: true }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
    'POST /webauthn/register/finish': finishOk,
  });
  renderPasskeys();
  await screen.findByText('No passkeys yet.');
  await waitFor(() => expect(called(fetchMock, '/users/2fa/status')).toBe(true));

  // The status fetch resolves after the list; wait until Add opens the dialog.
  await waitFor(() => {
    fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
    expect(screen.queryByRole('dialog')).toBeInTheDocument();
  });
  const dialog = screen.getByRole('dialog');
  // No passkey exists to prove with.
  expect(
    within(dialog).queryByRole('button', { name: 'Verify with a passkey instead' }),
  ).not.toBeInTheDocument();
  expect(called(fetchMock, 'register/begin')).toBe(false);

  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '123456' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify and continue' }));
  expect(await screen.findByText('Passkey added.')).toBeInTheDocument();
  expect(bodyOf(fetchMock, 'register/begin')).toEqual({ code: '123456' });
});

test('a wrong code keeps the proof dialog open with the backend message', async () => {
  const { create } = stubWebAuthn();
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'GET /users/2fa/status': () => ({ enabled: false }),
    'POST /webauthn/register/begin': () => ({
      ok: false,
      body: { error: { message: 'Invalid code. Please try again.' } },
    }),
  });
  renderPasskeys();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '000000' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify and continue' }));

  expect(await within(dialog).findByText(/Invalid code/)).toBeInTheDocument();
  expect(create).not.toHaveBeenCalled();
  expect(screen.queryByText('Passkey added.')).not.toBeInTheDocument();
});

test('a cancelled proof prompt is reported in the dialog and enrolls nothing', async () => {
  stubWebAuthn({
    get: () => {
      throw cancelledError();
    },
  });
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'GET /users/2fa/status': () => ({ enabled: false }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
  });
  renderPasskeys();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify with a passkey instead' }));

  expect(
    await within(dialog).findByText('The passkey prompt was cancelled or timed out.'),
  ).toBeInTheDocument();
  expect(called(fetchMock, 'register/begin')).toBe(false);
});

test('a non-Error proof failure falls back to the generic message; Cancel closes the dialog', async () => {
  stubWebAuthn({
    get: () => {
      throw 'nope';
    },
  });
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'GET /users/2fa/status': () => ({ enabled: false }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
  });
  renderPasskeys();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify with a passkey instead' }));
  expect(
    await within(dialog).findByText('Verification failed. Please try again.'),
  ).toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

// ---------------------------------------------------------------------------
// TwoFactorSettings (TOTP enrollment on a passkey-only account)
// ---------------------------------------------------------------------------

const setupResult = {
  secret: 'JBSWY3DPEHPK3PXP',
  otpauth_url: 'otpauth://totp/mycorrhizal:test?secret=JBSWY3DPEHPK3PXP',
};

test('passkey-only account: Enable asks for a proof, then runs setup with it', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /users/2fa/status': () => ({ enabled: false }),
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'POST /users/2fa/setup': () => setupResult,
  });
  renderTotp();
  await screen.findByText('Enable two-factor authentication');
  await waitFor(() => expect(called(fetchMock, '/webauthn/credentials')).toBe(true));

  await waitFor(() => {
    fireEvent.click(screen.getByRole('button', { name: 'Enable two-factor authentication' }));
    expect(screen.queryByText("Verify it's you")).toBeInTheDocument();
  });
  expect(called(fetchMock, '/users/2fa/setup')).toBe(false);
  const dialog = screen.getByRole('dialog');

  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: 'AAAAA-BBBBB-CCCCC' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify and continue' }));

  // The wizard opens with the freshly minted secret.
  expect(await screen.findByDisplayValue('JBSWY3DPEHPK3PXP')).toBeInTheDocument();
  expect(bodyOf(fetchMock, '/users/2fa/setup')).toEqual({ code: 'AAAAA-BBBBB-CCCCC' });
  await waitFor(() => expect(screen.queryByText("Verify it's you")).not.toBeInTheDocument());
});

test('passkey-only account: the proof can be a passkey assertion', async () => {
  const { get } = stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /users/2fa/status': () => ({ enabled: false }),
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
    'POST /users/2fa/setup': () => setupResult,
  });
  renderTotp();
  await screen.findByText('Enable two-factor authentication');
  await waitFor(() => expect(called(fetchMock, '/webauthn/credentials')).toBe(true));
  await waitFor(() => {
    fireEvent.click(screen.getByRole('button', { name: 'Enable two-factor authentication' }));
    expect(screen.queryByText("Verify it's you")).toBeInTheDocument();
  });

  fireEvent.click(screen.getByRole('button', { name: 'Verify with a passkey instead' }));

  expect(await screen.findByDisplayValue('JBSWY3DPEHPK3PXP')).toBeInTheDocument();
  expect(get).toHaveBeenCalledTimes(1);
  expect(bodyOf(fetchMock, '/users/2fa/setup')).toMatchObject({ assertion: { id: 'cred-id' } });
});

test('a rejected proof stays in the dialog with the server message and mints no secret', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /users/2fa/status': () => ({ enabled: false }),
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'POST /users/2fa/setup': () => ({
      ok: false,
      body: { error: { message: 'Invalid code. Please try again.' } },
    }),
  });
  renderTotp();
  await screen.findByText('Enable two-factor authentication');
  await waitFor(() => expect(called(fetchMock, '/webauthn/credentials')).toBe(true));
  await waitFor(() => {
    fireEvent.click(screen.getByRole('button', { name: 'Enable two-factor authentication' }));
    expect(screen.queryByText("Verify it's you")).toBeInTheDocument();
  });
  const dialog = screen.getByRole('dialog');
  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '000000' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify and continue' }));

  expect(await within(dialog).findByText(/Invalid code/)).toBeInTheDocument();
  expect(screen.queryByDisplayValue('JBSWY3DPEHPK3PXP')).not.toBeInTheDocument();
});

test('a cancelled passkey prompt / non-Error failure is reported in the TOTP proof dialog', async () => {
  const stub = stubWebAuthn({
    get: () => {
      throw cancelledError();
    },
  });
  const fetchMock = mockFetch({
    'GET /users/2fa/status': () => ({ enabled: false }),
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
  });
  renderTotp();
  await screen.findByText('Enable two-factor authentication');
  await waitFor(() => expect(called(fetchMock, '/webauthn/credentials')).toBe(true));
  await waitFor(() => {
    fireEvent.click(screen.getByRole('button', { name: 'Enable two-factor authentication' }));
    expect(screen.queryByText("Verify it's you")).toBeInTheDocument();
  });
  const dialog = screen.getByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify with a passkey instead' }));
  expect(
    await within(dialog).findByText('The passkey prompt was cancelled or timed out.'),
  ).toBeInTheDocument();
  expect(stub.get).toHaveBeenCalledTimes(1);
  expect(called(fetchMock, '/users/2fa/setup')).toBe(false);

  // A non-Error rejection gets the generic message.
  stub.get.mockImplementation(() => {
    throw 'nope';
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Verify with a passkey instead' }));
  expect(
    await within(dialog).findByText('Verification failed. Please try again.'),
  ).toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

test('an account with no passkey enables TOTP with no proof (first factor)', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /users/2fa/status': () => ({ enabled: false }),
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'POST /users/2fa/setup': () => setupResult,
  });
  renderTotp();
  await screen.findByText('Enable two-factor authentication');
  await waitFor(() => expect(called(fetchMock, '/webauthn/credentials')).toBe(true));

  fireEvent.click(screen.getByRole('button', { name: 'Enable two-factor authentication' }));

  expect(await screen.findByDisplayValue('JBSWY3DPEHPK3PXP')).toBeInTheDocument();
  expect(screen.queryByText("Verify it's you")).not.toBeInTheDocument();
  expect(bodyOf(fetchMock, '/users/2fa/setup')).toBeUndefined();
});
