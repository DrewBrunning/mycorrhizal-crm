import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  addHouseholdMember,
  createHousehold,
  deleteHousehold,
  type Household,
  type HouseholdInput,
  type HouseholdListResponse,
  type HouseholdMember,
  listHouseholds,
  removeHouseholdMember,
  suggestHouseholdRelationships,
  updateHousehold,
  updateHouseholdMember,
} from '../api/households';
import { useHouseholds } from './useHouseholds';

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

test('defaults to empty lists when the response omits households or members', async () => {
  vi.mocked(listHouseholds).mockResolvedValue({} as HouseholdListResponse);

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.households).toEqual([]);
  expect(result.current.members).toEqual([]);
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

  expect(addHouseholdMember).toHaveBeenCalledWith('h-1', {
    member_vcard_uid: 'uid-9',
    role: 'child',
  });
});

test('handleAddMember omits role when none is given', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleAddMember('h-1', 'uid-9');
  });

  expect(addHouseholdMember).toHaveBeenCalledWith('h-1', {
    member_vcard_uid: 'uid-9',
    role: undefined,
  });
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

  await expect(result.current.handleUpdate('h-1', { name: 'x', type: 'other' })).rejects.toThrow(
    'boom',
  );
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(listHouseholds).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.households).toEqual([]);
});

test('create errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([], []));
  vi.mocked(createHousehold).mockRejectedValue(new Error('create failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleCreate({ name: 'x', type: 'family_unit' })).rejects.toThrow(
    'create failed',
  );
  expect(showError).toHaveBeenCalledWith('create failed');
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});

test('delete errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(deleteHousehold).mockRejectedValue(new Error('delete failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete('h-1')).rejects.toThrow('delete failed');
  expect(showError).toHaveBeenCalledWith('delete failed');
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});

test('add-member errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(addHouseholdMember).mockRejectedValue(new Error('add failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleAddMember('h-1', 'uid-9')).rejects.toThrow('add failed');
  expect(showError).toHaveBeenCalledWith('add failed');
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});

test('remove-member errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(removeHouseholdMember).mockRejectedValue(new Error('remove failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleRemoveMember('h-1', 'uid-1')).rejects.toThrow('remove failed');
  expect(showError).toHaveBeenCalledWith('remove failed');
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});

test('update-member errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(updateHouseholdMember).mockRejectedValue(new Error('update member failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleUpdateMember('h-1', 'uid-1', 'adult')).rejects.toThrow(
    'update member failed',
  );
  expect(showError).toHaveBeenCalledWith('update member failed');
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});

test('suggest-relationships errors notify through the notifier and rethrow', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([household], []));
  vi.mocked(suggestHouseholdRelationships).mockRejectedValue(new Error('suggest failed'));
  const showError = vi.fn();

  const { result } = renderHook(() => useHouseholds({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleSuggestRelationships('h-1')).rejects.toThrow('suggest failed');
  expect(showError).toHaveBeenCalledWith('suggest failed');
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty, asks for up to 200 households with members, and reports loading while in flight', async () => {
  const d = deferred<HouseholdListResponse>();
  vi.mocked(listHouseholds).mockReturnValue(d.promise);
  const { result } = renderHook(() => useHouseholds());
  expect(result.current.households).toEqual([]);
  expect(result.current.members).toEqual([]);
  expect(result.current.error).toBeNull();
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(listHouseholds).toHaveBeenCalledWith({ limit: 200, include_members: true });
  await act(async () => {
    d.resolve(listResponse([household], [member]));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.households).toEqual([household]);
  expect(result.current.members).toEqual([member]);
});

test('a failed load logs its operation and the next refresh clears the error', async () => {
  vi.mocked(listHouseholds).mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useHouseholds());
  await waitFor(() => expect(result.current.error).toBe('offline'));
  expect(console.error).toHaveBeenCalledWith('[fetching households] Error:', expect.anything());

  const d = deferred<HouseholdListResponse>();
  vi.mocked(listHouseholds).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(listResponse([], []));
  });
});

test('every mutation logs its own operation and uses the latest notifier', async () => {
  vi.mocked(listHouseholds).mockResolvedValue(listResponse([], []));
  const boom = new Error('x');
  vi.mocked(createHousehold).mockRejectedValue(boom);
  vi.mocked(updateHousehold).mockRejectedValue(boom);
  vi.mocked(deleteHousehold).mockRejectedValue(boom);
  vi.mocked(addHouseholdMember).mockRejectedValue(boom);
  vi.mocked(removeHouseholdMember).mockRejectedValue(boom);
  vi.mocked(updateHouseholdMember).mockRejectedValue(boom);
  vi.mocked(suggestHouseholdRelationships).mockRejectedValue(boom);
  const first = { showError: vi.fn() };
  const second = { showError: vi.fn() };
  const { result, rerender } = renderHook(({ n }) => useHouseholds(n), {
    initialProps: { n: first },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ n: second });
  await act(async () => {
    await expect(result.current.handleCreate({} as HouseholdInput)).rejects.toThrow();
    await expect(result.current.handleUpdate('h', {} as HouseholdInput)).rejects.toThrow();
    await expect(result.current.handleDelete('h')).rejects.toThrow();
    await expect(result.current.handleAddMember('h', 'u')).rejects.toThrow();
    await expect(result.current.handleRemoveMember('h', 'u')).rejects.toThrow();
    await expect(result.current.handleUpdateMember('h', 'u', 'r')).rejects.toThrow();
    await expect(result.current.handleSuggestRelationships('h')).rejects.toThrow();
  });
  expect(first.showError).not.toHaveBeenCalled();
  expect(second.showError).toHaveBeenCalledTimes(7);
  for (const op of [
    'creating household',
    'updating household',
    'deleting household',
    'adding household member',
    'removing household member',
    'updating household member role',
    'generating relationship suggestions',
  ]) {
    expect(console.error).toHaveBeenCalledWith(`[${op}] Error:`, expect.anything());
  }
  // None of the failed mutations reloaded the list.
  expect(listHouseholds).toHaveBeenCalledTimes(1);
});
