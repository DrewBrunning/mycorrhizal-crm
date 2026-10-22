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
});

type Handler = (init?: RequestInit) => unknown;

// Routes by "METHOD /path-fragment"; each handler may return `{ ok: false, body }`.
function mockFetch(handlers: Record<string, Handler>) {
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    const key = `${init?.method ?? 'GET'} ${url}`;
    for (const [pattern, respond] of Object.entries(handlers)) {
      const [method, fragment] = pattern.split(' ');
      if (key.startsWith(method) && url.includes(fragment)) {
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
    throw new Error(`unexpected fetch: ${key}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const laptop = {
  id: 'p1',
  name: 'Laptop',
  created_at: '2026-01-05T12:00:00Z',
  last_used_at: '2026-02-01T12:00:00Z',
};
const yubikey = {
  id: 'p2',
  name: 'YubiKey',
  created_at: '2026-01-06T12:00:00Z',
  last_used_at: null,
};

function renderCard() {
  return render(
    <SnackbarProvider>
      <PasskeySettings />
    </SnackbarProvider>,
  );
}

test('lists enrolled passkeys with created / last-used info', async () => {
  stubWebAuthn();
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop, yubikey] }) });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  expect(screen.getByText('YubiKey')).toBeInTheDocument();
  expect(screen.getByText(/last used/)).toBeInTheDocument();
  expect(screen.getByText(/never used/)).toBeInTheDocument();
});

test('shows an empty state when there are none', async () => {
  stubWebAuthn();
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [] }) });
  renderCard();
  expect(await screen.findByText('No passkeys yet.')).toBeInTheDocument();
});

test('a failed list load shows the error with Retry, never the empty state (issue #1779)', async () => {
  stubWebAuthn();
  let fail = true;
  mockFetch({
    'GET /webauthn/credentials': () =>
      fail ? { ok: false, body: { error: { message: 'boom' } } } : { credentials: [laptop] },
  });
  renderCard();
  expect(await screen.findByRole('alert')).toHaveTextContent('boom');
  expect(screen.queryByText('No passkeys yet.')).not.toBeInTheDocument();

  // Retry refetches in place.
  fail = false;
  fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  expect(screen.queryByRole('alert')).not.toBeInTheDocument();
});

test('explains and disables enrollment when the browser has no WebAuthn', async () => {
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [] }) });
  renderCard();
  expect(await screen.findByText("This browser doesn't support passkeys.")).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Add a passkey' })).not.toBeInTheDocument();
});

test('enrollment: register-begin → create() → register-finish → list refresh', async () => {
  const { create } = stubWebAuthn();
  let listed = [] as (typeof laptop)[];
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: listed }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
    'POST /webauthn/register/finish': () => {
      listed = [laptop];
      return { id: 'p1', name: 'Laptop', created_at: laptop.created_at, recovery_codes: [] };
    },
  });
  renderCard();
  await screen.findByText('No passkeys yet.');

  fireEvent.change(screen.getByLabelText('Passkey name (optional)'), {
    target: { value: 'Laptop' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));

  expect(await screen.findByText('Passkey added.')).toBeInTheDocument();
  expect(create).toHaveBeenCalledTimes(1);
  const begin = fetchMock.mock.calls.find(([u]) => String(u).includes('register/begin'));
  expect(JSON.parse(String(begin?.[1]?.body))).toEqual({ name: 'Laptop' });
  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  // No recovery codes minted → no recovery dialog.
  expect(screen.queryByText('Recovery codes')).not.toBeInTheDocument();
});

test('first second factor: the minted recovery codes are shown once', async () => {
  stubWebAuthn();
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
    'POST /webauthn/register/finish': () => ({
      id: 'p1',
      name: 'Laptop',
      created_at: 'x',
      recovery_codes: ['AAAA-1111', 'BBBB-2222'],
    }),
  });
  const writeText = vi.fn().mockResolvedValue(undefined);
  vi.stubGlobal('navigator', Object.assign(Object.create(navigator), { clipboard: { writeText } }));
  renderCard();
  await screen.findByText('No passkeys yet.');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));

  expect(await screen.findByText('AAAA-1111')).toBeInTheDocument();
  expect(screen.getByText('BBBB-2222')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Copy all codes' }));
  expect(writeText).toHaveBeenCalledWith('AAAA-1111\nBBBB-2222');
  fireEvent.click(screen.getByRole('button', { name: 'Done' }));
  await waitFor(() => expect(screen.queryByText('AAAA-1111')).not.toBeInTheDocument());
});

test('a cancelled ceremony shows the cancelled message and never calls finish', async () => {
  stubWebAuthn({
    create: () => {
      throw cancelledError();
    },
  });
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
  });
  renderCard();
  await screen.findByText('No passkeys yet.');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));

  expect(
    await screen.findByText('The passkey prompt was cancelled or timed out.'),
  ).toBeInTheDocument();
  expect(fetchMock.mock.calls.some(([u]) => String(u).includes('register/finish'))).toBe(false);
  // Button is usable again.
  expect(screen.getByRole('button', { name: 'Add a passkey' })).toBeEnabled();
});

test('a backend failure shows the backend message', async () => {
  stubWebAuthn();
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'POST /webauthn/register/begin': () => ({
      ok: false,
      body: { error: { message: 'OIDC accounts cannot enroll passkeys' } },
    }),
  });
  renderCard();
  await screen.findByText('No passkeys yet.');

  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));

  expect(await screen.findByText(/OIDC accounts cannot enroll passkeys/)).toBeInTheDocument();
});

test('a non-Error rejection falls back to the generic add error', async () => {
  stubWebAuthn({
    create: () => {
      throw 'nope';
    },
  });
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [] }),
    'POST /webauthn/register/begin': () => creationOptionsWire,
  });
  renderCard();
  await screen.findByText('No passkeys yet.');
  fireEvent.click(screen.getByRole('button', { name: 'Add a passkey' }));
  expect(await screen.findByText(/couldn't add the passkey/)).toBeInTheDocument();
});

test('removal requires the proof dialog; the code is sent and the list refreshes', async () => {
  stubWebAuthn();
  let listed = [laptop, yubikey];
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: listed }),
    'DELETE /webauthn/credentials/p1': () => {
      listed = [yubikey];
      return { message: 'Passkey removed' };
    },
  });
  renderCard();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  // Nothing is deleted until a proof is submitted.
  expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false);
  const confirm = within(dialog).getByRole('button', { name: 'Remove passkey' });
  expect(confirm).toBeDisabled();

  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '123456' },
  });
  fireEvent.click(confirm);

  expect(await screen.findByText('Passkey removed.')).toBeInTheDocument();
  const del = fetchMock.mock.calls.find(([, i]) => i?.method === 'DELETE');
  expect(JSON.parse(String(del?.[1]?.body))).toEqual({ code: '123456' });
  await waitFor(() => expect(screen.queryByText('Laptop')).not.toBeInTheDocument());
});

test('a rejected proof code keeps the dialog open with the backend message', async () => {
  stubWebAuthn();
  mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'DELETE /webauthn/credentials/p1': () => ({
      ok: false,
      body: { error: { message: 'Invalid code. Please try again.' } },
    }),
  });
  renderCard();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.change(within(dialog).getByLabelText('Verification code *'), {
    target: { value: '000000' },
  });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Remove passkey' }));

  expect(await within(dialog).findByText(/Invalid code/)).toBeInTheDocument();
  expect(screen.getByText('Laptop')).toBeInTheDocument();
});

test('removal can be proven with another passkey (assertion), only offered when one remains', async () => {
  const { get } = stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop, yubikey] }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
    'DELETE /webauthn/credentials/p1': () => ({ message: 'Passkey removed' }),
  });
  renderCard();
  await screen.findByText('Laptop');

  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Verify with another passkey instead' }),
  );

  expect(await screen.findByText('Passkey removed.')).toBeInTheDocument();
  expect(get).toHaveBeenCalledTimes(1);
  const begin = fetchMock.mock.calls.find(([u]) => String(u).includes('/webauthn/assert/begin'));
  expect(JSON.parse(String(begin?.[1]?.body))).toEqual({ exclude_id: 'p1' });
  const del = fetchMock.mock.calls.find(([, i]) => i?.method === 'DELETE');
  expect(JSON.parse(String(del?.[1]?.body))).toMatchObject({ assertion: { id: 'cred-id' } });
});

test('a failed proof begin shows the backend message and deletes nothing', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop, yubikey] }),
    'POST /webauthn/assert/begin': () => ({
      ok: false,
      body: { message: 'No other passkey is registered to verify with' },
    }),
  });
  renderCard();
  await screen.findByText('Laptop');
  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Verify with another passkey instead' }),
  );

  expect(await within(dialog).findByText(/No other passkey is registered/)).toBeInTheDocument();
  expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false);
});

test('the another-passkey route is hidden when this is the only passkey', async () => {
  stubWebAuthn();
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop] }) });
  renderCard();
  await screen.findByText('Laptop');
  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  expect(
    within(dialog).queryByRole('button', { name: 'Verify with another passkey instead' }),
  ).not.toBeInTheDocument();
});

test('a cancelled proof ceremony shows the cancelled message and deletes nothing', async () => {
  stubWebAuthn({
    get: () => {
      throw cancelledError();
    },
  });
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop, yubikey] }),
    'POST /webauthn/assert/begin': () => requestOptionsWire,
  });
  renderCard();
  await screen.findByText('Laptop');
  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Verify with another passkey instead' }),
  );

  expect(await within(dialog).findByText(/cancelled or timed out/)).toBeInTheDocument();
  expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false);
  expect(within(dialog).getByRole('button', { name: 'Remove passkey' })).toBeDisabled();
});

test('Cancel closes the proof dialog without deleting', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop] }) });
  renderCard();
  await screen.findByText('Laptop');
  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(fetchMock.mock.calls.some(([, i]) => i?.method === 'DELETE')).toBe(false);
});

// Issue #1793: a passkey label is editable after enrollment, and renaming is
// proof-free (unlike adding/removing a factor).
test('renames an existing passkey without a second-factor proof', async () => {
  stubWebAuthn();
  let listed = [laptop];
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: listed }),
    'PATCH /webauthn/credentials/p1': (init) => {
      const body = JSON.parse(String(init?.body));
      listed = [{ ...laptop, name: body.name }];
      return listed[0];
    },
  });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Rename passkey Laptop' }));

  const dialog = await screen.findByRole('dialog');
  const nameInput = within(dialog).getByLabelText(/passkey name/i);
  expect(nameInput).toHaveValue('Laptop');
  fireEvent.change(nameInput, { target: { value: 'Kitchen tablet' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('Passkey renamed.')).toBeInTheDocument();
  expect(await screen.findByText('Kitchen tablet')).toBeInTheDocument();
  const patch = fetchMock.mock.calls.find(
    ([url, init]) => String(url).includes('/webauthn/credentials/p1') && init?.method === 'PATCH',
  );
  expect(patch).toBeDefined();
  expect(JSON.parse(String(patch?.[1]?.body))).toEqual({ name: 'Kitchen tablet' });
  // No TOTP/recovery-code proof dialog appeared.
  expect(screen.queryByText('Verify it\u2019s you')).not.toBeInTheDocument();
});

test('cancelling a rename closes the dialog without sending a request', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop] }) });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Rename passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');

  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(fetchMock.mock.calls.some(([, init]) => init?.method === 'PATCH')).toBe(false);
});

test('an empty rename is disabled, and a backend failure keeps the dialog open with its message', async () => {
  stubWebAuthn();
  const fetchMock = mockFetch({
    'GET /webauthn/credentials': () => ({ credentials: [laptop] }),
    'PATCH /webauthn/credentials/p1': () => ({
      ok: false,
      body: { error: { message: 'Rename rejected' } },
    }),
  });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Rename passkey Laptop' }));
  const dialog = await screen.findByRole('dialog');
  const nameInput = within(dialog).getByLabelText(/passkey name/i);

  fireEvent.change(nameInput, { target: { value: '   ' } });
  expect(within(dialog).getByRole('button', { name: 'Save' })).toBeDisabled();

  fireEvent.change(nameInput, { target: { value: 'Kitchen tablet' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

  expect(await within(dialog).findByText('Rename rejected')).toBeInTheDocument();
  expect(screen.getByRole('dialog')).toBeInTheDocument();
  expect(nameInput).toHaveValue('Kitchen tablet');
  expect(fetchMock.mock.calls.filter(([, init]) => init?.method === 'PATCH')).toHaveLength(1);
});

test('Escape closes the rename dialog without renaming', async () => {
  stubWebAuthn();
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop] }) });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Rename passkey Laptop' }));
  expect(await screen.findByRole('dialog')).toBeInTheDocument();

  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });

  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
});

test('Escape closes the removal dialog without removing', async () => {
  stubWebAuthn();
  mockFetch({ 'GET /webauthn/credentials': () => ({ credentials: [laptop] }) });
  renderCard();

  expect(await screen.findByText('Laptop')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Remove passkey Laptop' }));
  expect(await screen.findByRole('dialog')).toBeInTheDocument();

  fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape', code: 'Escape' });

  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
  expect(screen.getByText('Laptop')).toBeInTheDocument();
});
