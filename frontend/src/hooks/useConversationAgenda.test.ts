import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type ConversationAgenda,
  type ConversationAgendaListResponse,
  createConversationAgenda,
  deleteConversationAgenda,
  discussConversationAgenda,
  getConversationAgenda,
  updateConversationAgenda,
} from '../api/conversationAgenda';
import { useConversationAgenda } from './useConversationAgenda';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/conversationAgenda', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/conversationAgenda')>();
  return {
    ...actual,
    getConversationAgenda: vi.fn(),
    createConversationAgenda: vi.fn(),
    updateConversationAgenda: vi.fn(),
    discussConversationAgenda: vi.fn(),
    deleteConversationAgenda: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getConversationAgenda).mockReset();
  vi.mocked(createConversationAgenda).mockReset();
  vi.mocked(updateConversationAgenda).mockReset();
  vi.mocked(discussConversationAgenda).mockReset();
  vi.mocked(deleteConversationAgenda).mockReset();
});

function item(id: string, overrides: Partial<ConversationAgenda> = {}): ConversationAgenda {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    content: `item ${id}`,
    ...overrides,
  };
}

function listResponse(items: ConversationAgenda[]): ConversationAgendaListResponse {
  return { conversation_agenda: items, next_cursor: '', limit: 100 };
}

test('loads the agenda', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(listResponse([item('a-1')]));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(getConversationAgenda).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 100 });
  expect(result.current.items).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('splits open and discussed items', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(
    listResponse([
      item('a-1'),
      item('a-2', { discussed_at: '2026-01-02T00:00:00Z', activity_id: 7 }),
    ]),
  );

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.openItems.map((i) => i.id)).toEqual(['a-1']);
  expect(result.current.discussedItems.map((i) => i.id)).toEqual(['a-2']);
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => useConversationAgenda(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getConversationAgenda).not.toHaveBeenCalled();
  expect(result.current.items).toEqual([]);
});

test('handleCreate creates the item and refreshes', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(listResponse([]));
  vi.mocked(createConversationAgenda).mockResolvedValue(item('a-9'));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  const data = { entity_id: 'uid-1', content: 'bring up the trip' };
  await act(async () => {
    await result.current.handleCreate(data);
  });

  expect(createConversationAgenda).toHaveBeenCalledWith(data);
  expect(getConversationAgenda).toHaveBeenCalledTimes(2);
});

test('handleUpdate updates the item and refreshes', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(listResponse([item('a-1')]));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const data = { entity_id: 'uid-1', content: 'updated', reference_url: 'https://x.test' };
  await act(async () => {
    await result.current.handleUpdate('a-1', data);
  });

  expect(updateConversationAgenda).toHaveBeenCalledWith('a-1', data);
});

test('handleDiscuss marks an item discussed with an optional activity link', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(listResponse([item('a-1')]));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDiscuss('a-1', 7);
  });

  expect(discussConversationAgenda).toHaveBeenCalledWith('a-1', 7);
});

test('handleDelete deletes the item and refreshes', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue(listResponse([item('a-1')]));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  await act(async () => {
    await result.current.handleDelete('a-1');
  });

  expect(deleteConversationAgenda).toHaveBeenCalledWith('a-1');
  expect(getConversationAgenda).toHaveBeenCalledTimes(2);
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getConversationAgenda).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useConversationAgenda('uid-1'));
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

test('starts empty and idle; refresh asks for up to 100 items of the entity and reports loading', async () => {
  const d = deferred<ConversationAgendaListResponse>();
  vi.mocked(getConversationAgenda).mockReturnValue(d.promise);
  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  expect(result.current.items).toEqual([]);
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getConversationAgenda).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 100 });
  await act(async () => {
    d.resolve(listResponse([item('a-1')]));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
});

test('an override entity id wins; a response without the list yields none', async () => {
  vi.mocked(getConversationAgenda).mockResolvedValue({} as ConversationAgendaListResponse);
  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh('uid-2');
  });
  expect(getConversationAgenda).toHaveBeenCalledWith({ entityId: 'uid-2', limit: 100 });
  expect(result.current.items).toEqual([]);
});

test('a failed refresh is logged by operation and the next refresh clears the error', async () => {
  vi.mocked(getConversationAgenda).mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useConversationAgenda('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('offline');
  expect(result.current.loading).toBe(false);
  expect(console.error).toHaveBeenCalledWith(
    '[fetching conversation agenda] Error:',
    expect.anything(),
  );
  vi.mocked(getConversationAgenda).mockResolvedValueOnce(listResponse([]));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBeNull();
});

test('open and discussed lists follow the items, and handlers follow the current entity id', async () => {
  vi.mocked(getConversationAgenda)
    .mockResolvedValueOnce(listResponse([item('a', { discussed_at: undefined })]))
    .mockResolvedValue(listResponse([item('a', { discussed_at: '2026-02-01T00:00:00Z' })]));
  vi.mocked(createConversationAgenda).mockResolvedValue(item('n') as never);
  const { result, rerender } = renderHook(({ id }) => useConversationAgenda(id), {
    initialProps: { id: 'uid-1' },
  });
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.openItems).toHaveLength(1);
  expect(result.current.discussedItems).toHaveLength(0);

  rerender({ id: 'uid-9' });
  await act(async () => {
    await result.current.handleCreate({ entity_id: 'uid-9', content: 'x' });
    await result.current.handleUpdate('a', { entity_id: 'uid-9', content: 'y' });
    await result.current.handleDiscuss('a');
    await result.current.handleDelete('a');
  });
  expect(getConversationAgenda).toHaveBeenLastCalledWith({ entityId: 'uid-9', limit: 100 });
  expect(result.current.openItems).toHaveLength(0);
  expect(result.current.discussedItems).toHaveLength(1);
});
