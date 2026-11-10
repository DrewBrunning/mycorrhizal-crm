import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, expect, test, vi } from 'vitest';
import { type Entity, listEntities } from '../api/entities';
import { useEntityList } from './useEntityList';

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

vi.mock('../api/entities', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/entities')>();
  return { ...actual, listEntities: vi.fn() };
});
const listMock = vi.mocked(listEntities);

const ent = (id: string, name = id): Entity => ({
  id,
  kind: 'organisation',
  name,
  core: {},
  sensitivity: 'normal',
  revision: 1,
  created_at: '',
  updated_at: '',
});

test('loads the first page for a kind and appends the next on loadMore', async () => {
  listMock
    .mockResolvedValueOnce({ entities: [ent('a')], limit: 25, next_cursor: 'c1' })
    .mockResolvedValueOnce({ entities: [ent('b')], limit: 25, next_cursor: '' });
  const { result } = renderHook(() => useEntityList('organisation', ''));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.entities.map((e) => e.id)).toEqual(['a']);
  expect(result.current.hasMore).toBe(true);

  await act(async () => {
    await result.current.loadMore();
  });
  expect(listMock).toHaveBeenLastCalledWith(expect.objectContaining({ cursor: 'c1' }));
  expect(result.current.entities.map((e) => e.id)).toEqual(['a', 'b']);
  expect(result.current.hasMore).toBe(false);
});

test('passes kind and the trimmed query, and refetches when they change', async () => {
  listMock.mockResolvedValue({ entities: [], limit: 25, next_cursor: '' });
  const { rerender } = renderHook(({ kind, q }) => useEntityList(kind, q), {
    initialProps: { kind: 'organisation' as const, q: ' ac ' } as {
      kind: 'organisation' | 'place';
      q: string;
    },
  });
  await waitFor(() => expect(listMock).toHaveBeenCalledTimes(1));
  expect(listMock).toHaveBeenLastCalledWith(
    expect.objectContaining({ kind: 'organisation', q: 'ac' }),
  );
  rerender({ kind: 'place', q: '' });
  await waitFor(() => expect(listMock).toHaveBeenCalledTimes(2));
  expect(listMock).toHaveBeenLastCalledWith(expect.objectContaining({ kind: 'place' }));
});

test('a slow response for a previous kind never overwrites the current one', async () => {
  let resolveSlow: (v: Awaited<ReturnType<typeof listEntities>>) => void = () => {};
  listMock.mockImplementationOnce(
    () =>
      new Promise((r) => {
        resolveSlow = r;
      }),
  );
  listMock.mockResolvedValueOnce({ entities: [ent('place1')], limit: 25, next_cursor: '' });
  const { result, rerender } = renderHook(({ kind }) => useEntityList(kind, ''), {
    initialProps: { kind: 'organisation' as 'organisation' | 'place' },
  });
  rerender({ kind: 'place' });
  await waitFor(() => expect(result.current.entities.map((e) => e.id)).toEqual(['place1']));
  await act(async () => {
    resolveSlow({ entities: [ent('stale')], limit: 25, next_cursor: '' });
  });
  expect(result.current.entities.map((e) => e.id)).toEqual(['place1']);
});

test('a failed load exposes the error message and retry reloads', async () => {
  listMock.mockRejectedValueOnce(new Error('boom'));
  listMock.mockResolvedValueOnce({ entities: [ent('a')], limit: 25, next_cursor: '' });
  const { result } = renderHook(() => useEntityList('organisation', ''));
  await waitFor(() => expect(result.current.error).toBeTruthy());
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBeNull();
  expect(result.current.entities).toHaveLength(1);
});
