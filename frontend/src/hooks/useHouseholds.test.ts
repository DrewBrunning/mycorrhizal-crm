import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useHouseholds } from './useHouseholds';
import {
  listHouseholds,
  createHousehold,
  updateHousehold,
  deleteHousehold,
  addHouseholdMember,
  removeHouseholdMember,
  updateHouseholdMember,
  suggestHouseholdRelationships,
  Household,
  HouseholdMember,
  HouseholdInput,
  HouseholdListResponse,
} from '../api/households';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/households', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/households')>();
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
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listHouseholds).mockReset();
  vi.mocked(createHousehold).mockReset();
  vi.mocked(updateHousehold).mockReset();
  vi.mocked(deleteHousehold).mockReset();
  vi.mocked(addHouseholdMember).mockReset();
  vi.mocked(removeHouseholdMember).mockReset();
  vi.mocked(updateHouseholdMember).mockReset();
  vi.mocked(suggestHouseholdRelationships).mockReset();
});

const household: Household = {
  id: 'h-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  name: 'The Smiths',
  type: 'family_unit',
};

const member: HouseholdMember = {
  id: 1,
  household_id: 'h-1',
  member_vcard_uid: 'uid-1',
  role: 'adult',
};

function listResponse(households: Household[], members: HouseholdMember[]): HouseholdListResponse {
  return { households, members, total: households.length, next_cursor: '', limit: 200 };
}

test('loads households and members on mount', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], [member]));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(listHouseholds).toHaveBeenCalledWith({ limit: 200, include_members: true });
  expect(result.current.households).toHaveLength(1);
  expect(result.current.members).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('handleCreate creates, refreshes and returns the household', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([], []));
  vi.mocked(createHousehold).mockResolvedValue(household);

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: HouseholdInput = { name: 'The Smiths', type: 'family_unit' };
  await expect(result.current.handleCreate(input)).resolves.toEqual(household);
  expect(createHousehold).toHaveBeenCalledWith(input);
  expect(listHouseholds).toHaveBeenCalledTimes(2);
});

test('handleUpdate updates and refreshes', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: HouseholdInput = { name: 'Renamed', type: 'other' };
  await act(async () => {
    await result.current.handleUpdate('h-1', input);
  });

  expect(updateHousehold).toHaveBeenCalledWith('h-1', input);
  expect(listHouseholds).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes and refreshes', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('h-1');
  });

  expect(deleteHousehold).toHaveBeenCalledWith('h-1');
  expect(listHouseholds).toHaveBeenCalledTimes(2);
});

test('handleAddMember adds a member with its role', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleAddMember('h-1', 'uid-9', 'child');
  });

  expect(addHouseholdMember).toHaveBeenCalledWith('h-1', { member_vcard_uid: 'uid-9', role: 'child' });
});

test('handleAddMember omits role when none is given', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleAddMember('h-1', 'uid-9');
  });

  expect(addHouseholdMember).toHaveBeenCalledWith('h-1', { member_vcard_uid: 'uid-9', role: undefined });
});

test('handleRemoveMember removes and refreshes', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleRemoveMember('h-1', 'uid-1');
  });

  expect(removeHouseholdMember).toHaveBeenCalledWith('h-1', 'uid-1');
  expect(listHouseholds).toHaveBeenCalledTimes(2);
});

test('handleUpdateMember patches the role', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleUpdateMember('h-1', 'uid-1', 'adult');
  });

  expect(updateHouseholdMember).toHaveBeenCalledWith('h-1', 'uid-1', 'adult');
  expect(listHouseholds).toHaveBeenCalledTimes(2);
});

test('handleSuggestRelationships returns the new edge count', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(suggestHouseholdRelationships).mockResolvedValue({
    message: 'ok',
    household_id: 'h-1',
    suggested_edges: [],
    total: 3,
  });

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleSuggestRelationships('h-1')).resolves.toBe(3);
});

test('mutation errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(updateHousehold).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    result.current.handleUpdate('h-1', { name: 'x', type: 'other' })
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(listHouseholds).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.households).toEqual([]);
});
