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
