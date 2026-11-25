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

type Row = {
  contact: ReturnType<typeof summary>;
  deleted_at: string;
  purge_at: string | null;
};
let rows: Row[];
let retention = 30;
let calls: { method: string; url: string }[];
let restoreStatus = 200;
type EntityRow = {
  entity: { id: string; kind: string; name: string };
  deleted_at: string;
  purge_at: string | null;
  merged_into?: string;
  merged_into_name?: string;
};
let entityRows: EntityRow[];
let entityRestoreWarns = false;

beforeEach(() => {
  retention = 30;
  restoreStatus = 200;
  entityRestoreWarns = false;
  entityRows = [];
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
      if (url.includes('/trash/entities')) {
        return {
          ok: true,
          json: async () => ({
            entities: entityRows,
            total: entityRows.length,
            page: 1,
            limit: 100,
            retention_days: retention,
          }),
        };
      }
      const entityRestore = url.match(/\/entities\/([^/]+)\/restore$/);
      if (entityRestore && method === 'POST') {
        entityRows = entityRows.filter((r) => r.entity.id !== entityRestore[1]);
        return {
          ok: true,
          json: async () => ({
            entity: { id: entityRestore[1], kind: 'organisation', name: 'x' },
            links_revived: 1,
            same_name_live: entityRestoreWarns,
            skipped: [],
          }),
        };
      }
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
            json: async () => ({
              error: { code: 'CONFLICT', message: 'uid in use' },
            }),
          };
        }
        const id = Number(url.match(/trash\/contacts\/(\d+)\/restore/)?.[1]);
        const skipped = id === 2 ? [{ table: 'relationship_edges', reason: 'r', count: 3 }] : [];
        rows = rows.filter((r) => r.contact.id !== id);
        return {
          ok: true,
          json: async () => ({ contact: summary(id, 'x'), skipped }),
        };
      }
      const id = url.match(/trash\/contacts\/(\d+)$/)?.[1];
      const purged = id ? 1 : rows.length;
      rows = id ? rows.filter((r) => r.contact.id !== Number(id)) : [];
      return { ok: true, json: async () => ({ message: 'ok', purged }) };
    }),
  );
});

function seedEntities() {
  entityRows = [
    {
      entity: { id: 'e1', kind: 'organisation', name: 'Acme' },
      deleted_at: '2026-10-03T00:00:00Z',
      purge_at: '2026-11-02T00:00:00Z',
    },
    {
      entity: { id: 'e2', kind: 'pet', name: 'Rex' },
      deleted_at: '2026-10-04T00:00:00Z',
      purge_at: null,
      merged_into: 'e9',
      merged_into_name: 'Rexy',
    },
  ];
}

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
    screen.queryByRole('button', {
      name: /empty trash/i,
    }) as HTMLButtonElement | null,
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

// #2717: the destructive confirmations are in-app, translated dialogs, not window.confirm.
test('delete forever opens an in-app dialog and only purges when confirmed', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm');
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;

  fireEvent.click(within(ada).getByRole('button', { name: /^delete forever: /i }));
  let dialog = await screen.findByRole('dialog', { name: 'Delete forever?' });
  expect(within(dialog).getByText(/Permanently delete Ada Test\?/)).toBeTruthy();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);

  fireEvent.click(within(ada).getByRole('button', { name: /^delete forever: /i }));
  dialog = await screen.findByRole('dialog', { name: 'Delete forever?' });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete forever' }));
  await waitFor(() => expect(screen.queryByText('Ada Test')).toBeNull());
  expect(calls.some((c) => c.method === 'DELETE' && /trash\/contacts\/1$/.test(c.url))).toBe(true);
  expect(confirmSpy).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

test('empty trash opens an in-app dialog, then purges everything', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm');
  renderPage();
  await screen.findByText('Ada Test');
  fireEvent.click(screen.getByRole('button', { name: /empty trash/i }));
  const dialog = await screen.findByRole('dialog', { name: 'Empty trash?' });
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
  fireEvent.click(within(dialog).getByRole('button', { name: 'Empty trash' }));
  expect(await screen.findByText(/Trash is empty/i)).toBeTruthy();
  expect(calls.some((c) => c.method === 'DELETE' && /trash\/contacts$/.test(c.url))).toBe(true);
  expect(confirmSpy).not.toHaveBeenCalled();
  confirmSpy.mockRestore();
});

test('cancelling the empty-trash dialog purges nothing', async () => {
  renderPage();
  await screen.findByText('Ada Test');
  fireEvent.click(screen.getByRole('button', { name: /empty trash/i }));
  const dialog = await screen.findByRole('dialog', { name: 'Empty trash?' });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
  await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
  expect(screen.getByText('Ada Test')).toBeTruthy();
});

// #2707: the retention line covers every kind of item, not only contacts.
test('the retention line says items, not contacts (#2707)', async () => {
  renderPage();
  await screen.findByText('Ada Test');
  expect(
    screen.getByText('Deleted items are kept for 30 days, then deleted forever.'),
  ).toBeTruthy();
});

test('a load failure shows a retry control', async () => {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => ({
      ok: false,
      status: 500,
      statusText: 'x',
      json: async () => ({
        error: { code: 'DATABASE_ERROR', message: 'boom' },
      }),
    })),
  );
  renderPage();
  expect(await screen.findByRole('button', { name: /retry/i })).toBeTruthy();
});

test('lists trashed entities with their kind and merge origin', async () => {
  seedEntities();
  renderPage();
  const acme = (await screen.findByText('Acme')).closest('li') as HTMLElement;
  expect(within(acme).getByText('Organisation')).toBeTruthy();
  const rex = screen.getByText('Rex').closest('li') as HTMLElement;
  expect(within(rex).getByText('Pet')).toBeTruthy();
  expect(within(rex).getByText(/merged into Rexy/i)).toBeTruthy();
  expect(within(rex).queryByText(/Deletes forever/)).toBeNull();
  // Contacts are still listed alongside.
  expect(screen.getByText('Ada Test')).toBeTruthy();
});

test('restoring an entity uses the entity restore route and removes the row', async () => {
  seedEntities();
  renderPage();
  const acme = (await screen.findByText('Acme')).closest('li') as HTMLElement;
  fireEvent.click(within(acme).getByRole('button', { name: /restore/i }));
  await waitFor(() => expect(screen.queryByText('Acme')).toBeNull());
  expect(calls.some((c) => c.method === 'POST' && c.url.endsWith('/entities/e1/restore'))).toBe(
    true,
  );
  expect(await screen.findByText(/Restored Acme/i)).toBeTruthy();
  expect(screen.getByText('Ada Test')).toBeTruthy();
});

test('restoring an entity warns when another entity has the same name', async () => {
  seedEntities();
  entityRestoreWarns = true;
  renderPage();
  const acme = (await screen.findByText('Acme')).closest('li') as HTMLElement;
  fireEvent.click(within(acme).getByRole('button', { name: /restore/i }));
  expect(await screen.findByText(/same name/i)).toBeTruthy();
});

test('entity rows have no delete-forever action; the empty-trash dialog says it leaves them alone (#2717)', async () => {
  seedEntities();
  renderPage();
  const acme = (await screen.findByText('Acme')).closest('li') as HTMLElement;
  expect(within(acme).queryByRole('button', { name: /^delete forever: /i })).toBeNull();
  fireEvent.click(screen.getByRole('button', { name: /empty trash/i }));
  const dialog = await screen.findByRole('dialog', { name: 'Empty trash?' });
  expect(within(dialog).getByText(/all 2 contacts/)).toBeTruthy();
  expect(within(dialog).getByText(/Organisations, places & pets are not affected/)).toBeTruthy();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Empty trash' }));
  await waitFor(() => expect(screen.queryByText('Ada Test')).toBeNull());
  expect(screen.getByText('Acme')).toBeTruthy();
});

test('a note explains that trashed organisations, places & pets purge automatically (#2717)', async () => {
  seedEntities();
  renderPage();
  await screen.findByText('Acme');
  expect(
    screen.getByText(
      'Organisations, places & pets are deleted forever automatically after 30 days.',
    ),
  ).toBeTruthy();
});

test('no automatic-purge note when retention is off or nothing is trashed (#2717)', async () => {
  seedEntities();
  retention = 0;
  renderPage();
  await screen.findByText('Acme');
  expect(screen.queryByText(/deleted forever automatically/)).toBeNull();
  cleanup();
  retention = 30;
  entityRows = [];
  renderPage();
  await screen.findByText('Ada Test');
  expect(screen.queryByText(/deleted forever automatically/)).toBeNull();
});

test('only entities in the trash is not the empty state', async () => {
  seedEntities();
  rows = [];
  renderPage();
  await screen.findByText('Acme');
  expect(screen.queryByText(/Trash is empty/i)).toBeNull();
  expect(screen.queryByRole('button', { name: /empty trash/i })).toBeNull();
});

// #2706: each row's Restore / Delete forever button names the item.
test('row buttons name the trashed item they act on', async () => {
  renderPage();
  await screen.findByText('Ada Test');
  expect(screen.getByRole('button', { name: 'Restore Ada Test' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Restore Bob Test' })).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Delete forever: Ada Test' })).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Restore' })).toBeNull();
});

// #2918 / #2920: an action that removes its own row must not drop focus on <body>.
test('restoring a contact moves focus to the next row, then the heading', async () => {
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;
  fireEvent.click(within(ada).getByRole('button', { name: /^restore /i }));
  await waitFor(() => expect(screen.queryByText('Ada Test')).toBeNull());
  const bob = screen.getByText('Bob Test').closest('li') as HTMLElement;
  await waitFor(() =>
    expect(document.activeElement).toBe(within(bob).getByRole('button', { name: /^restore /i })),
  );

  fireEvent.click(within(bob).getByRole('button', { name: /^restore /i }));
  await waitFor(() => expect(screen.queryByText('Bob Test')).toBeNull());
  await waitFor(() =>
    expect(document.activeElement).toBe(screen.getByRole('heading', { name: 'Trash' })),
  );
});

test('deleting a contact forever moves focus to the next row', async () => {
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;
  fireEvent.click(within(ada).getByRole('button', { name: /^delete forever: /i }));
  const dialog = await screen.findByRole('dialog', { name: 'Delete forever?' });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete forever' }));
  await waitFor(() => expect(screen.queryByText('Ada Test')).toBeNull());
  const bob = screen.getByText('Bob Test').closest('li') as HTMLElement;
  await waitFor(() =>
    expect(document.activeElement).toBe(
      within(bob).getByRole('button', { name: /^delete forever: /i }),
    ),
  );
});

test('a failed restore does not move focus later', async () => {
  restoreStatus = 409;
  renderPage();
  const ada = (await screen.findByText('Ada Test')).closest('li') as HTMLElement;
  const restore = within(ada).getByRole('button', { name: /^restore /i });
  restore.focus();
  fireEvent.click(restore);
  await screen.findByText(/already uses this contact/i);
  expect(document.activeElement).toBe(restore);
});
