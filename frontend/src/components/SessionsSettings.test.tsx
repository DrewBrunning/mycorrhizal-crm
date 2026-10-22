import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import SessionsSettings from './SessionsSettings';

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
});

type MockResponse = unknown | { body: unknown; ok?: boolean };

function mockFetchByUrl(
  handlers: Record<string, (method: string, init?: RequestInit) => MockResponse>,
) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = (init?.method || 'GET').toUpperCase();
      for (const [pattern, respond] of Object.entries(handlers)) {
        const [pMethod, pUrl] = pattern.includes(' ') ? pattern.split(' ') : ['', pattern];
        if (url.includes(pUrl) && (!pMethod || pMethod === method)) {
          const result = respond(method, init);
          const body =
            result && typeof result === 'object' && 'body' in result ? result.body : result;
          const ok = result && typeof result === 'object' && 'ok' in result ? !!result.ok : true;
          return { ok, json: async () => body, text: async () => JSON.stringify(body) };
        }
      }
      throw new Error(`unexpected fetch: ${method} ${url}`);
    }),
  );
}

const twoSessions = {
  sessions: [
    {
      id: 'sid-current',
      created_at: '2026-09-01T10:00:00Z',
      last_seen_at: '2026-09-09T09:00:00Z',
      expires_at: '2026-09-13T10:00:00Z',
      user_agent:
        'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/153.0.0.0 Safari/537.36',
      ip: '203.0.113.4',
      current: true,
    },
    {
      id: 'sid-phone',
      created_at: '2026-09-05T08:00:00Z',
      last_seen_at: '2026-09-08T20:00:00Z',
      expires_at: '2026-09-17T08:00:00Z',
      user_agent:
        'Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1',
      ip: '198.51.100.9',
      current: false,
    },
  ],
};

test('shows a human-readable device name with the raw user-agent in a tooltip', async () => {
  mockFetchByUrl({ 'GET /sessions': () => twoSessions });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  const label = await screen.findByText('Chrome 153 on Linux');
  expect(screen.queryByText(/Mozilla\/5\.0/)).not.toBeInTheDocument();
  fireEvent.mouseOver(label);
  const tooltip = await screen.findByRole('tooltip');
  expect(tooltip).toHaveTextContent(/Mozilla\/5\.0 .*HeadlessChrome\/153/);
});

test('an unparseable user-agent falls back to Unknown device but keeps the raw string in a tooltip', async () => {
  mockFetchByUrl({
    'GET /sessions': () => ({
      sessions: [{ ...twoSessions.sessions[0], user_agent: 'Some Completely Unknown Agent/1.0' }],
    }),
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  const label = await screen.findByText('Unknown device');
  fireEvent.mouseOver(label);
  const tooltip = await screen.findByRole('tooltip');
  expect(tooltip).toHaveTextContent('Some Completely Unknown Agent/1.0');
});

test('lists sessions and marks the current one', async () => {
  mockFetchByUrl({ 'GET /sessions': () => twoSessions });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Active sessions')).toBeInTheDocument());
  expect(screen.getByText('Chrome 153 on Linux')).toBeInTheDocument();
  expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument();
  expect(screen.getByText('This device')).toBeInTheDocument();
});

// #196 (audit gap n-1): the section title rendered <h6>, jumping the settings
// heading order h2 -> h6 between its sibling sections. It must be an <h2> like
// TwoFactorSettings / WebhooksSettings.
test('section title is a level-2 heading', async () => {
  mockFetchByUrl({ 'GET /sessions': () => twoSessions });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('heading', { level: 2, name: 'Active sessions' })).toBeInTheDocument(),
  );
});

test('revoking a session calls the API and refetches', async () => {
  const getHandler = vi.fn(() => twoSessions);
  const delHandler = vi.fn(() => ({ message: 'Session revoked' }));
  mockFetchByUrl({
    'GET /sessions': getHandler,
    'DELETE /sessions/sid-phone': delHandler,
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument());

  const phoneRow = screen.getByText('Safari 17 on iPhone').closest('tr') as HTMLElement;
  fireEvent.click(within(phoneRow).getByRole('button', { name: 'Revoke' }));

  await waitFor(() => expect(delHandler).toHaveBeenCalledTimes(1));
  // one initial load + one refetch after the revoke
  await waitFor(() => expect(getHandler).toHaveBeenCalledTimes(2));
});

test('"log out all other devices" revokes the rest and refetches', async () => {
  const getHandler = vi.fn(() => twoSessions);
  const delAllHandler = vi.fn(() => ({ message: 'Other sessions revoked', revoked: 1 }));
  mockFetchByUrl({
    'GET /sessions': getHandler,
    'DELETE /sessions': delAllHandler,
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Log out all other devices' }));

  await waitFor(() => expect(delAllHandler).toHaveBeenCalledTimes(1));
  await waitFor(() => expect(getHandler).toHaveBeenCalledTimes(2));
  expect(await screen.findByText('1 other session revoked')).toBeInTheDocument();
});

test('the button is hidden and the unknown-device fallback shows for a lone session', async () => {
  mockFetchByUrl({
    'GET /sessions': () => ({
      sessions: [{ ...twoSessions.sessions[0], user_agent: '', ip: '' }],
    }),
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Unknown device')).toBeInTheDocument());
  expect(
    screen.queryByRole('button', { name: 'Log out all other devices' }),
  ).not.toBeInTheDocument();
});

test('shows an error alert when the list fails to load', async () => {
  mockFetchByUrl({
    'GET /sessions': () => ({ ok: false, body: { error: 'sessions are down' } }),
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  // The refresh() catch branch surfaces the failure as an inline Alert.
  expect(await screen.findByText('sessions are down')).toBeInTheDocument();
  expect(screen.queryByText('Active sessions')).toBeInTheDocument(); // card still renders
});

test('renders the empty state when there are no sessions', async () => {
  mockFetchByUrl({ 'GET /sessions': () => ({ sessions: [] }) });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  expect(await screen.findByText('No active sessions')).toBeInTheDocument();
});

test('surfaces a snackbar error when revoking fails', async () => {
  mockFetchByUrl({
    'GET /sessions': () => twoSessions,
    'DELETE /sessions/sid-phone': () => ({ ok: false, body: { error: 'nope' } }),
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument());
  const phoneRow = screen.getByText('Safari 17 on iPhone').closest('tr') as HTMLElement;
  fireEvent.click(within(phoneRow).getByRole('button', { name: 'Revoke' }));

  expect(await screen.findByRole('alert')).toBeInTheDocument();
  // the row is still there — the failure didn't wipe the list
  expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument();
});

test('surfaces a snackbar error when "log out others" fails', async () => {
  mockFetchByUrl({
    'GET /sessions': () => twoSessions,
    'DELETE /sessions': () => ({ ok: false, body: { error: 'nope' } }),
  });

  render(
    <SnackbarProvider>
      <SessionsSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByText('Safari 17 on iPhone')).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Log out all other devices' }));

  expect(await screen.findByRole('alert')).toBeInTheDocument();
});
