import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createPreference,
  deletePreference,
  getPreferences,
  type Preference,
  type PreferenceInput,
  updatePreference,
} from '../api/preferences';
import { usePreferences } from './usePreferences';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/preferences', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/preferences')>();
  return {
    ...actual,
    getPreferences: vi.fn(),
    createPreference: vi.fn(),
    updatePreference: vi.fn(),
    deletePreference: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getPreferences).mockReset();
  vi.mocked(createPreference).mockReset();
  vi.mocked(updatePreference).mockReset();
  vi.mocked(deletePreference).mockReset();
});

const preference: Preference = {
  id: 'p-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'uid-1',
  category: 'food',
  value: 'pizza',
  sensitivity: 'normal',
};

test('loads preferences on mount', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getPreferences).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 200 });
  expect(result.current.preferences).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => usePreferences(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getPreferences).not.toHaveBeenCalled();
  expect(result.current.preferences).toEqual([]);
});

test('handleSave updates an existing preference', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: PreferenceInput = { entity_id: 'uid-1', category: 'food', value: 'sushi' };
  await act(async () => {
    await result.current.handleSave(preference, input);
  });

  expect(updatePreference).toHaveBeenCalledWith('p-1', input);
  expect(createPreference).not.toHaveBeenCalled();
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('handleSave creates when no preference exists', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [],
    total: 0,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: PreferenceInput = { entity_id: 'uid-1', category: 'food', value: 'pizza' };
  await act(async () => {
    await result.current.handleSave(null, input);
  });

  expect(createPreference).toHaveBeenCalledWith(input);
  expect(updatePreference).not.toHaveBeenCalled();
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes and refreshes', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('p-1');
  });

  expect(deletePreference).toHaveBeenCalledWith('p-1');
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('save errors notify through the notifier and rethrow', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [],
    total: 0,
    next_cursor: '',
    limit: 200,
  });
  vi.mocked(createPreference).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => usePreferences('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    result.current.handleSave(null, { entity_id: 'uid-1', category: 'food', value: 'x' }),
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getPreferences).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.preferences).toEqual([]);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty, asks for up to 200 preferences of the entity, and reports loading', async () => {
  const d = deferred<Awaited<ReturnType<typeof getPreferences>>>();
  vi.mocked(getPreferences).mockReturnValue(d.promise);
  const { result } = renderHook(() => usePreferences('uid-1'));
  expect(result.current.preferences).toEqual([]);
  expect(result.current.error).toBeNull();
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getPreferences).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 200 });
  await act(async () => {
    d.resolve({ preferences: [preference] } as never);
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
});

test('a response without preferences yields an empty list, and a failed load is logged then cleared on refresh', async () => {
  vi.mocked(getPreferences).mockResolvedValueOnce({} as never);
  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.preferences).toEqual([]);

  vi.mocked(getPreferences).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('offline');
  expect(console.error).toHaveBeenCalledWith('[fetching preferences] Error:', expect.anything());

  const d = deferred<Awaited<ReturnType<typeof getPreferences>>>();
  vi.mocked(getPreferences).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve({ preferences: [] } as never);
  });
});

test('refetches when the entity id changes', async () => {
  vi.mocked(getPreferences).mockResolvedValue({ preferences: [] } as never);
  const { rerender } = renderHook(({ id }) => usePreferences(id), { initialProps: { id: 'a' } });
  await waitFor(() => expect(getPreferences).toHaveBeenCalledTimes(1));
  rerender({ id: 'b' });
  await waitFor(() =>
    expect(getPreferences).toHaveBeenLastCalledWith({ entityId: 'b', limit: 200 }),
  );
});

test('save and delete failures log their operations and use the latest notifier', async () => {
  vi.mocked(getPreferences).mockResolvedValue({ preferences: [] } as never);
  vi.mocked(createPreference).mockRejectedValue(new Error('s'));
  vi.mocked(deletePreference).mockRejectedValue(new Error('d'));
  const first = { showError: vi.fn() };
  const second = { showError: vi.fn() };
  const { result, rerender } = renderHook(({ n }) => usePreferences('uid-1', n), {
    initialProps: { n: first },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ n: second });
  await act(async () => {
    await expect(result.current.handleSave(null, {} as PreferenceInput)).rejects.toThrow('s');
    await expect(result.current.handleDelete('p-1')).rejects.toThrow('d');
  });
  expect(first.showError).not.toHaveBeenCalled();
  expect(second.showError.mock.calls.map((c) => c[0])).toEqual(['s', 'd']);
  expect(console.error).toHaveBeenCalledWith('[saving preference] Error:', expect.anything());
  expect(console.error).toHaveBeenCalledWith('[deleting preference] Error:', expect.anything());
});
