import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { ApiError } from '../api/client';
import { type Contact, getContactsByUid } from '../api/contacts';
import {
  createLifeEvent,
  deleteLifeEvent,
  getLifeEvents,
  type LifeEvent,
  type LifeEventInputData,
  updateLifeEvent,
} from '../api/lifeEvents';
import { useLifeEvents } from './useLifeEvents';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/lifeEvents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/lifeEvents')>();
  return {
    ...actual,
    getLifeEvents: vi.fn(),
    createLifeEvent: vi.fn(),
    updateLifeEvent: vi.fn(),
    deleteLifeEvent: vi.fn(),
  };
});

vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getLifeEvents).mockReset();
  vi.mocked(createLifeEvent).mockReset();
  vi.mocked(updateLifeEvent).mockReset();
  vi.mocked(deleteLifeEvent).mockReset();
  vi.mocked(getContactsByUid).mockReset();
});

const relatedContact: Contact = { ID: 2, firstname: 'Bob', lastname: 'Burns' };

function lifeEvent(id: string, overrides: Partial<LifeEvent> = {}): LifeEvent {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    type: 'moved',
    related_entity_ids: ['uid-2'],
    ...overrides,
  };
}

test('loads life events and the total on mount', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1')],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['uid-2', relatedContact]]));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getLifeEvents).toHaveBeenCalledWith({ entity_id: 'uid-1', limit: 50 });
  expect(result.current.events).toHaveLength(1);
  expect(result.current.total).toBe(1);
  expect(result.current.error).toBeNull();
});

test('resolves related contacts, excluding the entity itself', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { related_entity_ids: ['uid-1', 'uid-2', 'uid-2'] })],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['uid-2', relatedContact]]));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactsByUid).toHaveBeenCalledWith(['uid-2']);
  expect(result.current.contactsByUid.get('uid-2')).toEqual(relatedContact);
});

test('skips the contact lookup when nothing is related', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { related_entity_ids: [] })],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactsByUid).not.toHaveBeenCalled();
  expect(result.current.contactsByUid.size).toBe(0);
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => useLifeEvents(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getLifeEvents).not.toHaveBeenCalled();
  expect(result.current.events).toEqual([]);
});

test('handleCreate creates the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  vi.mocked(createLifeEvent).mockResolvedValue({
    message: 'created',
    life_event: lifeEvent('le-9'),
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const data: LifeEventInputData = { entity_id: 'uid-1', type: 'married' };
  await act(async () => {
    await result.current.handleCreate(data);
  });

  expect(createLifeEvent).toHaveBeenCalledWith(data);
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

test('handleUpdate updates the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { revision: 3 })],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const data: LifeEventInputData = {
    entity_id: 'uid-1',
    type: 'moved',
    description: 'to a new city',
  };
  await act(async () => {
    await result.current.handleUpdate('le-1', data, 3);
  });

  expect(updateLifeEvent).toHaveBeenCalledWith('le-1', data, 3);
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes the event and refreshes', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { revision: 3 })],
    next_cursor: '',
    limit: 50,
  });

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('le-1');
  });

  expect(deleteLifeEvent).toHaveBeenCalledWith('le-1', 3);
  expect(getLifeEvents).toHaveBeenCalledTimes(2);
});

// ADR 0008: a 412 calls onConflict. An update rethrows so the dialog stays open;
// a delete swallows it (the list click has no dialog to keep open).
test('handleUpdate answered 412 calls onConflict, rethrows and does not refresh', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  const stale = new ApiError('stale', 'PRECONDITION_FAILED', 412);
  vi.mocked(updateLifeEvent).mockRejectedValue(stale);
  const onConflict = vi.fn();

  const { result } = renderHook(() => useLifeEvents('uid-1', onConflict));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    act(async () => {
      await result.current.handleUpdate('le-1', { entity_id: 'uid-1', type: 'moved' }, 3);
    }),
  ).rejects.toBe(stale);
  expect(onConflict).toHaveBeenCalledTimes(1);
  expect(getLifeEvents).toHaveBeenCalledTimes(1);
});

test('handleUpdate rethrows a non-412 error without calling onConflict', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  vi.mocked(updateLifeEvent).mockRejectedValue(new Error('boom'));
  const onConflict = vi.fn();

  const { result } = renderHook(() => useLifeEvents('uid-1', onConflict));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    act(async () => {
      await result.current.handleUpdate('le-1', { entity_id: 'uid-1', type: 'moved' }, 3);
    }),
  ).rejects.toThrow('boom');
  expect(onConflict).not.toHaveBeenCalled();
});

test('handleDelete answered 412 calls onConflict and does not refresh or throw', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [lifeEvent('le-1', { revision: 3 })],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(deleteLifeEvent).mockRejectedValue(new ApiError('stale', 'PRECONDITION_FAILED', 412));
  const onConflict = vi.fn();

  const { result } = renderHook(() => useLifeEvents('uid-1', onConflict));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('le-1');
  });
  expect(onConflict).toHaveBeenCalledTimes(1);
  expect(getLifeEvents).toHaveBeenCalledTimes(1);
});

test('handleDelete rethrows a non-412 error', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  vi.mocked(deleteLifeEvent).mockRejectedValue(new Error('boom'));
  const { result } = renderHook(() => useLifeEvents('uid-1', vi.fn()));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    act(async () => {
      await result.current.handleDelete('le-1');
    }),
  ).rejects.toThrow('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getLifeEvents).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.events).toEqual([]);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

type LifePage = Awaited<ReturnType<typeof getLifeEvents>>;

test('starts empty, reports loading, logs a failure by operation, and a refresh clears it', async () => {
  const d = deferred<LifePage>();
  vi.mocked(getLifeEvents).mockReturnValueOnce(d.promise);
  const { result } = renderHook(() => useLifeEvents('uid-1'));
  expect(result.current.events).toEqual([]);
  expect(result.current.total).toBe(0);
  expect(result.current.contactsByUid.size).toBe(0);
  await waitFor(() => expect(result.current.loading).toBe(true));
  await act(async () => {
    d.resolve({ life_events: [], next_cursor: '', limit: 50 });
  });
  await waitFor(() => expect(result.current.loading).toBe(false));

  vi.mocked(getLifeEvents).mockRejectedValueOnce(new Error('offline'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('offline');
  expect(console.error).toHaveBeenCalledWith('[fetching life events] Error:', expect.anything());

  const d2 = deferred<LifePage>();
  vi.mocked(getLifeEvents).mockReturnValueOnce(d2.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d2.resolve({ life_events: [], next_cursor: '', limit: 50 });
  });
});

test('a response without life_events yields none; an override entity id wins', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({} as LifePage);
  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.events).toEqual([]);
  await act(async () => {
    await result.current.refresh('uid-9');
  });
  expect(getLifeEvents).toHaveBeenLastCalledWith({ entity_id: 'uid-9', limit: 50 });
});

test('an event with no related ids is skipped, and related ids from every event are collected once', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({
    life_events: [
      lifeEvent('a', { related_entity_ids: undefined }),
      lifeEvent('b', { related_entity_ids: ['uid-2', 'uid-3'] }),
      lifeEvent('c', { related_entity_ids: ['uid-3', 'uid-4'] }),
    ],
    next_cursor: '',
    limit: 50,
  });
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());
  const { result } = renderHook(() => useLifeEvents('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(getContactsByUid).toHaveBeenCalledWith(['uid-2', 'uid-3', 'uid-4']);
});

test('refresh and the mutations follow the current entity id', async () => {
  vi.mocked(getLifeEvents).mockResolvedValue({ life_events: [], next_cursor: '', limit: 50 });
  vi.mocked(createLifeEvent).mockResolvedValue({ message: '', life_event: lifeEvent('n') });
  const { result, rerender } = renderHook(({ id }) => useLifeEvents(id), {
    initialProps: { id: 'uid-1' },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ id: 'uid-9' });
  await waitFor(() =>
    expect(getLifeEvents).toHaveBeenLastCalledWith({ entity_id: 'uid-9', limit: 50 }),
  );
  vi.mocked(getLifeEvents).mockClear();
  await act(async () => {
    await result.current.handleCreate({} as LifeEventInputData);
    await result.current.handleUpdate('x', {} as LifeEventInputData);
    await result.current.handleDelete('x');
  });
  expect(getLifeEvents).toHaveBeenCalledTimes(3);
  expect(getLifeEvents).toHaveBeenLastCalledWith({ entity_id: 'uid-9', limit: 50 });
});
