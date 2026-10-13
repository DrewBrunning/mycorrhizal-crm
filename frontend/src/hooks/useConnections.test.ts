import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getConnections } from '../api/graph';
import { useConnections } from './useConnections';

vi.mock('../api/graph', () => ({ getConnections: vi.fn() }));

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getConnections).mockResolvedValue({ connections: [] } as never);
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

test('refresh is a no-op without a starting contact', async () => {
  const { result } = renderHook(() => useConnections(undefined));
  await act(() => result.current.refresh());
  expect(getConnections).not.toHaveBeenCalled();
  expect(result.current.loading).toBe(false);
});

test('defaults to one hop and drops a blank relation', async () => {
  const { result } = renderHook(() => useConnections('alice-uid'));
  await act(() => result.current.refresh({ relation: '   ' }));
  expect(getConnections).toHaveBeenCalledWith({ from: 'alice-uid', depth: 1, relation: undefined });
  expect(result.current.connections).toEqual({ connections: [] });
});

test('passes depth, a trimmed relation, and an override uid through', async () => {
  const { result } = renderHook(() => useConnections('alice-uid'));
  await act(() =>
    result.current.refresh({ depth: 3, relation: ' friend_of ', overrideUid: 'bob-uid' }),
  );
  expect(getConnections).toHaveBeenCalledWith({ from: 'bob-uid', depth: 3, relation: 'friend_of' });
});

test('a failed fetch surfaces an error and clears loading', async () => {
  vi.mocked(getConnections).mockRejectedValue(new Error('boom'));
  const { result } = renderHook(() => useConnections('alice-uid'));
  await act(() => result.current.refresh());
  expect(result.current.error).toBeTruthy();
  expect(result.current.loading).toBe(false);
});

// --- behavior pins (issue #1662) ---

test('starts with no connections and idle; reports loading and clears an old error on retry', async () => {
  const { result } = renderHook(() => useConnections('alice-uid'));
  expect(result.current.connections).toBeNull();
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();

  vi.mocked(getConnections).mockRejectedValueOnce(new Error('boom'));
  await act(() => result.current.refresh());
  expect(result.current.error).toBe('boom');
  expect(console.error).toHaveBeenCalledWith('[fetching connections] Error:', expect.anything());

  let release!: (v: unknown) => void;
  vi.mocked(getConnections).mockReturnValueOnce(
    new Promise((res) => {
      release = res;
    }) as never,
  );
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    release({ connections: [] });
  });
});

test('a relation passed without depth or override uses the defaults; a blank-after-trim relation is dropped', async () => {
  const { result } = renderHook(() => useConnections('alice-uid'));
  await act(() => result.current.refresh({ relation: 'friend_of' }));
  expect(getConnections).toHaveBeenLastCalledWith({
    from: 'alice-uid',
    depth: 1,
    relation: 'friend_of',
  });
  await act(() => result.current.refresh({ depth: 2 }));
  expect(getConnections).toHaveBeenLastCalledWith({
    from: 'alice-uid',
    depth: 2,
    relation: undefined,
  });
});

test('refresh follows the starting contact when it changes', async () => {
  const { result, rerender } = renderHook(({ uid }) => useConnections(uid), {
    initialProps: { uid: 'alice-uid' as string | undefined },
  });
  rerender({ uid: 'carol-uid' });
  await act(() => result.current.refresh());
  expect(getConnections).toHaveBeenLastCalledWith(expect.objectContaining({ from: 'carol-uid' }));
});
