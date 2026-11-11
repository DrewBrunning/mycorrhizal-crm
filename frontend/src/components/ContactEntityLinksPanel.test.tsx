import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import { type ContactEntityLink, createEntity, type Entity, listEntities } from '../api/entities';
import { SnackbarProvider } from '../context/SnackbarContext';
import ContactEntityLinksPanel, { type ContactEntityLinksApi } from './ContactEntityLinksPanel';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntities: vi.fn(), createEntity: vi.fn() };
});
const listMock = vi.mocked(listEntities);
const createMock = vi.mocked(createEntity);

function link(overrides: Partial<ContactEntityLink> = {}): ContactEntityLink {
  return {
    id: 'l1',
    contact_vcard_uid: 'u',
    entity_id: 'e1',
    entity_name: 'Acme',
    kind: 'organisation',
    role: 'employee',
    role_label: '',
    is_primary: false,
    sensitivity: 'normal',
    current: true,
    projects_as: '',
    revision: 1,
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

function ent(id: string, name: string, kind: Entity['kind'] = 'organisation'): Entity {
  return {
    id,
    kind,
    name,
    core: {},
    sensitivity: 'normal',
    revision: 1,
    created_at: '',
    updated_at: '',
  };
}

function makeApi(links: ContactEntityLink[]): ContactEntityLinksApi {
  return {
    links,
    loading: false,
    error: null,
    refresh: vi.fn().mockResolvedValue(undefined),
    save: vi.fn().mockResolvedValue(link()),
    setPrimary: vi.fn().mockResolvedValue(undefined),
    remove: vi.fn().mockResolvedValue(undefined),
    adopt: vi.fn().mockResolvedValue({ adopted: true }),
    linkFromExisting: vi.fn().mockResolvedValue({
      entity: ent('e9', 'Globex'),
      entity_created: true,
      link: link(),
    }),
  } as unknown as ContactEntityLinksApi;
}

function renderPanel(
  api: ContactEntityLinksApi,
  own: {
    organizations?: { id?: string; name?: string }[];
    addresses?: { id?: string; components?: { kind: string; value: string }[] }[];
  } = {},
) {
  return render(
    <SnackbarProvider>
      <MemoryRouter>
        <ContactEntityLinksPanel
          api={api}
          ownOrganizations={own.organizations ?? []}
          ownAddresses={own.addresses ?? []}
        />
      </MemoryRouter>
    </SnackbarProvider>,
  );
}

beforeEach(() => {
  listMock.mockResolvedValue({
    entities: [ent('e1', 'Acme'), ent('e2', 'Globex')],
    limit: 20,
    next_cursor: '',
  });
});

test('empty state', () => {
  renderPanel(makeApi([]));
  expect(screen.getByText('Not linked to any organisation, place or pet yet.')).toBeInTheDocument();
});

test('a row shows entity link, role, period and what it projects as', () => {
  renderPanel(
    makeApi([
      link({
        is_primary: true,
        projects_as: 'ORG',
        period: { start: { year: 2019 } },
        role_label: 'CTO',
      }),
      link({
        id: 'l2',
        entity_id: 'e2',
        entity_name: 'Old Co',
        is_primary: true,
        current: false,
        period: { end: { year: 2001 } },
      }),
      link({ id: 'l3', entity_id: 'e3', entity_name: 'Rex', kind: 'pet', role: 'owner' }),
    ]),
  );
  const acme = screen.getByRole('link', { name: 'Acme' });
  expect(acme).toHaveAttribute('href', '/entities/e1');
  expect(screen.getAllByText('Employee')).toHaveLength(2);
  expect(screen.getByText('CTO')).toBeInTheDocument();
  expect(screen.getByText('Since 2019')).toBeInTheDocument();
  expect(screen.getByText('Shown as organisation')).toBeInTheDocument();
  // A primary link outside its dates is flagged instead of silently doing nothing.
  expect(screen.getByText('Not current')).toBeInTheDocument();
  // Pets never project: no primary switch is offered for them.
  expect(screen.queryByRole('switch', { name: /Primary: Rex/ })).not.toBeInTheDocument();
});

test('toggling primary, copying and removing call the matching handlers', async () => {
  const api = makeApi([link({ projects_as: '' })]);
  renderPanel(api);
  fireEvent.click(screen.getByRole('switch', { name: 'Primary: Acme' }));
  expect(api.setPrimary).toHaveBeenCalledWith(api.links[0], true);

  fireEvent.click(screen.getByRole('button', { name: 'Copy Acme as own organisation' }));
  await waitFor(() => expect(api.adopt).toHaveBeenCalledWith(api.links[0]));

  vi.spyOn(window, 'confirm').mockReturnValue(true);
  fireEvent.click(screen.getByRole('button', { name: 'Remove link to Acme' }));
  await waitFor(() => expect(api.remove).toHaveBeenCalledWith(api.links[0]));
});

test('declining the remove confirmation keeps the link', () => {
  const api = makeApi([link()]);
  vi.spyOn(window, 'confirm').mockReturnValue(false);
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Remove link to Acme' }));
  expect(api.remove).not.toHaveBeenCalled();
});

test('add: pick an existing organisation, role and primary, then save', async () => {
  const api = makeApi([]);
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));
  const dialog = await screen.findByRole('dialog', {
    name: 'Link to an organisation, place or pet',
  });
  fireEvent.mouseDown(within(dialog).getByLabelText('Organisation, place or pet *'));
  fireEvent.click(await screen.findByRole('option', { name: 'Globex' }));
  fireEvent.mouseDown(within(dialog).getByLabelText('Role'));
  fireEvent.click(await screen.findByRole('option', { name: 'Contractor' }));
  fireEvent.click(within(dialog).getByRole('checkbox', { name: /Primary/ }));
  fireEvent.change(within(dialog).getByLabelText('From (year)'), { target: { value: '2020' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.save).toHaveBeenCalled());
  expect(api.save).toHaveBeenCalledWith(
    {
      entity_id: 'e2',
      role: 'contractor',
      role_label: '',
      is_primary: true,
      period: { start: { year: 2020 } },
      sensitivity: 'normal',
    },
    null,
  );
});

test('add: typing a new name offers to create the entity first', async () => {
  createMock.mockResolvedValue(ent('e7', 'Initech'));
  const api = makeApi([]);
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));
  const dialog = await screen.findByRole('dialog');
  const input = within(dialog).getByLabelText('Organisation, place or pet *');
  fireEvent.change(input, { target: { value: 'Initech' } });
  fireEvent.click(await screen.findByRole('option', { name: 'Create "Initech"' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.save).toHaveBeenCalled());
  expect(createMock).toHaveBeenCalledWith(
    expect.objectContaining({ kind: 'organisation', name: 'Initech' }),
  );
  expect(vi.mocked(api.save).mock.calls[0][0]).toMatchObject({ entity_id: 'e7', role: 'employee' });
});

test('a bad year is rejected without a request', async () => {
  const api = makeApi([]);
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Link' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.mouseDown(within(dialog).getByLabelText('Organisation, place or pet *'));
  fireEvent.click(await screen.findByRole('option', { name: 'Acme' }));
  fireEvent.change(within(dialog).getByLabelText('From (year)'), { target: { value: '20x' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('four digits');
  expect(api.save).not.toHaveBeenCalled();
});

test('edit prefills the link and saves against it', async () => {
  const existing = link({ role_label: 'CTO', period: { start: { year: 2019 } } });
  const api = makeApi([existing]);
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Edit link to Acme' }));
  const dialog = await screen.findByRole('dialog', { name: 'Edit link to Acme' });
  expect(within(dialog).getByLabelText('Role detail (optional)')).toHaveValue('CTO');
  expect(within(dialog).getByLabelText('From (year)')).toHaveValue('2019');
  fireEvent.change(within(dialog).getByLabelText('To (year)'), { target: { value: '2024' } });
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  await waitFor(() => expect(api.save).toHaveBeenCalled());
  expect(vi.mocked(api.save).mock.calls[0][0]).toMatchObject({
    entity_id: 'e1',
    period: { start: { year: 2019 }, end: { year: 2024 } },
  });
  expect(vi.mocked(api.save).mock.calls[0][1]).toBe(existing);
});

test('a save error stays in the dialog', async () => {
  const api = makeApi([link()]);
  vi.mocked(api.save).mockRejectedValue(new ApiError('Role is not valid', 'VALIDATION_ERROR', 422));
  renderPanel(api);
  fireEvent.click(screen.getByRole('button', { name: 'Edit link to Acme' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('Role is not valid');
});

test('link from an own organisation: reuse a matching entity', async () => {
  listMock.mockResolvedValue({ entities: [ent('e1', 'Acme')], limit: 20, next_cursor: '' });
  const api = makeApi([]);
  renderPanel(api, { organizations: [{ id: 'o1', name: 'Acme' }] });
  fireEvent.click(screen.getByRole('button', { name: 'Link one of my own values' }));
  const dialog = await screen.findByRole('dialog', { name: 'Link one of my own values' });
  fireEvent.click(
    await within(dialog).findByRole('radio', { name: 'Use the existing organisation “Acme”' }),
  );
  fireEvent.click(within(dialog).getByRole('button', { name: 'Link' }));
  await waitFor(() => expect(api.linkFromExisting).toHaveBeenCalled());
  expect(api.linkFromExisting).toHaveBeenCalledWith({
    entity_id: 'e1',
    role: 'employee',
    source: { kind: 'organization', id: 'o1' },
  });
});

test('link from an own organisation: create a new entity by default', async () => {
  listMock.mockResolvedValue({ entities: [], limit: 20, next_cursor: '' });
  const api = makeApi([]);
  renderPanel(api, { organizations: [{ name: 'Globex' }, { name: 'Acme' }] });
  fireEvent.click(screen.getByRole('button', { name: 'Link one of my own values' }));
  const dialog = await screen.findByRole('dialog');
  fireEvent.mouseDown(within(dialog).getByLabelText('Value'));
  fireEvent.click(await screen.findByRole('option', { name: 'Acme' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Link' }));
  await waitFor(() => expect(api.linkFromExisting).toHaveBeenCalled());
  // No stable id on the value: it is addressed by position, and no entity_id means "create".
  expect(api.linkFromExisting).toHaveBeenCalledWith({
    role: 'employee',
    source: { kind: 'organization', index: 1 },
  });
});

test('link from own value is disabled when the contact has none', () => {
  renderPanel(makeApi([]));
  expect(screen.getByRole('button', { name: 'Link one of my own values' })).toBeDisabled();
});
