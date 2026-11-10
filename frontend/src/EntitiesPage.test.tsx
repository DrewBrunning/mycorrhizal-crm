import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import {
  dismissEntitySuggestion,
  type Entity,
  type EntityKind,
  getEntity,
  listEntities,
  listEntitySuggestions,
  mergeEntity,
} from './api/entities';
import { SnackbarProvider } from './context/SnackbarContext';
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
  };
}

function renderPage(path = '/entities') {
  return render(
    <SnackbarProvider>
      <MemoryRouter initialEntries={[path]}>
        <EntitiesPage />
      </MemoryRouter>
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
        a: { id: 'o1', kind: 'organisation', name: 'Acme' },
        b: { id: 'o2', kind: 'organisation', name: 'Acme, Inc' },
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

test('"Not a duplicate" dismisses the pair for good', async () => {
  suggestMock.mockResolvedValueOnce({
    mode: 'pairs',
    pairs: [
      {
        a: { id: 'o1', kind: 'organisation', name: 'Acme' },
        b: { id: 'o2', kind: 'organisation', name: 'Acme, Inc' },
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
