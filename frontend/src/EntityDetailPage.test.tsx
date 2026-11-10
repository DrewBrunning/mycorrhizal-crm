import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import { ApiError } from './api/client';
import {
  adoptContactEntityLink,
  deleteEntity,
  type Entity,
  type EntityContactLink,
  getEntity,
  listEntities,
  listEntityContacts,
  listEntityFieldValues,
  mergeEntity,
  replaceEntityFieldValues,
  restoreEntity,
  updateEntity,
} from './api/entities';
import { type FieldDefinition, getFieldDefinitions } from './api/fieldDefinitions';
import { SnackbarProvider } from './context/SnackbarContext';
import { DateFormatProvider } from './DateFormatProvider';
import EntityDetailPage from './EntityDetailPage';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/entities')>();
  return {
    ...actual,
    getEntity: vi.fn(),
    updateEntity: vi.fn(),
    deleteEntity: vi.fn(),
    restoreEntity: vi.fn(),
    mergeEntity: vi.fn(),
    listEntities: vi.fn(),
    listEntityContacts: vi.fn(),
    listEntityFieldValues: vi.fn(),
    replaceEntityFieldValues: vi.fn(),
    adoptContactEntityLink: vi.fn(),
  };
});
vi.mock('./api/fieldDefinitions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/fieldDefinitions')>();
  return { ...actual, getFieldDefinitions: vi.fn() };
});

const getMock = vi.mocked(getEntity);
const updateMock = vi.mocked(updateEntity);
const deleteMock = vi.mocked(deleteEntity);
const restoreMock = vi.mocked(restoreEntity);
const mergeMock = vi.mocked(mergeEntity);
const listMock = vi.mocked(listEntities);
const contactsMock = vi.mocked(listEntityContacts);
const valuesMock = vi.mocked(listEntityFieldValues);
const replaceMock = vi.mocked(replaceEntityFieldValues);
const adoptMock = vi.mocked(adoptContactEntityLink);
const defsMock = vi.mocked(getFieldDefinitions);

const acme: Entity = {
  id: 'e1',
  kind: 'organisation',
  name: 'Acme',
  core: { organization: { name: 'Acme' }, url: 'https://acme.test', notes: 'Big client' },
  sensitivity: 'normal',
  revision: 5,
  link_count: 3,
  projection_count: 2,
  created_at: '',
  updated_at: '',
};

function link(overrides: Partial<EntityContactLink> = {}): EntityContactLink {
  return {
    link_id: 'l1',
    contact_id: 11,
    contact_vcard_uid: 'u11',
    firstname: 'Alice',
    lastname: 'Anderson',
    role: 'employee',
    role_label: '',
    is_primary: true,
    sensitivity: 'normal',
    ...overrides,
  };
}

function renderPage(state?: unknown) {
  return render(
    <SnackbarProvider>
      <DateFormatProvider>
        <MemoryRouter initialEntries={[{ pathname: '/entities/e1', state }]}>
          <Routes>
            <Route path="/entities/:id" element={<EntityDetailPage />} />
            <Route path="/entities" element={<div>entity list</div>} />
            <Route path="/contacts/:id" element={<div>contact page</div>} />
          </Routes>
        </MemoryRouter>
      </DateFormatProvider>
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  getMock.mockResolvedValue(acme);
  contactsMock.mockResolvedValue([
    link(),
    link({
      link_id: 'l2',
      contact_id: 12,
      contact_vcard_uid: 'u12',
      firstname: 'Bob',
      lastname: 'Brown',
      role: 'owner',
      is_primary: false,
      period: { start: { year: 2019 }, end: { year: 2021 } },
    }),
  ]);
  valuesMock.mockResolvedValue([]);
  defsMock.mockResolvedValue({ field_definitions: [], total: 0, next_cursor: '', limit: 100 });
  listMock.mockResolvedValue({ entities: [], limit: 100, next_cursor: '' });
});

test('shows the entity, its details and the linked people with role, period and primary', async () => {
  renderPage();
  expect(await screen.findByRole('heading', { level: 1, name: 'Acme' })).toBeInTheDocument();
  expect(screen.getByText('https://acme.test')).toBeInTheDocument();
  expect(screen.getByText('Big client')).toBeInTheDocument();
  expect(screen.getByText('Linked to 3 contacts')).toBeInTheDocument();

  const people = screen.getByRole('region', { name: 'People' });
  const alice = within(people).getByRole('link', { name: 'Alice Anderson' });
  expect(alice).toHaveAttribute('href', '/contacts/11');
  expect(within(people).getByText('Employee')).toBeInTheDocument();
  expect(within(people).getByText('Primary')).toBeInTheDocument();
  expect(within(people).getByText('Owner')).toBeInTheDocument();
  expect(within(people).getByText('2019 – 2021')).toBeInTheDocument();
});

test('shows a not-found state when the entity is gone', async () => {
  getMock.mockRejectedValueOnce(new ApiError('Entity not found', 'NOT_FOUND', 404));
  renderPage();
  expect(await screen.findByText("This entity doesn't exist or was deleted.")).toBeInTheDocument();
});

test('edit saves with the current revision and refreshes the page', async () => {
  updateMock.mockResolvedValue({ ...acme, name: 'Acme Ltd', revision: 6 });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'Acme Ltd' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));
  expect(await screen.findByRole('heading', { name: 'Acme Ltd' })).toBeInTheDocument();
  expect(updateMock).toHaveBeenCalledWith('e1', expect.objectContaining({ name: 'Acme Ltd' }), 5);
});

test('custom fields for this kind are editable and saved as the full value set', async () => {
  const def = (id: string, label: string, target: FieldDefinition['target']): FieldDefinition => ({
    id,
    label,
    key: label.toLowerCase(),
    target,
    type: 'string',
    projection: 'internal-only',
    sensitivity: 'normal',
    position: 0,
    group: '',
    created_at: '',
    updated_at: '',
  });
  defsMock.mockResolvedValue({
    field_definitions: [
      def('d1', 'Account number', 'organisation'),
      def('d2', 'Favourite food', 'pet'),
      def('d3', 'Nickname', 'contact'),
    ],
    total: 3,
    next_cursor: '',
    limit: 100,
  });
  valuesMock.mockResolvedValue([
    {
      id: 1,
      field_definition_id: 'd1',
      entity_id: 'e1',
      value: 'A-1',
      created_at: '',
      updated_at: '',
    },
  ]);
  replaceMock.mockResolvedValue([]);
  renderPage();
  await screen.findByText('Account number');
  // Definitions of other targets never appear on this entity.
  expect(screen.queryByText('Favourite food')).not.toBeInTheDocument();
  expect(screen.queryByText('Nickname')).not.toBeInTheDocument();

  const region = screen.getByRole('region', { name: 'Custom fields' });
  fireEvent.click(within(region).getByLabelText('Edit'));
  fireEvent.change(within(region).getByRole('textbox'), { target: { value: 'B-2' } });
  fireEvent.click(within(region).getByRole('button', { name: 'Save' }));
  await waitFor(() =>
    expect(replaceMock).toHaveBeenCalledWith('e1', [{ field_definition_id: 'd1', value: 'B-2' }]),
  );
});

test('delete names how many contacts lose the organisation and deletes with the revision', async () => {
  deleteMock.mockResolvedValue({ message: 'ok', link_count: 3, projection_count: 2 });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  const dialog = await screen.findByRole('dialog', { name: 'Delete Acme?' });
  expect(
    within(dialog).getByText(
      'Acme is the primary organisation of 2 contacts; they will lose that organisation.',
    ),
  ).toBeInTheDocument();
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(deleteMock).toHaveBeenCalledWith('e1', 5));
  expect(adoptMock).not.toHaveBeenCalled();
  // Deleted state with a restore action instead of silently navigating away.
  expect(await screen.findByText('Acme was deleted')).toBeInTheDocument();
});

test('adopt-all copies the organisation into each current primary contact before deleting', async () => {
  contactsMock.mockResolvedValue([
    link(),
    link({ link_id: 'l3', contact_id: 13, firstname: 'Cy', lastname: 'Cole' }),
    // Not primary, and a primary that ended long ago: neither projects, so neither is adopted.
    link({ link_id: 'l2', contact_id: 12, is_primary: false }),
    link({ link_id: 'l4', contact_id: 14, period: { end: { year: 2001 } } }),
  ]);
  adoptMock.mockResolvedValue({} as never);
  deleteMock.mockResolvedValue({ message: 'ok', link_count: 4, projection_count: 2 });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(
    within(dialog).getByRole('checkbox', { name: /Keep the organisation on those contacts first/ }),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  await waitFor(() => expect(deleteMock).toHaveBeenCalled());
  expect(adoptMock.mock.calls.map((c) => [c[0], c[1]])).toEqual([
    [11, 'l1'],
    [13, 'l3'],
  ]);
});

test('if an adopt fails nothing is deleted and the error is shown', async () => {
  adoptMock.mockRejectedValue(new ApiError('nope', 'ERR', 500));
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('checkbox', { name: /Keep the organisation/ }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Delete' }));
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('so nothing was deleted');
  expect(deleteMock).not.toHaveBeenCalled();
});

test('restore brings the deleted entity back', async () => {
  deleteMock.mockResolvedValue({ message: 'ok', link_count: 3, projection_count: 2 });
  restoreMock.mockResolvedValue({
    entity: acme,
    links_revived: 3,
    same_name_live: false,
    skipped: [],
  });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  fireEvent.click(
    within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }),
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Restore' }));
  await waitFor(() => expect(restoreMock).toHaveBeenCalledWith('e1'));
  expect(await screen.findByRole('heading', { name: 'Acme' })).toBeInTheDocument();
});

test('a restore that finds a live same-name entity warns about it', async () => {
  deleteMock.mockResolvedValue({ message: 'ok', link_count: 0, projection_count: 0 });
  restoreMock.mockResolvedValue({
    entity: acme,
    links_revived: 0,
    same_name_live: true,
    skipped: [],
  });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
  fireEvent.click(
    within(await screen.findByRole('dialog')).getByRole('button', { name: 'Delete' }),
  );
  fireEvent.click(await screen.findByRole('button', { name: 'Restore' }));
  expect(
    await screen.findByText(/Another entity of this kind with the same name already exists/),
  ).toBeInTheDocument();
});

test('merge into another entity navigates to the survivor and offers to restore the loser', async () => {
  const other: Entity = { ...acme, id: 'e2', name: 'Acme Inc', core: {} };
  listMock.mockResolvedValue({ entities: [acme, other], limit: 100, next_cursor: '' });
  getMock.mockImplementation(async (id) => (id === 'e2' ? other : acme));
  mergeMock.mockResolvedValue({
    survivor: other,
    loser_id: 'e1',
    links_repointed: 3,
    links_collapsed: 0,
    primaries_demoted: [],
    field_values_moved: 0,
    field_values_dropped: [],
    resolved_fields: [],
    restore_scope: 'entity_only',
  });
  renderPage();
  await screen.findByRole('heading', { name: 'Acme' });
  fireEvent.click(screen.getByRole('button', { name: 'Merge into another…' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.mouseDown(within(dialog).getByLabelText('Merge into'));
  fireEvent.click(await screen.findByRole('option', { name: 'Acme Inc' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Merge' }));
  await waitFor(() => expect(mergeMock).toHaveBeenCalledWith('e1', { into: 'e2' }));
  expect(await screen.findByText('Acme was merged away')).toBeInTheDocument();
  restoreMock.mockResolvedValue({
    entity: acme,
    links_revived: 0,
    same_name_live: false,
    skipped: [],
  });
  fireEvent.click(screen.getByRole('button', { name: 'Restore Acme' }));
  await waitFor(() => expect(restoreMock).toHaveBeenCalledWith('e1'));
});
