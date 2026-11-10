import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import {
  type Entity,
  type EntityMergeResponse,
  getEntity,
  listEntities,
  mergeEntity,
} from '../api/entities';
import EntityMergeDialog from './EntityMergeDialog';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, getEntity: vi.fn(), listEntities: vi.fn(), mergeEntity: vi.fn() };
});
const getMock = vi.mocked(getEntity);
const listMock = vi.mocked(listEntities);
const mergeMock = vi.mocked(mergeEntity);

function ent(id: string, name: string, core: Record<string, unknown> = {}): Entity {
  return {
    id,
    kind: 'organisation',
    name,
    core,
    sensitivity: 'normal',
    revision: 1,
    created_at: '',
    updated_at: '',
  };
}
const acme = ent('a', 'Acme', {});
const acmeInc = ent('b', 'Acme, Inc', { url: 'https://acme.test', notes: 'Big' });

function result(overrides: Partial<EntityMergeResponse> = {}): EntityMergeResponse {
  return {
    survivor: acme,
    loser_id: 'b',
    links_repointed: 3,
    links_collapsed: 0,
    primaries_demoted: [],
    field_values_moved: 0,
    field_values_dropped: [],
    resolved_fields: [],
    restore_scope: 'entity_only',
    ...overrides,
  };
}

beforeEach(() => {
  getMock.mockImplementation(async (id) => (id === 'a' ? acme : acmeInc));
  mergeMock.mockResolvedValue(result());
  listMock.mockResolvedValue({ entities: [acme, acmeInc], limit: 25, next_cursor: '' });
});

test('merges the loser into the survivor with explicit field resolutions only', async () => {
  const onMerged = vi.fn();
  render(
    <EntityMergeDialog open loserId="b" survivorId="a" onClose={vi.fn()} onMerged={onMerged} />,
  );
  expect(
    await screen.findByText(/Acme, Inc will be deleted and its contacts move to Acme/),
  ).toBeInTheDocument();

  // Nothing is filled unless the user ticks it (ADR 0043: no silent union).
  const url = screen.getByRole('checkbox', { name: 'Website' });
  expect(url).not.toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Notes' })).toBeInTheDocument();
  fireEvent.click(url);
  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));

  await waitFor(() => expect(onMerged).toHaveBeenCalled());
  expect(mergeMock).toHaveBeenCalledWith('b', { into: 'a', resolutions: { url: 'loser' } });
  expect(onMerged.mock.calls[0][0].survivor.name).toBe('Acme');
});

test('with nothing ticked the request carries no resolutions', async () => {
  render(
    <EntityMergeDialog open loserId="b" survivorId="a" onClose={vi.fn()} onMerged={vi.fn()} />,
  );
  await screen.findByText(/will be deleted/);
  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));
  await waitFor(() => expect(mergeMock).toHaveBeenCalled());
  expect(mergeMock).toHaveBeenCalledWith('b', { into: 'a' });
});

test('swap flips which entity is kept', async () => {
  render(
    <EntityMergeDialog open loserId="b" survivorId="a" onClose={vi.fn()} onMerged={vi.fn()} />,
  );
  await screen.findByText(/Acme, Inc will be deleted/);
  fireEvent.click(screen.getByRole('button', { name: 'Swap which one is kept' }));
  expect(
    await screen.findByText(/Acme will be deleted and its contacts move to Acme, Inc/),
  ).toBeInTheDocument();
  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));
  await waitFor(() => expect(mergeMock).toHaveBeenCalledWith('a', { into: 'b' }));
});

test('opened from an entity page, the user picks the survivor from same-kind entities', async () => {
  render(<EntityMergeDialog open loserId="b" onClose={vi.fn()} onMerged={vi.fn()} />);
  const mergeButton = await screen.findByRole('button', { name: 'Merge' });
  expect(mergeButton).toBeDisabled();

  fireEvent.mouseDown(screen.getByLabelText('Merge into'));
  const list = await screen.findByRole('listbox');
  // The entity being merged is never offered as its own survivor.
  expect(within(list).queryByText('Acme, Inc')).not.toBeInTheDocument();
  fireEvent.click(within(list).getByText('Acme'));
  await waitFor(() => expect(screen.getByRole('button', { name: 'Merge' })).toBeEnabled());
  expect(listMock).toHaveBeenCalledWith(expect.objectContaining({ kind: 'organisation' }));
});

test('a merge error is shown and onMerged is not called', async () => {
  mergeMock.mockRejectedValueOnce(
    new ApiError('Only entities of the same kind can be merged', 'VALIDATION_ERROR', 422),
  );
  const onMerged = vi.fn();
  render(
    <EntityMergeDialog open loserId="b" survivorId="a" onClose={vi.fn()} onMerged={onMerged} />,
  );
  await screen.findByText(/will be deleted/);
  fireEvent.click(screen.getByRole('button', { name: 'Merge' }));
  expect(await screen.findByRole('alert')).toHaveTextContent('same kind');
  expect(onMerged).not.toHaveBeenCalled();
});
