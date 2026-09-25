import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import './i18n/config';
import type { Contact, ContactsResponse } from './api/contacts';
import { getContacts, getContactsByUid } from './api/contacts';
import { suggestContactAddresses } from './api/dataSuggestions';
import {
  acceptAddressHouseholdSuggestion,
  createHousehold,
  deleteHousehold,
  dismissAddressHouseholdSuggestion,
  type Household,
  type HouseholdListResponse,
  type HouseholdMember,
  listHouseholds,
  removeHouseholdMember,
  suggestAddressHouseholds,
  suggestHouseholdRelationships,
  updateHousehold,
  updateHouseholdMember,
} from './api/households';
import { SnackbarProvider } from './context/SnackbarContext';
import HouseholdsPage from './HouseholdsPage';

// This codebase's vitest setup has no auto-cleanup and no globals: true.
afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('./api/households', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/households')>();
  return {
    ...actual,
    listHouseholds: vi.fn(),
    createHousehold: vi.fn(),
    updateHousehold: vi.fn(),
    deleteHousehold: vi.fn(),
    addHouseholdMember: vi.fn(),
    removeHouseholdMember: vi.fn(),
    updateHouseholdMember: vi.fn(),
    suggestHouseholdRelationships: vi.fn(),
    suggestAddressHouseholds: vi.fn(),
    acceptAddressHouseholdSuggestion: vi.fn(),
    dismissAddressHouseholdSuggestion: vi.fn(),
  };
});

vi.mock('./api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/contacts')>();
  return {
    ...actual,
    getContactsByUid: vi.fn(),
    getContacts: vi.fn(),
  };
});

vi.mock('./api/dataSuggestions', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./api/dataSuggestions')>();
  return {
    ...actual,
    suggestContactAddresses: vi.fn(),
  };
});

const listHouseholdsMock = vi.mocked(listHouseholds);
const createHouseholdMock = vi.mocked(createHousehold);
const updateHouseholdMock = vi.mocked(updateHousehold);
const deleteHouseholdMock = vi.mocked(deleteHousehold);
const removeHouseholdMemberMock = vi.mocked(removeHouseholdMember);
const updateHouseholdMemberMock = vi.mocked(updateHouseholdMember);
const suggestHouseholdRelationshipsMock = vi.mocked(suggestHouseholdRelationships);
const suggestAddressHouseholdsMock = vi.mocked(suggestAddressHouseholds);
const acceptAddressHouseholdSuggestionMock = vi.mocked(acceptAddressHouseholdSuggestion);
const dismissAddressHouseholdSuggestionMock = vi.mocked(dismissAddressHouseholdSuggestion);
const getContactsByUidMock = vi.mocked(getContactsByUid);
const getContactsMock = vi.mocked(getContacts);
const suggestContactAddressesMock = vi.mocked(suggestContactAddresses);

function household(overrides: Partial<Household> = {}): Household {
  return {
    id: 'h1',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    name: 'Smith Family',
    type: 'family_unit',
    ...overrides,
  };
}

function member(overrides: Partial<HouseholdMember> = {}): HouseholdMember {
  return {
    id: 1,
    household_id: 'h1',
    member_vcard_uid: 'alice-uid',
    role: 'adult',
    ...overrides,
  };
}

function listResponse(overrides: Partial<HouseholdListResponse> = {}): HouseholdListResponse {
  return {
    households: [household()],
    total: 1,
    next_cursor: '',
    limit: 200,
    members: [member()],
    ...overrides,
  };
}

function contact(overrides: Partial<Contact> = {}): Contact {
  return { ID: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Anderson', ...overrides };
}

beforeEach(() => {
  getContactsByUidMock.mockResolvedValue(new Map([['alice-uid', contact()]]));
  suggestContactAddressesMock.mockResolvedValue({ suggestions: [], total: 0 });
  getContactsMock.mockResolvedValue({
    contacts: [],
    next_cursor: '',
    limit: 40,
  } as ContactsResponse);
});

function renderPage() {
  return render(
    <SnackbarProvider>
      <HouseholdsPage />
    </SnackbarProvider>,
  );
}

test('loads and renders the household list with resolved member names', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());

  renderPage();

  expect(await screen.findByText('Smith Family')).toBeInTheDocument();
  expect(screen.getByText('Alice Anderson')).toBeInTheDocument();
  expect(listHouseholdsMock).toHaveBeenCalledWith({ limit: 200, include_members: true });
});

test('renders the empty state when there are no households', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse({ households: [], members: [] }));

  renderPage();

  expect(
    await screen.findByText(
      'No households yet. Create one to start suggesting relationships between the people who live together.',
    ),
  ).toBeInTheDocument();
});

test('a fetch failure surfaces an error alert', async () => {
  listHouseholdsMock.mockRejectedValue(new Error('network down'));

  renderPage();

  expect(await screen.findByText('network down')).toBeInTheDocument();
});

test('creating a household calls createHousehold with the entered name and type, then shows success', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse({ households: [], members: [] }));
  createHouseholdMock.mockResolvedValue(household({ id: 'h2', name: 'New Household' }));

  renderPage();
  await screen.findByText(/No households yet/);

  fireEvent.click(screen.getByRole('button', { name: 'Add Household' }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'New Household' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    expect(createHouseholdMock).toHaveBeenCalledWith({
      name: 'New Household',
      type: 'family_unit',
    });
  });
  expect(await screen.findByText('Household created')).toBeInTheDocument();
});

test('a successful create that finds address suggestions nudges the user to review them', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse({ households: [], members: [] }));
  createHouseholdMock.mockResolvedValue(household({ id: 'h2', name: 'New Household' }));
  suggestContactAddressesMock.mockResolvedValue({
    suggestions: [
      {
        contact_vcard_uid: 'alice-uid',
        contact_name: 'Alice Anderson',
        source_kind: 'household',
        source_id: 'h1',
        source_name: 'Smith Family',
        address_key: 'addr-key-1',
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });

  renderPage();
  await screen.findByText(/No households yet/);

  fireEvent.click(screen.getByRole('button', { name: 'Add Household' }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'New Household' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  expect(
    await screen.findByText(
      'New address suggestions are available — review them under Settings → Data.',
    ),
  ).toBeInTheDocument();
});

test('a failing best-effort address-suggestion nudge after create does not break the create flow', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse({ households: [], members: [] }));
  createHouseholdMock.mockResolvedValue(household({ id: 'h2', name: 'New Household' }));
  suggestContactAddressesMock.mockRejectedValue(new Error('scan boom'));

  renderPage();
  await screen.findByText(/No households yet/);

  fireEvent.click(screen.getByRole('button', { name: 'Add Household' }));
  fireEvent.change(screen.getByLabelText('Name *'), { target: { value: 'New Household' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  // The create itself still succeeds and reports success even though the
  // nudge fetch failed -- it's explicitly best-effort.
  expect(await screen.findByText('Household created')).toBeInTheDocument();
});

test('editing a household pre-fills the dialog and calls updateHousehold with the id', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  updateHouseholdMock.mockResolvedValue(household({ name: 'Smith Family Renamed' }));

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
  const nameInput = screen.getByLabelText('Name *') as HTMLInputElement;
  expect(nameInput.value).toBe('Smith Family');

  fireEvent.change(nameInput, { target: { value: 'Smith Family Renamed' } });
  fireEvent.click(screen.getByRole('button', { name: 'Save' }));

  await waitFor(() => {
    expect(updateHouseholdMock).toHaveBeenCalledWith('h1', {
      name: 'Smith Family Renamed',
      type: 'family_unit',
    });
  });
  expect(await screen.findByText('Household updated')).toBeInTheDocument();
});

test('deleting a household prompts for confirmation and calls deleteHousehold when confirmed', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  listHouseholdsMock.mockResolvedValue(listResponse());
  deleteHouseholdMock.mockResolvedValue(undefined);

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(confirmSpy).toHaveBeenCalledWith(
    'Are you sure you want to delete this household? Its members will not be deleted.',
  );
  await waitFor(() => expect(deleteHouseholdMock).toHaveBeenCalledWith('h1'));
  expect(await screen.findByText('Household deleted')).toBeInTheDocument();

  confirmSpy.mockRestore();
});

test('declining the delete confirmation never calls deleteHousehold', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(false);
  listHouseholdsMock.mockResolvedValue(listResponse());

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  expect(confirmSpy).toHaveBeenCalled();
  expect(deleteHouseholdMock).not.toHaveBeenCalled();

  confirmSpy.mockRestore();
});

test('a failed delete is swallowed by the page (error already surfaced by the hook)', async () => {
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  listHouseholdsMock.mockResolvedValue(listResponse());
  deleteHouseholdMock.mockRejectedValue(new Error('delete boom'));

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Delete' }));

  await waitFor(() => expect(deleteHouseholdMock).toHaveBeenCalled());
  // No "Household deleted" success toast, and no unhandled rejection crash.
  expect(screen.queryByText('Household deleted')).toBeNull();

  confirmSpy.mockRestore();
});

test('suggesting relationships reports the number of newly generated suggestions', async () => {
  listHouseholdsMock.mockResolvedValue(
    listResponse({ members: [member(), member({ id: 2, member_vcard_uid: 'bob-uid' })] }),
  );
  suggestHouseholdRelationshipsMock.mockResolvedValue({
    message: 'ok',
    household_id: 'h1',
    suggested_edges: [],
    total: 3,
  });

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest relationships' }));

  await waitFor(() => expect(suggestHouseholdRelationshipsMock).toHaveBeenCalledWith('h1'));
  expect(
    await screen.findByText(
      "Generated 3 new suggested relationship(s). Review them on the members' contact pages.",
    ),
  ).toBeInTheDocument();
});

test('suggesting relationships with no new results shows the "no new suggestions" info message', async () => {
  listHouseholdsMock.mockResolvedValue(
    listResponse({ members: [member(), member({ id: 2, member_vcard_uid: 'bob-uid' })] }),
  );
  suggestHouseholdRelationshipsMock.mockResolvedValue({
    message: 'ok',
    household_id: 'h1',
    suggested_edges: [],
    total: 0,
  });

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest relationships' }));

  expect(
    await screen.findByText(
      'No new suggestions — the existing relationships already cover this household.',
    ),
  ).toBeInTheDocument();
});

test('a suggest-relationships failure is swallowed without crashing the page', async () => {
  listHouseholdsMock.mockResolvedValue(
    listResponse({ members: [member(), member({ id: 2, member_vcard_uid: 'bob-uid' })] }),
  );
  suggestHouseholdRelationshipsMock.mockRejectedValue(new Error('suggest boom'));

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest relationships' }));

  await waitFor(() => expect(suggestHouseholdRelationshipsMock).toHaveBeenCalled());
  expect(screen.queryByText(/Generated \d+ new/)).toBeNull();
});

test('changing a member role calls updateHouseholdMember with the new role', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  updateHouseholdMemberMock.mockResolvedValue(undefined);

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.mouseDown(screen.getByLabelText('Role'));
  fireEvent.click(await screen.findByRole('option', { name: 'Child' }));

  await waitFor(() => {
    expect(updateHouseholdMemberMock).toHaveBeenCalledWith('h1', 'alice-uid', 'child');
  });
});

test('a failed role change triggers a refresh so the select reverts to the server state', async () => {
  listHouseholdsMock.mockResolvedValueOnce(listResponse());
  updateHouseholdMemberMock.mockRejectedValue(new Error('role change boom'));
  listHouseholdsMock.mockResolvedValueOnce(listResponse());

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.mouseDown(screen.getByLabelText('Role'));
  fireEvent.click(await screen.findByRole('option', { name: 'Child' }));

  await waitFor(() => expect(listHouseholdsMock).toHaveBeenCalledTimes(2));
});

test('removing a member calls removeHouseholdMember with the household id and member uid', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  removeHouseholdMemberMock.mockResolvedValue(undefined);

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByLabelText('Remove Member'));

  await waitFor(() => {
    expect(removeHouseholdMemberMock).toHaveBeenCalledWith('h1', 'alice-uid');
  });
});

test('a failed member removal does not crash the page', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  removeHouseholdMemberMock.mockRejectedValue(new Error('remove boom'));

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByLabelText('Remove Member'));

  await waitFor(() => expect(removeHouseholdMemberMock).toHaveBeenCalled());
  expect(screen.getByText('Smith Family')).toBeInTheDocument();
});

test('adding a member via the contact search calls addHouseholdMember with the selected contact', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  getContactsMock.mockResolvedValue({
    contacts: [{ ID: 2, uid: 'carol-uid', firstname: 'Carol', lastname: 'Clark' }],
    next_cursor: '',
    limit: 40,
  } as ContactsResponse);
  const { addHouseholdMember } = await import('./api/households');
  vi.mocked(addHouseholdMember).mockResolvedValue({
    id: 9,
    household_id: 'h1',
    member_vcard_uid: 'carol-uid',
  });

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Add Member' }));
  const searchBox = screen.getByPlaceholderText('Search contacts…');
  fireEvent.change(searchBox, { target: { value: 'carol' } });

  const option = await screen.findByRole('option', { name: 'Carol Clark' });
  fireEvent.click(option);

  await waitFor(() => {
    expect(vi.mocked(addHouseholdMember)).toHaveBeenCalledWith('h1', {
      member_vcard_uid: 'carol-uid',
      role: 'adult',
    });
  });
});

test('scanning for address suggestions renders the returned suggestions', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockResolvedValue({
    suggestions: [
      {
        address_hash: 'addr-1',
        member_hash: 'mem-1',
        member_vcard_uids: ['alice-uid'],
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));

  expect(await screen.findByText('123 Main St')).toBeInTheDocument();
  expect(screen.getByText('Shared-address suggestions')).toBeInTheDocument();
});

test('a null suggestions payload from the backend normalizes to the empty state instead of crashing', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  // T64: a nil Go slice marshals as `null`; the TS type promises an array.
  suggestAddressHouseholdsMock.mockResolvedValue({
    suggestions: null as unknown as [],
    total: 0,
  });

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));

  expect(
    await screen.findByText(
      'No shared-address suggestions found. Add addresses to more contacts to find groups here.',
    ),
  ).toBeInTheDocument();
});

test('a scan failure surfaces the scanFailed error message', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockRejectedValue(new Error('scan endpoint down'));

  renderPage();
  await screen.findByText('Smith Family');

  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));

  expect(await screen.findByText("Couldn't scan shared addresses")).toBeInTheDocument();
});

test('accepting an address suggestion calls acceptAddressHouseholdSuggestion, removes it, and refreshes', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockResolvedValue({
    suggestions: [
      {
        address_hash: 'addr-1',
        member_hash: 'mem-1',
        member_vcard_uids: ['alice-uid'],
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });
  acceptAddressHouseholdSuggestionMock.mockResolvedValue(household({ id: 'h9' }));

  renderPage();
  await screen.findByText('Smith Family');
  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));
  await screen.findByText('123 Main St');

  fireEvent.click(screen.getByRole('button', { name: 'Accept' }));

  await waitFor(() => {
    expect(acceptAddressHouseholdSuggestionMock).toHaveBeenCalledWith(['alice-uid']);
  });
  expect(await screen.findByText('Household created from the suggestion')).toBeInTheDocument();
  // The accepted card is removed from the list.
  expect(screen.queryByText('123 Main St')).toBeNull();
});

test('a failed accept re-scans instead of leaving a stale suggestion displayed', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockResolvedValueOnce({
    suggestions: [
      {
        address_hash: 'addr-1',
        member_hash: 'mem-1',
        member_vcard_uids: ['alice-uid'],
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });
  acceptAddressHouseholdSuggestionMock.mockRejectedValue(new Error('accept boom'));
  suggestAddressHouseholdsMock.mockResolvedValueOnce({ suggestions: [], total: 0 });

  renderPage();
  await screen.findByText('Smith Family');
  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));
  await screen.findByText('123 Main St');

  fireEvent.click(screen.getByRole('button', { name: 'Accept' }));

  await waitFor(() => expect(suggestAddressHouseholdsMock).toHaveBeenCalledTimes(2));
  expect(
    await screen.findByText(
      'No shared-address suggestions found. Add addresses to more contacts to find groups here.',
    ),
  ).toBeInTheDocument();
});

test('dismissing an address suggestion calls dismissAddressHouseholdSuggestion and removes it', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockResolvedValue({
    suggestions: [
      {
        address_hash: 'addr-1',
        member_hash: 'mem-1',
        member_vcard_uids: ['alice-uid'],
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });
  dismissAddressHouseholdSuggestionMock.mockResolvedValue(undefined);

  renderPage();
  await screen.findByText('Smith Family');
  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));
  await screen.findByText('123 Main St');

  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));

  await waitFor(() => {
    expect(dismissAddressHouseholdSuggestionMock).toHaveBeenCalledWith(['alice-uid']);
  });
  expect(await screen.findByText('Suggestion dismissed')).toBeInTheDocument();
  expect(screen.queryByText('123 Main St')).toBeNull();
});

test('a failed dismiss surfaces the fetch error but leaves the page usable', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  suggestAddressHouseholdsMock.mockResolvedValue({
    suggestions: [
      {
        address_hash: 'addr-1',
        member_hash: 'mem-1',
        member_vcard_uids: ['alice-uid'],
        address: { full: '123 Main St' },
      },
    ],
    total: 1,
  });
  dismissAddressHouseholdSuggestionMock.mockRejectedValue(new Error('dismiss boom'));

  renderPage();
  await screen.findByText('Smith Family');
  fireEvent.click(screen.getByRole('button', { name: 'Suggest Households' }));
  await screen.findByText('123 Main St');

  fireEvent.click(screen.getByRole('button', { name: 'Dismiss' }));

  await waitFor(() => expect(dismissAddressHouseholdSuggestionMock).toHaveBeenCalled());
  // The card stays -- dismiss failed, so it wasn't filtered out.
  expect(screen.getByText('123 Main St')).toBeInTheDocument();
});

test('a member-resolution failure falls back to an empty contact map instead of crashing', async () => {
  listHouseholdsMock.mockResolvedValue(listResponse());
  getContactsByUidMock.mockRejectedValue(new Error('resolve boom'));

  renderPage();

  // Falls back to displaying the raw vcard uid when names can't be resolved.
  expect(await screen.findByText('alice-uid')).toBeInTheDocument();
});
