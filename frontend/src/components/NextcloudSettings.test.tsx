import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import NextcloudSettings from './NextcloudSettings';

// See ImmichSettings.test.tsx for the full explanation of this race: the
// config-load effect and the separate effect that mirrors `config` onto local
// field state flush in two different passive-effect passes, so a field edit
// right after the initial waitFor can be clobbered by the still-pending
// mirror effect. settle() flushes it before any interaction.
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
    }),
  );
}

test('renders the connect form when nothing is configured', async () => {
  mockFetchByUrl({
    '/nextcloud/config': () => ({
      base_url: '',
      username: '',
      has_app_password: false,
    }),
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByLabelText('Server URL')).toBeInTheDocument();
  });
  expect(screen.getByLabelText('Username')).toBeInTheDocument();
  expect(screen.getByLabelText('App Password')).toBeInTheDocument();
});

test('saving posts the base URL, username and app password, and the password never comes back', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/nextcloud/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return {
          base_url: 'https://nextcloud.example.com',
          username: 'alice',
          has_app_password: true,
        };
      }
      return { base_url: '', username: '', has_app_password: false };
    },
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Server URL'), {
    target: { value: 'https://nextcloud.example.com' },
  });
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });
  fireEvent.change(screen.getByLabelText('App Password'), { target: { value: 'sekret-app-pw' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toMatchObject({
    base_url: 'https://nextcloud.example.com',
    username: 'alice',
    app_password: 'sekret-app-pw',
  });

  await waitFor(() => expect(screen.getByLabelText('App Password')).toHaveValue(''));
});

test('an empty username is rejected client-side without sending a request', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/nextcloud/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
      }
      return { base_url: '', username: '', has_app_password: false };
    },
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Server URL'), {
    target: { value: 'https://nextcloud.example.com' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(screen.getByText('Username is required.')).toBeInTheDocument());
  expect(putBody).toBeNull();
});

test('a non-http(s) base URL is rejected client-side without sending a request (T41)', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/nextcloud/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
      }
      return { base_url: '', username: '', has_app_password: false };
    },
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Server URL'), {
    target: { value: 'nextcloud.example.com' },
  });
  fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'alice' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(screen.getByText(/http:\/\/ or https:\/\//i)).toBeInTheDocument());
  expect(putBody).toBeNull();
});

test('a configured connection shows the test-connection and remove buttons', async () => {
  mockFetchByUrl({
    '/nextcloud/config': () => ({
      base_url: 'https://nextcloud.example.com',
      username: 'alice',
      has_app_password: true,
    }),
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument();
  });
  expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument();
  expect(
    screen.getByText('An app password is already stored. Leave the field empty to keep it.'),
  ).toBeInTheDocument();
});

test('test connection shows the backend-diagnosed success message', async () => {
  mockFetchByUrl({
    '/nextcloud/config': () => ({
      base_url: 'https://nextcloud.example.com',
      username: 'alice',
      has_app_password: true,
    }),
    '/nextcloud/test-connection': () => ({
      ok: true,
      stage: 'ok',
      message: 'Connected to Nextcloud as alice',
    }),
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Connected to Nextcloud as alice')).toBeInTheDocument();
  });
});

test('test connection shows the backend-diagnosed failure message, not a generic one', async () => {
  mockFetchByUrl({
    '/nextcloud/config': () => ({
      base_url: 'https://nextcloud.example.com',
      username: 'alice',
      has_app_password: true,
    }),
    '/nextcloud/test-connection': () => ({
      ok: false,
      stage: 'auth',
      message: 'Nextcloud rejected the app password.',
    }),
  });

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Nextcloud rejected the app password.')).toBeInTheDocument();
  });
});

test('removing the connection prompts for confirmation and clears the form on confirm', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  mockFetchByUrl({
    '/nextcloud/config': (init?: RequestInit) => {
      if (init && init.method === 'DELETE') {
        return {};
      }
      return {
        base_url: 'https://nextcloud.example.com',
        username: 'alice',
        has_app_password: true,
      };
    },
  });
  const fetchMock = vi.mocked(fetch);

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(window.confirm).toHaveBeenCalledWith(
    'Remove the Nextcloud connection? Linked contacts keep their existing file links.',
  );
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/nextcloud/config'),
      expect.objectContaining({ method: 'DELETE' }),
    ),
  );
  await waitFor(() => expect(screen.getByLabelText('Server URL')).toHaveValue(''));
});

test('cancelling the remove confirmation leaves the connection in place', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  mockFetchByUrl({
    '/nextcloud/config': (init?: RequestInit) => {
      if (init && init.method === 'DELETE') {
        return {};
      }
      return {
        base_url: 'https://nextcloud.example.com',
        username: 'alice',
        has_app_password: true,
      };
    },
  });
  const fetchMock = vi.mocked(fetch);

  render(
    <SnackbarProvider>
      <NextcloudSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(fetchMock).not.toHaveBeenCalledWith(
    expect.stringContaining('/nextcloud/config'),
    expect.objectContaining({ method: 'DELETE' }),
  );
  expect(screen.getByLabelText('Server URL')).toHaveValue('https://nextcloud.example.com');
});
