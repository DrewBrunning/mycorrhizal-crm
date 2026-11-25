import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import '../i18n/config';
import { ApiError } from '../api/client';
import {
  createEntitiesFromContacts,
  type EntityValueGroup,
  listEntitySuggestions,
} from '../api/entities';
import { SnackbarProvider } from '../context/SnackbarContext';
import EntitiesFromContactsSettings from './EntitiesFromContactsSettings';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntitySuggestions: vi.fn(), createEntitiesFromContacts: vi.fn() };
});
const suggestMock = vi.mocked(listEntitySuggestions);
const createMock = vi.mocked(createEntitiesFromContacts);

function group(overrides: Partial<EntityValueGroup> = {}): EntityValueGroup {
  return {
    key: 'acme',
    kind: 'organisation',
    name: 'Acme Corp',
    reason: 'normalized_name',
    variants: ['Acme Corp', 'Acme, Inc'],
    existing_entity_id: '',
    existing_entity_name: '',
    contacts: [
      {
        contact_id: 1,
        firstname: 'Ann',
        lastname: 'A',
        value_name: 'Acme Corp',
        has_primary: false,
      },
      { contact_id: 2, firstname: 'Bo', lastname: 'B', value_name: 'Acme, Inc', has_primary: true },
    ],
    ...overrides,
  };
}

function page(groups: EntityValueGroup[], next = '') {
  return { mode: 'contact_values' as const, pairs: [], groups, limit: 25, next_cursor: next };
}

function renderCard() {
  return render(
    <SnackbarProvider>
      <MemoryRouter>
        <EntitiesFromContactsSettings />
      </MemoryRouter>
    </SnackbarProvider>,
  );
}

async function openWizard() {
  fireEvent.click(
    screen.getByRole('button', { name: 'Create organisations and places from my contacts…' }),
  );
  return screen.findByRole('dialog', { name: 'Create organisations and places from my contacts' });
}

beforeEach(() => {
  suggestMock.mockResolvedValue(page([group()]));
  createMock.mockResolvedValue({ created: [], skipped: [] });
});

test('does nothing until the user opens the wizard (no background scan)', () => {
  renderCard();
  expect(screen.getByText(/never changes or deletes a value on a contact/)).toBeInTheDocument();
  expect(suggestMock).not.toHaveBeenCalled();
});

test('lists groups of the contacts’ own organisations with counts, variants and a primary hint', async () => {
  renderCard();
  const dialog = await openWizard();
  expect(suggestMock).toHaveBeenCalledWith(
    expect.objectContaining({ kind: 'organisation', mode: 'contact_values' }),
  );
  expect(await within(dialog).findByText('Acme Corp — 2 contacts')).toBeInTheDocument();
  expect(within(dialog).getByText('Acme, Inc')).toBeInTheDocument();
  expect(
    within(dialog).getByText('1 of them already has a primary organisation.'),
  ).toBeInTheDocument();
});

test('the kind switch loads the places helper', async () => {
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Acme Corp — 2 contacts');
  suggestMock.mockResolvedValueOnce(page([]));
  fireEvent.click(within(dialog).getByRole('tab', { name: 'Places' }));
  await waitFor(() =>
    expect(suggestMock).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'place' })),
  );
  expect(
    await within(dialog).findByText('Nothing left to create from your contacts.'),
  ).toBeInTheDocument();
});

test('creating sends only ticked groups, with their edited name, role and primary choice', async () => {
  suggestMock.mockResolvedValue(
    page([
      group(),
      group({
        key: 'globex',
        name: 'Globex',
        variants: ['Globex'],
        contacts: [
          {
            contact_id: 3,
            firstname: 'Cy',
            lastname: 'C',
            value_name: 'Globex',
            has_primary: false,
          },
        ],
      }),
    ]),
  );
  createMock.mockResolvedValue({
    created: [{ key: 'acme', entity: { id: 'e1', name: 'Acme Ltd' } as never, links: [] }],
    skipped: [],
  });
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Acme Corp — 2 contacts');
  // Groups start unticked: nothing is created without a deliberate choice.
  expect(within(dialog).getByRole('button', { name: 'Tick a group to create it' })).toBeDisabled();
  expect(within(dialog).queryByRole('button', { name: /^Create 0/ })).toBeNull();

  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Acme Corp' }));
  fireEvent.change(within(dialog).getByLabelText('Name of Acme Corp'), {
    target: { value: 'Acme Ltd' },
  });
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Set as primary for Acme Corp' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create 1 organisation' }));

  await waitFor(() => expect(createMock).toHaveBeenCalled());
  expect(createMock).toHaveBeenCalledWith({
    kind: 'organisation',
    groups: [
      { key: 'acme', name: 'Acme Ltd', role: 'employee', set_primary: true, contact_ids: [1, 2] },
    ],
  });
  expect(await within(dialog).findByText('Created 1 organisation.')).toBeInTheDocument();
});

test('skipped groups are reported with a reason, never silently dropped', async () => {
  createMock.mockResolvedValue({
    created: [],
    skipped: [{ key: 'acme', reason: 'name_invalid' }],
  });
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Acme Corp — 2 contacts');
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Acme Corp' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create 1 organisation' }));
  expect(await within(dialog).findByText(/Acme Corp: the name is not valid/)).toBeInTheDocument();
});

// Issue #2919: a group that already matches a live entity links to it instead
// of creating a second organisation with the same name.
test('a group matching an existing entity offers to link to it, not create it', async () => {
  suggestMock.mockResolvedValue(
    page([
      group({
        key: 'green',
        name: 'Green Acres Co-op',
        variants: ['Green Acres Co-op'],
        existing_entity_id: 'e-green',
        existing_entity_name: 'Green Acres Co-op',
        contacts: [
          {
            contact_id: 11,
            firstname: 'Alice',
            lastname: 'A',
            value_name: 'Green Acres Co-op',
            has_primary: false,
          },
        ],
      }),
    ]),
  );
  createMock.mockResolvedValue({
    created: [
      {
        key: 'green',
        entity: { id: 'e-green', name: 'Green Acres Co-op' } as never,
        links: [],
      },
    ],
    skipped: [],
  });
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Green Acres Co-op — 1 contact');

  fireEvent.click(
    within(dialog).getByRole('checkbox', { name: 'Link to existing Green Acres Co-op' }),
  );
  expect(within(dialog).queryByLabelText('Name of Green Acres Co-op')).toBeNull();
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Link to existing Green Acres Co-op' }),
  );

  await waitFor(() => expect(createMock).toHaveBeenCalled());
  expect(createMock).toHaveBeenCalledWith({
    kind: 'organisation',
    groups: [
      {
        key: 'green',
        name: 'Green Acres Co-op',
        role: 'employee',
        set_primary: false,
        contact_ids: [11],
        existing_entity_id: 'e-green',
      },
    ],
  });
  // The server returns the linked group under `created`, but nothing was
  // created: the result must say it was linked.
  expect(
    await within(dialog).findByText('Linked 1 group to an existing organisation.'),
  ).toBeInTheDocument();
  expect(within(dialog).queryByText(/^Created/)).toBeNull();
});

// Issue #2919 follow-up: a mixed run reports created and linked groups
// separately, for organisations and places alike.
test.each([
  {
    tab: undefined,
    expected: 'Created 1 organisation. Linked 1 group to an existing organisation.',
  },
  {
    tab: 'Places',
    expected: 'Created 1 place. Linked 1 group to an existing place.',
  },
])('a mixed run reports created and linked groups apart ($expected)', async ({ tab, expected }) => {
  const kind = tab ? 'place' : 'organisation';
  const groups = [
    group({
      key: 'green',
      kind,
      name: 'Green Acres',
      variants: ['Green Acres'],
      existing_entity_id: 'e-green',
      existing_entity_name: 'Green Acres',
    }),
    group({ key: 'globex', kind, name: 'Globex', variants: ['Globex'] }),
  ];
  suggestMock.mockResolvedValue(page(groups));
  createMock.mockResolvedValue({
    created: [
      {
        key: 'green',
        entity: { id: 'e-green', name: 'Green Acres' } as never,
        links: [],
      },
      {
        key: 'globex',
        entity: { id: 'e-new', name: 'Globex' } as never,
        links: [],
      },
    ],
    skipped: [],
  });
  renderCard();
  const dialog = await openWizard();
  if (tab) fireEvent.click(within(dialog).getByRole('tab', { name: tab }));
  await within(dialog).findByText('Green Acres — 2 contacts');
  fireEvent.click(
    within(dialog).getByRole('checkbox', {
      name: 'Link to existing Green Acres',
    }),
  );
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Globex' }));
  fireEvent.click(
    within(dialog).getByRole('button', {
      name: 'Create new groups and link existing ones',
    }),
  );
  expect(await within(dialog).findByText(expected)).toBeInTheDocument();
});

// Issue #2919: the action copy distinguishes creating, linking and a mixed
// selection, so the user is told exactly what the helper will do.
test('the create button copy distinguishes create, link and mixed selections', async () => {
  suggestMock.mockResolvedValue(
    page([
      group({
        key: 'green',
        name: 'Green Acres Co-op',
        variants: ['Green Acres Co-op'],
        existing_entity_id: 'e-green',
        existing_entity_name: 'Green Acres Co-op',
      }),
      group({
        key: 'beta',
        name: 'Beta Co',
        variants: ['Beta Co'],
        existing_entity_id: 'e-beta',
        // No stored name: the group's own name is the fallback label.
        existing_entity_name: '',
      }),
      group({ key: 'globex', name: 'Globex', variants: ['Globex'] }),
    ]),
  );
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Green Acres Co-op — 2 contacts');

  const createButton = () => within(dialog).getByRole('button', { name: /^(Create|Link)/ });
  const toggle = (name: string) =>
    fireEvent.click(within(dialog).getByRole('checkbox', { name: `Include ${name}` }));
  const toggleLink = (name: string) =>
    fireEvent.click(within(dialog).getByRole('checkbox', { name: `Link to existing ${name}` }));

  // Only a create group: the original create copy.
  toggle('Globex');
  expect(createButton()).toHaveTextContent('Create 1 organisation');
  toggle('Globex'); // nothing selected

  // Only one link group: its existing name is in the label.
  toggleLink('Green Acres Co-op');
  expect(createButton()).toHaveTextContent('Link to existing Green Acres Co-op');
  toggleLink('Green Acres Co-op');

  // A link group with no stored existing name falls back to the group name.
  toggleLink('Beta Co');
  expect(createButton()).toHaveTextContent('Link to existing Beta Co');

  // Two link groups: the plural link label.
  toggleLink('Green Acres Co-op');
  expect(createButton()).toHaveTextContent('Link 2 groups to existing organisations');

  // Mixing in a create group switches to the combined copy.
  toggle('Globex');
  expect(createButton()).toHaveTextContent('Create new groups and link existing ones');

  fireEvent.click(createButton());
  await waitFor(() => expect(createMock).toHaveBeenCalled());
  const sent = createMock.mock.calls[0][0];
  const byKey = Object.fromEntries(sent.groups.map((g) => [g.key, g]));
  expect(byKey.green.existing_entity_id).toBe('e-green');
  expect(byKey.beta.existing_entity_id).toBe('e-beta');
  expect(byKey.globex.existing_entity_id).toBeUndefined();
});

// Issue #2919: the same link copy has a place variant, and a place group with
// no existing entity still offers the place create label.
test('place groups use the place link and create copy', async () => {
  suggestMock.mockResolvedValue(page([]));
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Nothing left to create from your contacts.');

  suggestMock.mockResolvedValue(
    page([
      group({
        key: 'p1',
        kind: 'place',
        name: '1 Main St',
        variants: ['1 Main St'],
        existing_entity_id: 'e-p1',
        existing_entity_name: '1 Main St',
      }),
      group({
        key: 'p2',
        kind: 'place',
        name: '2 Main St',
        variants: ['2 Main St'],
        existing_entity_id: 'e-p2',
        existing_entity_name: '2 Main St',
      }),
      group({ key: 'p3', kind: 'place', name: '3 Main St', variants: ['3 Main St'] }),
    ]),
  );
  fireEvent.click(within(dialog).getByRole('tab', { name: 'Places' }));
  await within(dialog).findByText('1 Main St — 2 contacts');

  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Link to existing 1 Main St' }));
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Link to existing 2 Main St' }));
  expect(
    within(dialog).getByRole('button', { name: 'Link 2 groups to existing places' }),
  ).toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include 3 Main St' }));
  expect(
    within(dialog).getByRole('button', { name: 'Create new groups and link existing ones' }),
  ).toBeInTheDocument();

  // Only the new place selected: the place create label.
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Link to existing 1 Main St' }));
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Link to existing 2 Main St' }));
  expect(within(dialog).getByRole('button', { name: 'Create 1 place' })).toBeInTheDocument();
});

test('every skip reason is explained in the user’s words', async () => {
  suggestMock.mockResolvedValue(
    page([
      group({ key: 'acme', name: 'Acme Corp' }),
      group({ key: 'beta', name: 'Beta Co' }),
      group({ key: 'gamma', name: 'Gamma Ltd' }),
    ]),
  );
  createMock.mockResolvedValue({
    created: [],
    skipped: [
      { key: 'acme', reason: 'group_not_found' },
      { key: 'beta', contact_id: 2, reason: 'contact_not_in_group' },
      { key: 'gamma', reason: 'core_invalid' },
    ],
  });
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Acme Corp — 2 contacts');
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Acme Corp' }));
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Beta Co' }));
  fireEvent.click(within(dialog).getByRole('checkbox', { name: 'Include Gamma Ltd' }));
  fireEvent.click(within(dialog).getByRole('button', { name: 'Create 3 organisations' }));

  expect(
    await within(dialog).findByText(/Acme Corp: skipped\. It may already exist or no longer match/),
  ).toBeInTheDocument();
  expect(
    within(dialog).getByText(/Beta Co: a contact no longer belongs to it\./),
  ).toBeInTheDocument();
  expect(within(dialog).getByText(/Gamma Ltd: its details are not valid\./)).toBeInTheDocument();
});

test('an entity_not_found skip names the group and explains the link failed', async () => {
  suggestMock.mockResolvedValue(
    page([
      group({
        key: 'green',
        name: 'Green Acres Co-op',
        variants: ['Green Acres Co-op'],
        existing_entity_id: 'e-green',
        existing_entity_name: 'Green Acres Co-op',
      }),
    ]),
  );
  createMock.mockResolvedValue({
    created: [],
    skipped: [{ key: 'green', reason: 'entity_not_found' }],
  });
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Green Acres Co-op — 2 contacts');
  fireEvent.click(
    within(dialog).getByRole('checkbox', { name: 'Link to existing Green Acres Co-op' }),
  );
  fireEvent.click(
    within(dialog).getByRole('button', { name: 'Link to existing Green Acres Co-op' }),
  );
  expect(
    await within(dialog).findByText(
      /Green Acres Co-op: the organisation or place it matched no longer exists/,
    ),
  ).toBeInTheDocument();
});

test('a failed scan or create shows the error and keeps the dialog', async () => {
  suggestMock.mockRejectedValueOnce(new ApiError('scan broke', 'ERR', 500));
  renderCard();
  const dialog = await openWizard();
  expect(await within(dialog).findByRole('alert')).toHaveTextContent('scan broke');
});

test('Load more appends the next page of groups', async () => {
  suggestMock.mockResolvedValueOnce(page([group()], 'c2'));
  suggestMock.mockResolvedValueOnce(
    page([
      group({
        key: 'globex',
        name: 'Globex',
        variants: ['Globex'],
        contacts: [
          {
            contact_id: 3,
            firstname: 'Cy',
            lastname: 'C',
            value_name: 'Globex',
            has_primary: false,
          },
        ],
      }),
    ]),
  );
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Acme Corp — 2 contacts');
  fireEvent.click(within(dialog).getByRole('button', { name: 'Load more' }));
  expect(await within(dialog).findByText('Globex — 1 contact')).toBeInTheDocument();
  expect(suggestMock).toHaveBeenLastCalledWith(expect.objectContaining({ cursor: 'c2' }));
});

// #2715: nothing selectable means nothing to create: only Close is offered.
test('with no groups left only Close is offered, not a disabled create button (#2715)', async () => {
  suggestMock.mockResolvedValue(page([]));
  renderCard();
  const dialog = await openWizard();
  await within(dialog).findByText('Nothing left to create from your contacts.');
  expect(within(dialog).getByRole('button', { name: 'Close' })).toBeInTheDocument();
  expect(within(dialog).queryByRole('button', { name: /^Create/ })).toBeNull();
  expect(within(dialog).queryByRole('button', { name: 'Tick a group to create it' })).toBeNull();
});
