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
  fireEvent.click(screen.getByRole('button', { name: 'Create entities from my contacts…' }));
  return screen.findByRole('dialog', { name: 'Create entities from my contacts' });
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
  expect(within(dialog).getByRole('button', { name: 'Create 0 organisations' })).toBeDisabled();

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
