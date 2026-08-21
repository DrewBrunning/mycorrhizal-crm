import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useGraph } from './useGraph';
import { getGraph } from '../api/graph';
import { GraphResponse } from '../types/graph';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  localStorage.clear();
  vi.restoreAllMocks();
});

vi.mock('../api/graph', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/graph')>();
  return { ...actual, getGraph: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getGraph).mockReset();
});

const graphResponse: GraphResponse = {
  nodes: [
    { id: 'c-1', type: 'contact', label: 'Alice' },
    { id: 'c-2', type: 'contact', label: 'Bob' },
  ],
  edges: [{ id: 'e-1', source: 'c-1', target: 'c-2', type: 'relationship', label: 'friend_of' }],
};

test('loads graph data on mount', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockResolvedValue(graphResponse);

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getGraph).toHaveBeenCalledTimes(1);
  expect(result.current.data).toEqual(graphResponse);
  expect(result.current.error).toBeNull();
});

test('refetch re-runs the request', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockResolvedValue(graphResponse);

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refetch();
  });

  expect(getGraph).toHaveBeenCalledTimes(2);
});

test('sets an auth error when not authenticated', async () => {
  localStorage.clear();

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('No authentication token found');
  expect(result.current.data).toBeNull();
  expect(getGraph).not.toHaveBeenCalled();
});

test('sets error when the fetch fails', async () => {
  localStorage.setItem('user_info', JSON.stringify({ user_id: 1 }));
  vi.mocked(getGraph).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useGraph());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.data).toBeNull();
});
