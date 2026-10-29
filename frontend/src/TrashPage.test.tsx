import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { SnackbarProvider } from './context/SnackbarContext';
import TrashPage from './TrashPage';

afterEach(cleanup);
afterEach(() => vi.unstubAllGlobals());

const summary = (id: number, firstname: string) => ({
  id,
  uid: `uid-${id}`,
  firstname,
  lastname: 'Test',
  nickname: '',
  fn: `${firstname} Test`,
  primary_email: '',
  primary_phone: '',
  birthday: '',
  org: '',
  photo: '',
  archived: false,
  is_favorite: false,
  revision: 1,
});

type Row = { contact: ReturnType<typeof summary>; deleted_at: string; purge_at: string | null };
let rows: Row[];
let retention = 30;
let calls: { method: string; url: string }[];
let restoreStatus = 200;

beforeEach(() => {
  retention = 30;
  restoreStatus = 200;
  calls = [];
  rows = [
    {
      contact: summary(1, 'Ada'),
      deleted_at: '2026-10-01T00:00:00Z',
      purge_at: '2026-10-31T00:00:00Z',
    },
    {
      contact: summary(2, 'Bob'),
      deleted_at: '2026-10-02T00:00:00Z',
      purge_at: '2026-11-01T00:00:00Z',
    },
  ];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      const method = init?.method ?? 'GET';
      calls.push({ method, url });
      if (method === 'GET') {
        return {
          ok: true,
          json: async () => ({
            contacts: rows,
            total: rows.length,
            page: 1,
            limit: 25,
            retention_days: retention,
          }),
        };
      }
      if (method === 'POST') {
        if (restoreStatus === 409) {
          return {
            ok: false,
            status: 409,
            statusText: 'Conflict',
            json: async () => ({ error: { code: 'CONFLICT', message: 'uid in use' } }),
          };
        }
        const id = Number(url.match(/trash\/contacts\/(\d+)\/restore/)?.[1]);
        const skipped = id === 2 ? [{ table: 'relationship_edges', reason: 'r', count: 3 }] : [];
        rows = rows.filter((r) => r.contact.id !== id);
        return { ok: true, json: async () => ({ contact: summary(id, 'x'), skipped }) };
      }
      const id = url.match(/trash\/contacts\/(\d+)$/)?.[1];
      const purged = id ? 1 : rows.length;
      rows = id ? rows.filter((r) => r.contact.id !== Number(id)) : [];
      return { ok: true, json: async () => ({ message: 'ok', purged }) };
    }),
  );
});

function renderPage() {
  return render(
    <SnackbarProvider>
      <TrashPage />
    </SnackbarProvider>,
  );
}

test('lists trashed contacts with deleted and purge dates', async () => {
  renderPage();
  expect(await screen.findByText('Ada Test')).toBeTruthy();
  expect(screen.getByText('Bob Test')).toBeTruthy();
  expect(screen.getAllByText(/Deleted/).length).toBeGreaterThan(0);
  expect(screen.getAllByText(/Deletes forever/).length).toBe(2);
});

test('retention 0 says it is kept until emptied instead of showing a purge date', async () => {
  retention = 0;
  rows = rows.map((r) => ({ ...r, purge_at: null }));
  renderPage();
  await screen.findByText('Ada Test');
  expect(screen.queryByText(/Deletes forever/)).toBeNull();
  expect(screen.getByText(/kept until you empty the trash/i)).toBeTruthy();
});

test('shows the empty state', async () => {
  rows = [];
  renderPage();
  expect(await screen.findByText(/Trash is empty/i)).toBeTruthy();
  expect(
    screen.queryByRole('button', { name: /empty trash/i }) as HTMLButtonElement | null,
  ).toBeNull();
});

test('restore calls the route, removes the row and reports skipped rows', async () => {
  renderPage();
  const bob = (await screen.findByText('Bob Test')).closest('li') as HTMLElement;
  fireEvent.click(within(bob).getByRole('button', { name: /restore/i }));
  await waitFor(() => expect(screen.queryByText('Bob Test')).toBeNull());
  expect(
    calls.some((c) => c.method === 'POST' && c.url.includes('/trash/contacts/2/restore')),
  ).toBe(true);
  expect(await screen.findByText(/3 items could not be restored/i)).toBeTruthy();
});

test('restore 409 explains a live contact already uses the UID and keeps the row', async () => {
  restoreStatus = 409;
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;
  fireEvent.click(within(ada).getByRole('button', { name: /restore/i }));
  expect(await screen.findByText(/already uses this contact/i)).toBeTruthy();
  expect(screen.getByText('Ada Test')).toBeTruthy();
});

test('delete forever asks for confirmation and only purges when confirmed', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm');
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;

  confirmSpy.mockReturnValueOnce(false);
  fireEvent.click(within(ada).getByRole('button', { name: /delete forever/i }));
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);

  confirmSpy.mockReturnValueOnce(true);
  fireEvent.click(within(ada).getByRole('button', { name: /delete forever/i }));
  await waitFor(() => expect(screen.queryByText('Ada Test')).toBeNull());
  expect(calls.some((c) => c.method === 'DELETE' && /trash\/contacts\/1$/.test(c.url))).toBe(true);
  confirmSpy.mockRestore();
});

test('empty trash confirms then purges everything', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  renderPage();
  await screen.findByText('Ada Test');
  fireEvent.click(screen.getByRole('button', { name: /empty trash/i }));
  expect(await screen.findByText(/Trash is empty/i)).toBeTruthy();
  expect(calls.some((c) => c.method === 'DELETE' && /trash\/contacts$/.test(c.url))).toBe(true);
  confirmSpy.mockRestore();
});

test('a load failure shows a retry control', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: false,
      status: 500,
      statusText: 'x',
      json: async () => ({ error: { code: 'DATABASE_ERROR', message: 'boom' } }),
    })),
  );
  renderPage();
  expect(await screen.findByRole('button', { name: /retry/i })).toBeTruthy();
});
