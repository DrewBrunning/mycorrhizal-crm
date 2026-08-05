import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup, fireEvent, waitFor } from '@testing-library/react';
import '../i18n/config';
import ImmichSettings from './ImmichSettings';
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

test('renders the connect form when nothing is configured', async () => {
  mockFetchByUrl({ '/immich/config': () => ({ base_url: '', has_api_key: false, sync_enabled: true, last_sync_status: '', last_sync_error: '' }) });

  render(
    <SnackbarProvider>
      <ImmichSettings />
    </SnackbarProvider>
  );

  await waitFor(() => {
    expect(screen.getByLabelText('Base URL')).toBeInTheDocument();
  });
  expect(screen.getByLabelText('API Key')).toBeInTheDocument();
});

test('saving posts the base URL and API key, and the key never comes back', async () => {
  let putBody: unknown = null;
  mockFetchByUrl({
    '/immich/config': (init?: RequestInit) => {
      if (init && init.method === 'PUT') {
        putBody = JSON.parse(String(init.body));
        return { base_url: 'http://immich:2283', has_api_key: true, sync_enabled: true, last_sync_status: '', last_sync_error: '' };
      }
      return { base_url: '', has_api_key: false, sync_enabled: true, last_sync_status: '', last_sync_error: '' };
    },
  });

  render(
    <SnackbarProvider>
      <ImmichSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByLabelText('Base URL')).toBeInTheDocument());
  fireEvent.change(screen.getByLabelText('Base URL'), { target: { value: 'http://immich:2283' } });
  fireEvent.change(screen.getByLabelText('API Key'), { target: { value: 'sekret-key' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save connection' }));

  await waitFor(() => expect(putBody).not.toBeNull());
  expect(putBody).toMatchObject({ base_url: 'http://immich:2283', api_key: 'sekret-key' });

  // After save the API key field is cleared (the backend never returns it).
  await waitFor(() => expect(screen.getByLabelText('API Key')).toHaveValue(''));
});

test('a configured connection shows the sync toggle and remove button', async () => {
  mockFetchByUrl({
    '/immich/config': () => ({ base_url: 'http://immich:2283', has_api_key: true, sync_enabled: true, last_sync_status: 'success', last_sync_error: '' }),
  });

  render(
    <SnackbarProvider>
      <ImmichSettings />
    </SnackbarProvider>
  );

  await waitFor(() => {
    expect(screen.getByRole('button', { name: 'Remove connection' })).toBeInTheDocument();
  });
  const toggle = screen.getByRole('switch');
  expect(toggle).toBeChecked();
  expect(screen.getByText('Automatically sync photo appearances')).toBeInTheDocument();
});

test('test connection shows the backend-diagnosed success message', async () => {
  mockFetchByUrl({
    '/immich/config': () => ({ base_url: 'http://immich:2283', has_api_key: true, sync_enabled: true, last_sync_status: '', last_sync_error: '' }),
    '/immich/test-connection': () => ({ ok: true, stage: 'ok', message: 'Connected to Immich as alice@example.com' }),
  });

  render(
    <SnackbarProvider>
      <ImmichSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Connected to Immich as alice@example.com')).toBeInTheDocument();
  });
});

test('test connection shows the backend-diagnosed failure message, not a generic one', async () => {
  mockFetchByUrl({
    '/immich/config': () => ({ base_url: 'http://immich:2283', has_api_key: true, sync_enabled: true, last_sync_status: '', last_sync_error: '' }),
    '/immich/test-connection': () => ({ ok: false, stage: 'auth', message: 'Immich rejected the API key.' }),
  });

  render(
    <SnackbarProvider>
      <ImmichSettings />
    </SnackbarProvider>
  );

  await waitFor(() => expect(screen.getByRole('button', { name: 'Test connection' })).toBeInTheDocument());
  fireEvent.click(screen.getByRole('button', { name: 'Test connection' }));

  await waitFor(() => {
    expect(screen.getByText('Immich rejected the API key.')).toBeInTheDocument();
  });
});
