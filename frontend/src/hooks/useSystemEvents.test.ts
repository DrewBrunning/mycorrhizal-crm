import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { getSystemEvents, type SystemEvent, type SystemEventsResponse } from '../api/systemEvents';
import { useSystemEvents } from './useSystemEvents';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/systemEvents', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/systemEvents')>();
  return {
    ...actual,
    getSystemEvents: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getSystemEvents).mockReset();
});

function event(id: number): SystemEvent {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    occurred_at: '2026-01-01T00:00:00Z',
    event_type: 'job_completed',
    severity: 'info',
    component: 'scheduler',
    correlation_id: 'c-1',
  };
}

function response(events: SystemEvent[]): SystemEventsResponse {
  return { system_events: events, total: events.length };
}

test('loads events on mount with the default limit and no optional filters', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([event(1)]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getSystemEvents).toHaveBeenCalledWith({
    component: undefined,
    severity: undefined,
    event_type: undefined,
    correlation_id: undefined,
    ids: undefined,
    limit: 100,
  });
  expect(result.current.events).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('defaults to an empty list when the response omits system_events', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue({} as SystemEventsResponse);

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.events).toEqual([]);
});

test('surfaces the fetch error', async () => {
  vi.mocked(getSystemEvents).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.events).toEqual([]);
});

test('patchFilters merges a patch onto the existing filters and resets the limit', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.patchFilters({ component: 'app' });
  });
  await act(async () => {
    result.current.patchFilters({ severity: 'warn' });
  });

  expect(result.current.filters.component).toBe('app');
  expect(result.current.filters.severity).toBe('warn');
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ component: 'app', severity: 'warn', limit: 100 }),
    ),
  );
});

test('patchFilters clears a previous ids drill-down unless the patch sets ids', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.showErrors([4, 5]);
  });
  expect(result.current.filters.ids).toEqual([4, 5]);

  await act(async () => {
    result.current.patchFilters({ component: 'backup' });
  });
  expect(result.current.filters.ids).toEqual([]);

  await act(async () => {
    result.current.patchFilters({ ids: [9] });
  });
  expect(result.current.filters.ids).toEqual([9]);
});

test('patchFilters resets the window to the default limit', async () => {
  // A full window so loadMore actually grows the limit -- otherwise the reset
  // to DEFAULT_LIMIT is invisible and the assertion proves nothing.
  const full = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getSystemEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 200 })),
  );

  await act(async () => {
    result.current.patchFilters({ component: 'app' });
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 100 })),
  );
});

test('clearFilters resets every filter and the limit', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.patchFilters({ component: 'app', severity: 'error' });
  });
  await act(async () => {
    result.current.clearFilters();
  });

  expect(result.current.filters).toEqual({
    component: '',
    severity: '',
    eventType: '',
    correlationId: '',
    ids: [],
  });
});

test('showRelated sets only the correlation id and jumps to the max window', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.patchFilters({ component: 'app' });
  });
  await act(async () => {
    result.current.showRelated('c-42');
  });

  expect(result.current.filters).toEqual({
    component: '',
    severity: '',
    eventType: '',
    correlationId: 'c-42',
    ids: [],
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ correlation_id: 'c-42', limit: 500 }),
    ),
  );
});

test('showErrors sets exactly the ids and jumps to the max window', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.showErrors([7, 8]);
  });

  expect(result.current.filters.ids).toEqual([7, 8]);
  expect(result.current.filters.component).toBe('');
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ ids: [7, 8], limit: 500 }),
    ),
  );
});

test('trims the correlation id and omits blank filters from the request', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.patchFilters({ correlationId: '  c-9  ', component: '' });
  });

  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(
      expect.objectContaining({ correlation_id: 'c-9', component: undefined }),
    ),
  );
});

test('hasFilters is true only when at least one filter is set', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.hasFilters).toBe(false);

  await act(async () => {
    result.current.patchFilters({ component: 'app' });
  });
  expect(result.current.hasFilters).toBe(true);
});

test('a whitespace-only correlation id does not count as a filter', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.patchFilters({ correlationId: '   ' });
  });
  expect(result.current.hasFilters).toBe(false);
});

test('loadMore grows the window by the default step, capped at 500', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    result.current.loadMore();
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 200 })),
  );

  // Drive to the cap; further loadMore calls must not exceed it.
  for (let i = 0; i < 5; i++) {
    await act(async () => {
      result.current.loadMore();
    });
  }
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 500 })),
  );
});

test('canLoadMore is true only once the loaded count fills the window below the cap', async () => {
  const full = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getSystemEvents).mockResolvedValue(response(full));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.canLoadMore).toBe(true);
});

test('canLoadMore is false when fewer events than the window are loaded', async () => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([event(1)]));

  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.canLoadMore).toBe(false);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

test.each([
  ['component', { component: 'app' }],
  ['severity', { severity: 'warn' as const }],
  ['event type', { eventType: 'job_completed' as const }],
  ['correlation id', { correlationId: 'c-1' }],
  ['ids', { ids: [1] }],
])('hasFilters is true when only the %s filter is set', async (_name, patch) => {
  vi.mocked(getSystemEvents).mockResolvedValue(response([]));
  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    result.current.patchFilters(patch);
  });
  expect(result.current.hasFilters).toBe(true);
});

test('canLoadMore: a window filled exactly counts, one short does not, and the cap closes it', async () => {
  const exactly = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getSystemEvents).mockResolvedValue(response(exactly.slice(0, 99)));
  const short = renderHook(() => useSystemEvents());
  await waitFor(() => expect(short.result.current.loading).toBe(false));
  expect(short.result.current.canLoadMore).toBe(false);
  cleanup();

  const atCap = Array.from({ length: 500 }, (_, i) => event(i + 1));
  vi.mocked(getSystemEvents).mockResolvedValue(response(atCap));
  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    result.current.showRelated('c-1'); // jumps to the 500 window
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 500 })),
  );
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.events).toHaveLength(500);
  expect(result.current.canLoadMore).toBe(false);
});

test('clearFilters returns the window to the default limit', async () => {
  const full = Array.from({ length: 100 }, (_, i) => event(i + 1));
  vi.mocked(getSystemEvents).mockResolvedValue(response(full));
  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.loading).toBe(false));
  await act(async () => {
    result.current.loadMore();
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 200 })),
  );
  await act(async () => {
    result.current.clearFilters();
  });
  await waitFor(() =>
    expect(getSystemEvents).toHaveBeenLastCalledWith(expect.objectContaining({ limit: 100 })),
  );
});

test('a refetch clears the error while loading, and the failure is logged by operation', async () => {
  vi.mocked(getSystemEvents).mockRejectedValueOnce(new Error('down'));
  const { result } = renderHook(() => useSystemEvents());
  await waitFor(() => expect(result.current.error).toBe('down'));
  expect(console.error).toHaveBeenCalledWith('[fetching system events] Error:', expect.anything());

  const d = deferred<SystemEventsResponse>();
  vi.mocked(getSystemEvents).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(response([]));
  });
});

test('a superseded fetch is ignored whether it succeeds or fails, and does not end loading', async () => {
  const stale = deferred<SystemEventsResponse>();
  const fresh = deferred<SystemEventsResponse>();
  vi.mocked(getSystemEvents).mockReturnValueOnce(stale.promise).mockReturnValueOnce(fresh.promise);
  const { result } = renderHook(() => useSystemEvents());
  act(() => {
    void result.current.refetch();
  });
  await waitFor(() => expect(getSystemEvents).toHaveBeenCalledTimes(2));

  await act(async () => {
    stale.resolve(response([event(1)]));
  });
  expect(result.current.events).toEqual([]);
  expect(result.current.loading).toBe(true);
  await act(async () => {
    fresh.resolve(response([event(2)]));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.events.map((e) => e.id)).toEqual([2]);

  const stale2 = deferred<SystemEventsResponse>();
  const fresh2 = deferred<SystemEventsResponse>();
  vi.mocked(getSystemEvents)
    .mockReturnValueOnce(stale2.promise)
    .mockReturnValueOnce(fresh2.promise);
  act(() => {
    void result.current.refetch();
  });
  act(() => {
    void result.current.refetch();
  });
  await act(async () => {
    stale2.reject(new Error('old failure'));
  });
  expect(result.current.error).toBeNull();
  expect(result.current.loading).toBe(true);
  await act(async () => {
    fresh2.resolve(response([event(3)]));
  });
});
