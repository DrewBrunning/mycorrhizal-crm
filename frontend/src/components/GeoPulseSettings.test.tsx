import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import GeoPulseSettings from './GeoPulseSettings';

// See PaperlessSettings.test.tsx for the full explanation: the config-load
// effect and the effect that mirrors `config` onto local field state flush in
// different passive-effect passes, so a field edit right after the initial
// waitFor can be clobbered by the still-pending mirror. settle() flushes it.
async function settle() {
  await act(async () => {});
}

beforeEach(() => {
  localStorage.setItem(
    'user_info',
    JSON.stringify({ user_id: 1, username: 'test', is_admin: false }),
  );
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

type Handler = (init?: RequestInit) => unknown | { __status: number; body: unknown };

function mockFetchByUrl(handlers: Record<string, Handler>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          const result = respond(init) as { __status?: number; body?: unknown };
          if (result && typeof result === 'object' && '__status' in result) {
            return {
              ok: false,
              status: result.__status,
              statusText: 'error',
              json: async () => result.body,
            };
          }
          return { ok: true, json: async () => result };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    }),
  );
}

const empty = { base_url: '', has_api_key: false };
const connected = { base_url: 'http://geopulse:8080', has_api_key: true };

function renderSettings() {
  return render(
    <SnackbarProvider>
      <GeoPulseSettings />
    </SnackbarProvider>,
  );
}

test('renders the connect form when nothing is configured, with no test/remove buttons', async () => {
  mockFetchByUrl({ '/geopulse/config': () => empty });
  renderSettings();

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  expect(screen.getByLabelText('API Token')).toBeInTheDocument();
  expect(
    screen.getByText('Create a token in GeoPulse (Profile → Security) and paste it here.'),
  ).toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Test connection' })).not.toBeInTheDocument();
  expect(screen.queryByRole('button', { name: 'Remove connection' })).not.toBeInTheDocument();
});

test('saving PUTs the base URL and API token, and the token field is cleared afterwards', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/geopulse/config': (init) => {
      if (init?.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return connected;
      }
      return empty;
    },
  });
  renderSettings();

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Base URL'), {
    target: { value: '  http://geopulse:8080  ' },
  });
  fireEvent.change(screen.getByLabelText('API Token'), { target: { value: ' sekret ' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toEqual({ base_url: 'http://geopulse:8080', api_key: 'sekret' });
  await waitFor(() => expect(screen.getByLabelText('API Token')).toHaveValue(''));
  expect(screen.getByLabelText('Base URL')).toHaveValue('http://geopulse:8080');
  expect(screen.queryByText(/sekret/)).not.toBeInTheDocument();
});

test('saving with the token field empty omits api_key so the stored token is kept', async () => {
  let putBody: Record<string, unknown> | null = null;
  mockFetchByUrl({
    '/geopulse/config': (init) => {
      if (init?.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return connected;
      }
      return connected;
    },
  });
  renderSettings();

  await waitFor(() =>
    expect(screen.getByLabelText('Base URL')).toHaveValue('http://geopulse:8080'),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toEqual({ base_url: 'http://geopulse:8080' });
  expect(putBody).not.toHaveProperty('api_key');
});

test('an empty base URL is rejected client-side without sending a request', async () => {
  let putSeen = false;
  mockFetchByUrl({
    '/geopulse/config': (init) => {
      if (init?.method === 'PUT') putSeen = true;
      return empty;
    },
  });
  renderSettings();

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(screen.getByText('Base URL is required.')).toBeInTheDocument());
  expect(putSeen).toBe(false);
});

test('a non-http(s) base URL is rejected client-side without sending a request', async () => {
  let putSeen = false;
  mockFetchByUrl({
    '/geopulse/config': (init) => {
      if (init?.method === 'PUT') putSeen = true;
      return empty;
    },
  });
  renderSettings();

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'ftp://geopulse' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() =>
    expect(screen.getByText('Base URL must be an http:// or https:// URL.')).toBeInTheDocument(),
  );
  expect(putSeen).toBe(false);

  // Editing the field clears the stale error.
  fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'http://geopulse' } });
  expect(
    screen.queryByText('Base URL must be an http:// or https:// URL.'),
  ).not.toBeInTheDocument();
});

test('a server-side save failure is shown inline', async () => {
  mockFetchByUrl({
    '/geopulse/config': (init) =>
      init?.method === 'PUT'
        ? {
            __status: 400,
            body: { error: { code: 'VALIDATION_ERROR', message: 'an API token is required' } },
          }
        : empty,
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  renderSettings();

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'http://geopulse' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  // Shown both inline (so it stays next to the field) and as a toast.
  await waitFor(() =>
    expect(screen.getAllByText(/api token is required/i).length).toBeGreaterThan(0),
  );
  // The button is usable again.
  expect(screen.getByRole('button', { name: 'Save connection' })).toBeEnabled();
});

test('a failure to load the config is shown', async () => {
  mockFetchByUrl({
    '/geopulse/config': () => ({
      __status: 500,
      body: { error: { code: 'INTERNAL_ERROR', message: 'database is down' } },
    }),
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  renderSettings();

  await waitFor(() => expect(screen.getByText(/database is down/i)).toBeInTheDocument());
});

test('a configured connection shows the test-connection and remove buttons', async () => {
  mockFetchByUrl({ '/geopulse/config': () => connected });
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument();
  expect(
    screen.getByText('A token is already stored. Leave the field empty to keep it.'),
  ).toBeInTheDocument();
});

test('test connection shows the backend-diagnosed success message', async () => {
  mockFetchByUrl({
    '/geopulse/config': () => connected,
    '/geopulse/test-connection': () => ({
      ok: true,
      stage: 'ok',
      message: 'Connected to GeoPulse as Test User',
    }),
  });
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() =>
    expect(screen.getByText('Connected to GeoPulse as Test User')).toBeInTheDocument(),
  );
});

test('test connection shows the backend-diagnosed failure message, not a generic one', async () => {
  mockFetchByUrl({
    '/geopulse/config': () => connected,
    '/geopulse/test-connection': () => ({
      ok: false,
      stage: 'auth',
      message: 'GeoPulse rejected the API token.',
    }),
  });
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() =>
    expect(screen.getByText('GeoPulse rejected the API token.')).toBeInTheDocument(),
  );
});

test('a test that cannot run (e.g. the connection vanished) does not crash the card', async () => {
  mockFetchByUrl({
    '/geopulse/config': () => connected,
    '/geopulse/test-connection': () => ({
      __status: 400,
      body: { error: { code: 'VALIDATION_ERROR', message: 'GeoPulse is not configured' } },
    }),
  });
  vi.spyOn(console, 'error').mockImplementation(() => {});
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeEnabled(),
  );
  expect(screen.getByLabelText('Base URL')).toBeInTheDocument();
});

test('removing the connection prompts for confirmation and clears the form on confirm', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  mockFetchByUrl({
    '/geopulse/config': (init) => (init?.method === 'DELETE' ? {} : connected),
  });
  const fetchMock = vi.mocked(fetch);
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(window.confirm).toHaveBeenCalledWith(
    'Remove the GeoPulse connection? Activities you already logged from location history are kept.',
  );
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/geopulse/config'),
      expect.objectContaining({ method: 'DELETE' }),
    ),
  );
  await waitFor(() => expect(screen.getByLabelText('Base URL')).toHaveValue(''));
  expect(screen.queryByRole('button', { name: 'Remove connection' })).not.toBeInTheDocument();
});

test('a failed removal is reported and leaves the form as it was', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  vi.spyOn(console, 'error').mockImplementation(() => {});
  mockFetchByUrl({
    '/geopulse/config': (init) =>
      init?.method === 'DELETE'
        ? { __status: 500, body: { error: { code: 'DATABASE_ERROR', message: 'delete failed' } } }
        : connected,
  });
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  await waitFor(() => expect(screen.getByText(/delete failed/i)).toBeInTheDocument());
  expect(screen.getByLabelText('Base URL')).toHaveValue('http://geopulse:8080');
});

test('cancelling the remove confirmation leaves the connection in place', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  mockFetchByUrl({ '/geopulse/config': () => connected });
  const fetchMock = vi.mocked(fetch);
  renderSettings();

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(fetchMock).not.toHaveBeenCalledWith(
    expect.stringContaining('/geopulse/config'),
    expect.objectContaining({ method: 'DELETE' }),
  );
  expect(screen.getByLabelText('Base URL')).toHaveValue('http://geopulse:8080');
});
