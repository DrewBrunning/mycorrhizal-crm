import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import NotificationSettings from './NotificationSettings';
import { SnackbarProvider } from '../context/SnackbarContext';

beforeEach(() => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1, username: 'test', is_admin: false }));
});

afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.unstubAllGlobals();
});

function mockFetchByUrl(handlers: Record<string, (init?: RequestInit) => unknown>) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      for (const [pattern, respond] of Object.entries(handlers)) {
        if (url.includes(pattern)) {
          return { ok: true, json: async () => respond(init) };
        }
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );
}

const emptyConfig = {
  ntfy_url: '',
  ntfy_topic: '',
  gotify_url: '',
  gotify_has_token: false,
  notify_ntfy: false,
  notify_gotify: false,
  notify_push: false,
  vapid_public_key: 'public-key-abc',
};

test('renders the settings card with all three channels', async () => {
  mockFetchByUrl({
    '/notifications/config': () => emptyConfig,
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText('Send reminders via ntfy')).toBeInTheDocument());
  expect(screen.getByText('Send reminders via Gotify')).toBeInTheDocument();
  expect(screen.getByText('Send reminders via browser push')).toBeInTheDocument();
  // No devices registered yet.
  expect(screen.getByText('No devices registered yet.')).toBeInTheDocument();
});

test('saving posts the channel config and per-user toggles', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/notifications/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return {
          ntfy_url: 'https://ntfy.example.com',
          ntfy_topic: 'alerts',
          gotify_url: '',
          gotify_has_token: false,
          notify_ntfy: true,
          notify_gotify: false,
          notify_push: false,
          vapid_public_key: 'public-key-abc',
        };
      }
      return emptyConfig;
    },
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByLabelText('ntfy server URL')).toBeInTheDocument());
  fireEvent.change(screen.getByLabelText('ntfy server URL'), { target: { value: 'https://ntfy.example.com' } });
  fireEvent.change(screen.getByLabelText('Topic'), { target: { value: 'alerts' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save notification settings' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toMatchObject({
    ntfy_url: 'https://ntfy.example.com',
    ntfy_topic: 'alerts',
  });
  // The empty gotify token is omitted from the wire (write-only, keep-stored).
  expect('gotify_token' in (putBody as Record<string, unknown>)).toBe(false);
});

test('warns about a private ntfy URL before it is saved', async () => {
  mockFetchByUrl({
    '/notifications/config': () => emptyConfig,
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByLabelText('ntfy server URL')).toBeInTheDocument());
  expect(screen.queryByText(/WEBHOOK_BLOCK_PRIVATE_URLS/)).not.toBeInTheDocument();

  fireEvent.change(screen.getByLabelText('ntfy server URL'), { target: { value: 'http://localhost:8000' } });

  await waitFor(() => {
    expect(screen.getByText(/WEBHOOK_BLOCK_PRIVATE_URLS/)).toBeInTheDocument();
  });
});

test('test notification shows the backend-diagnosed success', async () => {
  let testChannel: unknown = null;
  mockFetchByUrl({
    // Must be registered before /notifications/config so the POST matches this
    // handler, not the config GET/PUT.
    '/notifications/config/test': (init?: RequestInit) => {
      testChannel = init?.body ? JSON.parse(String(init.body)).channel : null;
      return { ok: true };
    },
    '/notifications/config': () => ({
      ntfy_url: 'https://ntfy.example.com',
      ntfy_topic: 'alerts',
      gotify_url: '',
      gotify_has_token: false,
      notify_ntfy: true,
      notify_gotify: false,
      notify_push: false,
      vapid_public_key: 'public-key-abc',
    }),
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getAllByRole('button', { name: 'Send test notification' })[0]).toBeInTheDocument());
  fireEvent.click(screen.getAllByRole('button', { name: 'Send test notification' })[0]);

  await waitFor(() => expect(testChannel).not.toBeNull());
  expect(testChannel).toBe('ntfy');
  await waitFor(() => expect(screen.getByText(/Test notification sent via ntfy/)).toBeInTheDocument());
});

test('test failure surfaces the backend reason, not a generic message', async () => {
  mockFetchByUrl({
    '/notifications/config/test': () => ({ ok: false, error: 'unexpected status 401 from notification endpoint' }),
    '/notifications/config': () => ({
      ntfy_url: 'https://ntfy.example.com',
      ntfy_topic: 'alerts',
      gotify_url: '',
      gotify_has_token: false,
      notify_ntfy: true,
      notify_gotify: false,
      notify_push: false,
      vapid_public_key: 'public-key-abc',
    }),
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getAllByRole('button', { name: 'Send test notification' })[0]).toBeInTheDocument());
  fireEvent.click(screen.getAllByRole('button', { name: 'Send test notification' })[0]);

  await waitFor(() => {
    expect(screen.getByText(/unexpected status 401 from notification endpoint/)).toBeInTheDocument();
  });
});

test('registered devices are listed and can be removed', async () => {
  let deleteId: number | null = null;
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      if (String(url).includes('/notifications/config')) {
        return { ok: true, json: async () => emptyConfig };
      }
      if (String(url).includes('/notifications/push-subscriptions')) {
        // The DELETE response body is never read, so the branch must run
        // eagerly here rather than inside the lazy json() body.
        if (init && init.method === 'DELETE') {
          deleteId = Number(String(url).split('/').pop());
          return { ok: true, json: async () => ({}) };
        }
        return {
          ok: true,
          json: async () => ({
            subscriptions: [
              { id: 7, endpoint: 'https://push.example.com/x', p256dh: 'k', auth: 'a', device_label: 'Chrome', created_at: '2026-01-01T00:00:00Z' },
            ],
          }),
        };
      }
      if (String(url).includes('/notifications/devices')) {
        return { ok: true, json: async () => ({ devices: [] }) };
      }
      throw new Error(`unexpected fetch: ${url}`);
    })
  );

  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText('Chrome')).toBeInTheDocument());
  fireEvent.click(screen.getByTitle('Remove this device'));

  await waitFor(() => expect(deleteId).toBe(7));
  confirmSpy.mockRestore();
});

test('the push enable button is disabled when the browser lacks push support', async () => {
  mockFetchByUrl({
    '/notifications/config': () => emptyConfig,
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': () => ({ devices: [] }),
  });

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText('Enable browser notifications')).toBeInTheDocument());
  // jsdom has no serviceWorker/PushManager, so the button must be disabled.
  expect(screen.getByRole('button', { name: 'Enable browser notifications' })).toBeDisabled();
});

test('registered mobile devices (M2) are listed, can be removed, and unblock the push test button', async () => {
  let deleteId: number | null = null;
  mockFetchByUrl({
    '/notifications/config': () => ({ ...emptyConfig, notify_push: true }),
    '/notifications/push-subscriptions': () => ({ subscriptions: [] }),
    '/notifications/devices': (init?: RequestInit) => {
      if (init && init.method === 'DELETE') {
        return {};
      }
      return {
        devices: [
          { id: 3, token: 'fcm-token-abc', client: 'fcm', device_label: "Drew's Pixel", created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z' },
        ],
      };
    },
  });
  // The DELETE response body is never read; override fetch for that one case
  // so the id makes it out of the mock (mockFetchByUrl's json() is lazy).
  const realFetch = globalThis.fetch;
  vi.stubGlobal('fetch', vi.fn(async (url: string, init?: RequestInit) => {
    if (String(url).includes('/notifications/devices/') && init?.method === 'DELETE') {
      deleteId = Number(String(url).split('/').pop());
      return { ok: true, json: async () => ({}) };
    }
    return realFetch(url, init);
  }));

  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);

  render(
    <SnackbarProvider>
      <NotificationSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByText("Drew's Pixel")).toBeInTheDocument());
  // No browser subscriptions but one mobile device: the push test button must
  // be enabled (M2 design decision 5 — the test button probes both). The
  // three "Send test notification" buttons are ntfy/Gotify/push in that
  // document order; the push one is last.
  const testButtons = screen.getAllByRole('button', { name: 'Send test notification' });
  expect(testButtons[testButtons.length - 1]).not.toBeDisabled();

  fireEvent.click(screen.getByTitle('Remove this device'));
  await waitFor(() => expect(deleteId).toBe(3));

  confirmSpy.mockRestore();
});
