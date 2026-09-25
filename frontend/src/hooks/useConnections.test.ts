import { act, cleanup, renderHook } from '@testing-library/react';
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
