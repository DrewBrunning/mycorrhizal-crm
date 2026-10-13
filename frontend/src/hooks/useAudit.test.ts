import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type AuditEvent,
  type AuditEventsResponse,
  getAuditEvents,
  undoAuditEvent,
} from '../api/audit';
import { useAudit } from './useAudit';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/audit', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/audit')>();
  return {
    ...actual,
    getAuditEvents: vi.fn(),
    undoAuditEvent: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getAuditEvents).mockReset();
  vi.mocked(undoAuditEvent).mockReset();
});

function event(id: number): AuditEvent {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    entity_type: 'contact',
    entity_id: 'uid-1',
    operation: 'update',
  };
}

function response(events: AuditEvent[]): AuditEventsResponse {
  return { audit_events: events, total: events.length };
}

test('loads events on mount with the default limit and no filters', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getAuditEvents).toHaveBeenCalledWith({
    entity_type: undefined,
    entity_id: undefined,
    limit: 100,
  });
  expect(result.current.events).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('starts in the loading state before the first response', () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([]));
  const { result } = renderHook(() => useAudit());
  expect(result.current.loading).toBe(true);
});

test('defaults to an empty list when the response omits audit_events', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue({} as AuditEventsResponse);

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.events).toEqual([]);
});

test('surfaces the fetch error', async () => {
  vi.mocked(getAuditEvents).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.events).toEqual([]);
});

test('clears a previous error on a successful refetch', async () => {
  vi.mocked(getAuditEvents).mockRejectedValueOnce(new Error('boom'));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.error).toBe('boom'));

  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));
  await act(async () => {
    await result.current.refetch();
  });

  expect(result.current.error).toBeNull();
  expect(result.current.events).toHaveLength(1);
});

test('applyEntityType sets the filter and restarts the window at the default limit', async () => {
  // A full window so loadMore actually grows the limit -- otherwise the reset
  // to DEFAULT_LIMIT is invisible and the assertion proves nothing.
  const full = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getAuditEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
  });
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 200 })),
  );

  await act(async () => {
    result.current.applyEntityType('note');
  });

  expect(result.current.entityType).toBe('note');
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ entity_type: 'note', limit: 100 }),
    ),
  );
});

test('applyEntityId trims the id, sets it and restarts the window', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.applyEntityId('  uid-9  ');
  });

  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ entity_id: 'uid-9' }),
    ),
  );
});

test('clearFilters resets the entity type, id and limit', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.applyEntityType('contact');
    result.current.applyEntityId('uid-1');
  });
  await act(async () => {
    result.current.clearFilters();
  });

  expect(result.current.entityType).toBe('');
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ entity_type: undefined, entity_id: undefined, limit: 100 }),
    ),
  );
});

test('canLoadMore is true only when the loaded count fills the window below the cap', async () => {
  const full = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getAuditEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.canLoadMore).toBe(true);
});

test('canLoadMore is false when fewer events than the window are loaded', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.canLoadMore).toBe(false);
});

test('loadMore grows the window in default steps, capped at 500', async () => {
  const full = Array.from({ length: 500 }, (_, i) => event(i + 1));
  vi.mocked(getAuditEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
  });
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 200 })),
  );

  for (let i = 0; i < 6; i++) {
    await act(async () => {
      result.current.loadMore();
    });
  }
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 500 })),
  );
});

test('loadMore is a no-op when the window is not full', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));
  const calls = vi.mocked(getAuditEvents).mock.calls.length;

  await act(async () => {
    result.current.loadMore();
  });

  expect(vi.mocked(getAuditEvents).mock.calls.length).toBe(calls);
});

test('canLoadMore is false once the cap is reached even with a full window', async () => {
  const full = Array.from({ length: 500 }, (_, i) => event(i + 1));
  vi.mocked(getAuditEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  for (let i = 0; i < 4; i++) {
    await act(async () => {
      result.current.loadMore();
    });
  }
  await waitFor(() =>
    expect(getAuditEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 500 })),
  );
  expect(result.current.canLoadMore).toBe(false);
});

test('a stale response that resolves after a newer one is ignored', async () => {
  const deferred: Array<(v: AuditEventsResponse) => void> = [];
  vi.mocked(getAuditEvents).mockImplementation(
    () => new Promise<AuditEventsResponse>((resolve) => deferred.push(resolve)),
  );

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(deferred.length).toBe(1));

  // Start a second fetch before the first resolves.
  await act(async () => {
    void result.current.refetch();
  });
  await waitFor(() => expect(deferred.length).toBe(2));

  // The newer request resolves first; the older one must not clobber it.
  await act(async () => {
    deferred[1](response([event(2)]));
  });
  await act(async () => {
    deferred[0](response([event(1)]));
  });

  expect(result.current.events).toEqual([event(2)]);
});

test('handleUndo undoes the event and refreshes the list', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));
  vi.mocked(undoAuditEvent).mockResolvedValue({ message: 'undone' });

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleUndo(7);
  });

  expect(undoAuditEvent).toHaveBeenCalledWith(7);
  expect(getAuditEvents).toHaveBeenCalledTimes(2);
});

test('handleUndo propagates an undo failure to the caller without refreshing', async () => {
  vi.mocked(getAuditEvents).mockResolvedValue(response([event(1)]));
  vi.mocked(undoAuditEvent).mockRejectedValue(new Error('410 gone'));

  const { result } = renderHook(() => useAudit());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleUndo(7)).rejects.toThrow('410 gone');
  expect(getAuditEvents).toHaveBeenCalledTimes(1);
});
