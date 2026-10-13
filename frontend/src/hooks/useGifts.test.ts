import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createGift,
  deleteGift,
  type Gift,
  type GiftInput,
  type GiftListResponse,
  getGifts,
  updateGift,
} from '../api/gifts';
import { useGifts } from './useGifts';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/gifts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/gifts')>();
  return {
    ...actual,
    getGifts: vi.fn(),
    createGift: vi.fn(),
    updateGift: vi.fn(),
    deleteGift: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getGifts).mockReset();
  vi.mocked(createGift).mockReset();
  vi.mocked(updateGift).mockReset();
  vi.mocked(deleteGift).mockReset();
});

function gift(id: string, overrides: Partial<Gift> = {}): Gift {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    status: 'idea',
    description: `gift ${id}`,
    ...overrides,
  };
}

function listResponse(gifts: Gift[]): GiftListResponse {
  return { gifts, next_cursor: '', limit: 100 };
}

test('loads gifts', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([gift('g-1')]));

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(getGifts).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 100 });
  expect(result.current.items).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('splits ideas from resolved gifts', async () => {
  vi.mocked(getGifts).mockResolvedValue(
    listResponse([
      gift('g-idea', { status: 'idea' }),
      gift('g-given', { status: 'given', date: '2026-01-01T00:00:00Z' }),
      gift('g-received', { status: 'received' }),
    ]),
  );

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.ideas.map((g) => g.id)).toEqual(['g-idea']);
  expect(result.current.resolved.map((g) => g.id)).toEqual(['g-given', 'g-received']);
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => useGifts(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getGifts).not.toHaveBeenCalled();
  expect(result.current.items).toEqual([]);
});

test('handleCreate creates the gift and refreshes', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([]));
  vi.mocked(createGift).mockResolvedValue(gift('g-9'));

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  const input: GiftInput = { entity_id: 'uid-1', description: 'a nice pen' };
  await act(async () => {
    await result.current.handleCreate(input);
  });

  expect(createGift).toHaveBeenCalledWith(input);
  expect(getGifts).toHaveBeenCalledTimes(2);
});

test('handleUpdate updates the gift and refreshes', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([gift('g-1')]));

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  const input: GiftInput = { entity_id: 'uid-1', description: 'updated', status: 'given' };
  await act(async () => {
    await result.current.handleUpdate('g-1', input);
  });

  expect(updateGift).toHaveBeenCalledWith('g-1', input);
  expect(getGifts).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes the gift and refreshes', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([gift('g-1')]));

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  await act(async () => {
    await result.current.handleDelete('g-1');
  });

  expect(deleteGift).toHaveBeenCalledWith('g-1');
  expect(getGifts).toHaveBeenCalledTimes(2);
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getGifts).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.items).toEqual([]);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty and idle; refresh asks for up to 100 gifts of the entity and reports loading', async () => {
  const d = deferred<GiftListResponse>();
  vi.mocked(getGifts).mockReturnValue(d.promise);
  const { result } = renderHook(() => useGifts('uid-1'));
  expect(result.current.items).toEqual([]);
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getGifts).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 100 });
  await act(async () => {
    d.resolve(listResponse([gift('g-1')]));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.items).toHaveLength(1);
});

test('an override entity id wins over the hook entity id', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([]));
  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh('uid-2');
  });
  expect(getGifts).toHaveBeenCalledWith({ entityId: 'uid-2', limit: 100 });
});

test('a response without gifts yields none; a failure is logged then cleared by the next refresh', async () => {
  vi.mocked(getGifts).mockResolvedValueOnce({} as GiftListResponse);
  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.items).toEqual([]);

  vi.mocked(getGifts).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('offline');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith('[fetching gifts] Error:', expect.anything());

  vi.mocked(getGifts).mockResolvedValueOnce(listResponse([]));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBeNull();
});

test('refresh and the mutation handlers follow the current entity id', async () => {
  vi.mocked(getGifts).mockResolvedValue(listResponse([]));
  vi.mocked(createGift).mockResolvedValue(gift('n'));
  const { result, rerender } = renderHook(({ id }) => useGifts(id), {
    initialProps: { id: 'uid-1' as string | undefined },
  });
  rerender({ id: 'uid-9' });
  await act(async () => {
    await result.current.handleCreate({} as GiftInput);
    await result.current.handleUpdate('g', {} as GiftInput);
    await result.current.handleDelete('g');
  });
  expect(getGifts).toHaveBeenCalledTimes(3);
  expect(getGifts).toHaveBeenLastCalledWith({ entityId: 'uid-9', limit: 100 });
});

test('ideas and resolved update when the items change', async () => {
  vi.mocked(getGifts)
    .mockResolvedValueOnce(listResponse([gift('g-1', { status: 'idea' })]))
    .mockResolvedValueOnce(listResponse([gift('g-1', { status: 'given' })]));
  const { result } = renderHook(() => useGifts('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.ideas).toHaveLength(1);
  expect(result.current.resolved).toHaveLength(0);
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.ideas).toHaveLength(0);
  expect(result.current.resolved).toHaveLength(1);
});
