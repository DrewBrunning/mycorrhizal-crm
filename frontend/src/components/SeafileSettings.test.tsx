import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { SnackbarProvider } from '../context/SnackbarContext';
import SeafileSettings from './SeafileSettings';

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
    '/seafile/config': () => ({
      base_url: '',
      has_api_token: false,
    }),
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByLabelText('Server URL')).toBeInTheDocument();
  });
  expect(screen.getByLabelText('API Token')).toBeInTheDocument();
});

test('saving posts the base URL and API token, and the token never comes back', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/seafile/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return {
          base_url: 'https://seafile.example.com',
          has_api_token: true,
        };
      }
      return { base_url: '', has_api_token: false };
    },
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Server URL'), {
    target: { value: 'https://seafile.example.com' },
  });
  fireEvent.change(screen.getByLabelText('API Token'), { target: { value: 'sekret-token' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toMatchObject({
    base_url: 'https://seafile.example.com',
    api_token: 'sekret-token',
  });

  await waitFor(() => expect(screen.getByLabelText('API Token')).toHaveValue(''));
});

test('an empty base URL is rejected client-side without sending a request', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/seafile/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
      }
      return { base_url: '', has_api_token: false };
    },
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(screen.getByText('Server URL is required.')).toBeInTheDocument());
  expect(putBody).toBeNull();
});

test('a non-http(s) base URL is rejected client-side without sending a request (T41)', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/seafile/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
      }
      return { base_url: '', has_api_token: false };
    },
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => expect(screen.getByLabelText('Server URL')).toBeInTheDocument());
  await settle();
  fireEvent.change(screen.getByLabelText('Server URL'), {
    target: { value: 'seafile.example.com' },
  });
  fireEvent.change(screen.getByLabelText('API Token'), { target: { value: 'sekret-token' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(screen.getByText(/http:\/\/ or https:\/\//i)).toBeInTheDocument());
  expect(putBody).toBeNull();
});

test('a configured connection shows the test-connection and remove buttons', async () => {
  mockFetchByUrl({
    '/seafile/config': () => ({
      base_url: 'https://seafile.example.com',
      has_api_token: true,
    }),
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument();
  });
  expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument();
  expect(
    screen.getByText('A token is already stored. Leave the field empty to keep it.'),
  ).toBeInTheDocument();
});

test('test connection shows the backend-diagnosed success message', async () => {
  mockFetchByUrl({
    '/seafile/config': () => ({
      base_url: 'https://seafile.example.com',
      has_api_token: true,
    }),
    '/seafile/test-connection': () => ({
      ok: true,
      stage: 'ok',
      message: 'Connected to Seafile',
    }),
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Connected to Seafile')).toBeInTheDocument();
  });
});

test('test connection shows the backend-diagnosed failure message, not a generic one', async () => {
  mockFetchByUrl({
    '/seafile/config': () => ({
      base_url: 'https://seafile.example.com',
      has_api_token: true,
    }),
    '/seafile/test-connection': () => ({
      ok: false,
      stage: 'auth',
      message: 'Seafile rejected the API token.',
    }),
  });

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument(),
  );
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Seafile rejected the API token.')).toBeInTheDocument();
  });
});

test('removing the connection prompts for confirmation and clears the form on confirm', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true);
  mockFetchByUrl({
    '/seafile/config': (init?: RequestInit) => {
      if (init && init.method === 'DELETE') {
        return {};
      }
      return { base_url: 'https://seafile.example.com', has_api_token: true };
    },
  });
  const fetchMock = vi.mocked(fetch);

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(window.confirm).toHaveBeenCalledWith(
    'Remove the Seafile connection? Linked contacts keep their existing file links.',
  );
  await waitFor(() =>
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/seafile/config'),
      expect.objectContaining({ method: 'DELETE' }),
    ),
  );
  await waitFor(() => expect(screen.getByLabelText('Server URL')).toHaveValue(''));
});

test('cancelling the remove confirmation leaves the connection in place', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  mockFetchByUrl({
    '/seafile/config': (init?: RequestInit) => {
      if (init && init.method === 'DELETE') {
        return {};
      }
      return { base_url: 'https://seafile.example.com', has_api_token: true };
    },
  });
  const fetchMock = vi.mocked(fetch);

  render(
    <SnackbarProvider>
      <SeafileSettings />
    </SnackbarProvider>,
  );

  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument(),
  );
  await settle();
  fireEvent.click(screen.getByRole('button', { name: 'Remove connection' }));

  expect(fetchMock).not.toHaveBeenCalledWith(
    expect.stringContaining('/seafile/config'),
    expect.objectContaining({ method: 'DELETE' }),
  );
  expect(screen.getByLabelText('Server URL')).toHaveValue('https://seafile.example.com');
});
