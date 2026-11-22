import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { createMemoryRouter, MemoryRouter, RouterProvider } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import {
  createEntity,
  dismissEntitySuggestion,
  type Entity,
  type EntityKind,
  getEntity,
  listEntities,
  listEntitySuggestions,
  mergeEntity,
} from './api/entities';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import EntitiesPage from './EntitiesPage';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/entities')>();
  return {
    ...actual,
    listEntities: vi.fn(),
    listEntitySuggestions: vi.fn(),
    dismissEntitySuggestion: vi.fn(),
    getEntity: vi.fn(),
    mergeEntity: vi.fn(),
    createEntity: vi.fn(),
  };
});
const listMock = vi.mocked(listEntities);
const suggestMock = vi.mocked(listEntitySuggestions);
const dismissMock = vi.mocked(dismissEntitySuggestion);
const getMock = vi.mocked(getEntity);
const mergeMock = vi.mocked(mergeEntity);
const createMock = vi.mocked(createEntity);

function ent(id: string, name: string, kind: EntityKind = 'organisation', core = {}): Entity {
  return {
    id,
    kind,
    name,
    core,
    sensitivity: 'normal',
    revision: 1,
    created_at: '',
    updated_at: '',
    link_count: 0,
  };
}

function side(id: string, name: string, link_count = 0, updated_at = '2026-01-01T10:00:00Z') {
  return { id, kind: 'organisation' as const, name, updated_at, link_count };
}

function renderPage(path = '/entities') {
  return render(
    <SnackbarProvider>
      <DateFormatProvider>
        <MemoryRouter initialEntries={[path]}>
          <EntitiesPage />
        </MemoryRouter>
      </DateFormatProvider>
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  listMock.mockImplementation(async ({ kind }) => ({
    entities:
      kind === 'place'
        ? [
            ent('p1', 'Cabin', 'place', {
              address: { components: [{ kind: 'locality', value: 'Bern' }] },
            }),
          ]
        : [ent('o1', 'Acme', 'organisation', { url: 'https://acme.test' })],
    limit: 50,
    next_cursor: '',
  }));
  suggestMock.mockResolvedValue({
    mode: 'pairs',
    pairs: [],
    groups: [],
    limit: 25,
    next_cursor: '',
  });
});

test('lists organisations by default with a link to each entity and its summary', async () => {
  renderPage();
  expect(
    screen.getByRole('heading', { level: 1, name: 'Organisations, places & pets' }),
  ).toBeInTheDocument();
  const link = await screen.findByRole('link', { name: /Acme/ });
  expect(link).toHaveAttribute('href', '/entities/o1');
  expect(screen.getByText('https://acme.test')).toBeInTheDocument();
  expect(listMock).toHaveBeenCalledWith(expect.objectContaining({ kind: 'organisation' }));
});

test('kind tabs switch the list and reflect the kind in the URL', async () => {
  renderPage();
  await screen.findByText('Acme');
  fireEvent.click(screen.getByRole('tab', { name: 'Places' }));
  expect(await screen.findByText('Cabin')).toBeInTheDocument();
  expect(screen.getByText('Bern')).toBeInTheDocument();
  expect(screen.queryByText('Acme')).not.toBeInTheDocument();
  expect(screen.getByRole('tab', { name: 'Places' })).toHaveAttribute('aria-selected', 'true');
});

test('opens on the kind named in ?kind=', async () => {
  renderPage('/entities?kind=pet');
  await waitFor(() =>
    expect(listMock).toHaveBeenCalledWith(expect.objectContaining({ kind: 'pet' })),
  );
  expect(screen.getByRole('tab', { name: 'Pets' })).toHaveAttribute('aria-selected', 'true');
});

test('searching sends the query for the current kind', async () => {
  renderPage();
  await screen.findByText('Acme');
  fireEvent.change(screen.getByLabelText('Search by name'), { target: { value: 'ac' } });
  await waitFor(() =>
    expect(listMock).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'organisation', q: 'ac' }),
    ),
  );
});

test('shows a per-kind empty state and a retryable error', async () => {
  listMock.mockResolvedValueOnce({ entities: [], limit: 50, next_cursor: '' });
  renderPage();
  expect(await screen.findByText(/No organisations yet/)).toBeInTheDocument();

  cleanup();
  listMock.mockRejectedValueOnce(new Error('boom'));
  renderPage();
  const alert = await screen.findByRole('alert');
  expect(alert).toBeInTheDocument();
  listMock.mockResolvedValueOnce({ entities: [ent('o1', 'Acme')], limit: 50, next_cursor: '' });
  fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
  expect(await screen.findByText('Acme')).toBeInTheDocument();
});

test('the add button opens the create form for the current kind', async () => {
  renderPage('/entities?kind=place');
  await screen.findByText('Cabin');
  fireEvent.click(screen.getByRole('button', { name: 'Add place' }));
  expect(await screen.findByRole('dialog', { name: 'Add place' })).toBeInTheDocument();
});

test('duplicate suggestions: find, then merge via the dialog', async () => {
  suggestMock.mockResolvedValueOnce({
    mode: 'pairs',
    pairs: [
      {
        a: side('o1', 'Acme'),
        b: side('o2', 'Acme, Inc'),
        reason: 'normalized_name',
      },
    ],
    groups: [],
    limit: 25,
    next_cursor: '',
  });
  getMock.mockImplementation(async (id) => ent(id, id === 'o1' ? 'Acme' : 'Acme, Inc'));
  mergeMock.mockResolvedValue({
    survivor: ent('o1', 'Acme'),
    loser_id: 'o2',
    links_repointed: 2,
    links_collapsed: 0,
    primaries_demoted: [],
    field_values_moved: 0,
    field_values_dropped: [],
    resolved_fields: [],
    restore_scope: 'entity_only',
  });
  renderPage();
  await screen.findByText('Acme');
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('Very similar name')).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Merge…' }));
  await screen.findByText(/will be deleted/);
  fireEvent.click(within(screen.getByRole('dialog')).getByRole('button', { name: 'Merge' }));
  await waitFor(() => expect(mergeMock).toHaveBeenCalledWith('o2', { into: 'o1' }));
  // The merged pair disappears and the list refreshes.
  await waitFor(() => expect(screen.queryByText('Very similar name')).not.toBeInTheDocument());
});

// #2743: the merge dialog opened from a same-named pair describes each side with the pair's
// distinct-contact count, not the detail endpoint's link-row count (decoy 99).
test('merging a same-named pair describes each side with the pair counts (#2743)', async () => {
  suggestMock.mockResolvedValueOnce({
    mode: 'pairs',
    pairs: [
      {
        a: side('g1', 'Green Acres Co-op', 2, '2026-03-05T10:00:00Z'),
        b: side('g2', 'Green Acres Co-op', 1, '2026-04-19T10:00:00Z'),
        reason: 'exact_name',
      },
    ],
    groups: [],
    limit: 25,
    next_cursor: '',
  });
  getMock.mockImplementation(async (id) => ({
    ...ent(id, 'Green Acres Co-op'),
    link_count: 99,
    updated_at: id === 'g1' ? '2026-03-05T10:00:00Z' : '2026-04-19T10:00:00Z',
  }));
  renderPage();
  fireEvent.click(await screen.findByRole('button', { name: 'Find duplicates' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Merge…' }));
  const intro = await screen.findByText(/will be deleted and its contacts move to/);
  expect(intro.textContent).toMatch(
    /\(updated .+, 1 contact\) will be deleted.*\(updated .+, 2 contacts\)\./,
  );
  expect(intro.textContent).not.toMatch(/99/);
});

test('"Not a duplicate" dismisses the pair for good', async () => {
  suggestMock.mockResolvedValueOnce({
    mode: 'pairs',
    pairs: [
      {
        a: side('o1', 'Acme'),
        b: side('o2', 'Acme, Inc'),
        reason: 'exact_name',
      },
    ],
    groups: [],
    limit: 25,
    next_cursor: '',
  });
  dismissMock.mockResolvedValue({ id_low: 'o1', id_high: 'o2' });
  renderPage();
  await screen.findByText('Acme');
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  fireEvent.click(await screen.findByRole('button', { name: 'Not a duplicate' }));
  await waitFor(() => expect(dismissMock).toHaveBeenCalledWith('o1', 'o2'));
  await waitFor(() => expect(screen.queryByText('Same name')).not.toBeInTheDocument());
});

test('an empty suggestion result says so', async () => {
  renderPage();
  await screen.findByText('Acme');
  fireEvent.click(screen.getByRole('button', { name: 'Find duplicates' }));
  expect(await screen.findByText('No duplicates found.')).toBeInTheDocument();
});

// #2693: the create dialog arms a discard guard; without allowNavigation() the
// blocker swallowed the navigate() to the new entity, and the list stayed stale.
test('saving a new entity opens it and reloads the list (#2693)', async () => {
  createMock.mockResolvedValue(ent('new1', 'Rex', 'pet'));
  const router = createMemoryRouter(
    [
      { path: '/entities', element: <EntitiesPage /> },
      { path: '/entities/:id', element: <div>entity detail page</div> },
    ],
    { initialEntries: ['/entities?kind=pet'] },
  );
  render(
    <SnackbarProvider>
      <DateFormatProvider>
        <RouterProvider router={router} />
      </DateFormatProvider>
    </SnackbarProvider>,
  );
  await waitFor(() => expect(listMock).toHaveBeenCalledTimes(1));
  fireEvent.click(await screen.findByRole('button', { name: 'Add pet' }));
  const dialog = await screen.findByRole('dialog', { name: 'Add pet' });
  fireEvent.change(within(dialog).getByLabelText('Name *'), { target: { value: 'Rex' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));

  expect(await screen.findByText('entity detail page')).toBeInTheDocument();
  expect(router.state.location.pathname).toBe('/entities/new1');
  expect(screen.queryByText('Discard unsaved changes?')).toBeNull();
  // The list was reloaded so it is not stale when the user comes back.
  expect(listMock.mock.calls.length).toBeGreaterThanOrEqual(2);
});

// #2699: rows must be <ul><li><a>, not <a> directly inside the <ul>.
test('each entity row is a list item containing its link (#2699)', async () => {
  listMock.mockResolvedValue({
    entities: [ent('o1', 'Acme'), ent('o2', 'Globex')],
    limit: 50,
    next_cursor: '',
  });
  renderPage();
  const list = await screen.findByRole('list', { name: 'Organisations' });
  const items = within(list).getAllByRole('listitem');
  expect(items).toHaveLength(2);
  for (const li of items) expect(within(li).getByRole('link')).toBeInTheDocument();
  expect(within(items[0]).getByRole('link', { name: /Acme/ })).toHaveAttribute(
    'href',
    '/entities/o1',
  );
});

// #2701/#2743: same-named entities need a distinguishing detail in the list; the list
// rows carry link_count (distinct contacts) and updated_at, so there is no created-date fallback.
test('same-named entities show linked-contact count and updated date so they can be told apart (#2743)', async () => {
  listMock.mockResolvedValue({
    entities: [
      { ...ent('g1', 'Green Acres Co-op'), updated_at: '2026-03-05T10:00:00Z', link_count: 3 },
      { ...ent('g2', 'Green Acres Co-op'), updated_at: '2026-04-19T10:00:00Z', link_count: 1 },
    ],
    limit: 50,
    next_cursor: '',
  });
  renderPage();
  const links = await screen.findAllByRole('link', { name: /Green Acres Co-op/ });
  expect(links).toHaveLength(2);
  expect(links[0].textContent).toMatch(/3 contacts · updated /i);
  expect(links[1].textContent).toMatch(/1 contact · updated /i);
  expect(links[0].textContent).not.toMatch(/Added /);
  expect(getMock).not.toHaveBeenCalled();
});

test('a zero link count is shown, not hidden (#2743)', async () => {
  listMock.mockResolvedValue({
    entities: [{ ...ent('z1', 'Lonely Org'), updated_at: '2026-03-05T10:00:00Z', link_count: 0 }],
    limit: 50,
    next_cursor: '',
  });
  renderPage();
  const link = await screen.findByRole('link', { name: /Lonely Org/ });
  expect(link.textContent).toMatch(/0 contacts · updated /i);
});
