// Behavior pins for the load-once admin snapshot hooks (issue #1662): they
// share one shape — a request-id guard against stale responses, a loading flag
// that only the newest request may clear, and an error string that a refresh
// resets — so one parameterized suite exercises all of them.
import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { runDiagnostics } from '../api/diagnostics';
import { getErrorAggregation } from '../api/errorAggregation';
import { getNotificationChannelHealth } from '../api/notificationHealth';
import { getSubsystemHealth } from '../api/subsystemHealth';
import { getSystemStatus } from '../api/systemStatus';
import { useDiagnostics } from './useDiagnostics';
import { useErrorAggregation } from './useErrorAggregation';
import { useNotificationChannelHealth } from './useNotificationChannelHealth';
import { useSubsystemHealth } from './useSubsystemHealth';
import { useSystemStatus } from './useSystemStatus';

vi.mock('../api/diagnostics', () => ({ runDiagnostics: vi.fn() }));
vi.mock('../api/errorAggregation', () => ({ getErrorAggregation: vi.fn() }));
vi.mock('../api/notificationHealth', () => ({ getNotificationChannelHealth: vi.fn() }));
vi.mock('../api/subsystemHealth', () => ({ getSubsystemHealth: vi.fn() }));
vi.mock('../api/systemStatus', () => ({ getSystemStatus: vi.fn() }));

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  for (const c of cases) c.api.mockReset();
});

interface Snapshot {
  loading: boolean;
  error: string | null;
  value: unknown;
}

interface Case {
  name: string;
  api: ReturnType<typeof vi.fn>;
  // Hook under test, adapted to a uniform { snapshot, trigger } surface.
  use: () => { snapshot: Snapshot; trigger: () => Promise<void> };
  // Response whose payload is identified by `tag`, and the expected value.
  response: (tag: string) => unknown;
  expected: (tag: string) => unknown;
  autoLoads: boolean;
  // The operation label handleFetchError logs on failure.
  operation: string;
}

const cases: Case[] = [
  {
    name: 'useSystemStatus',
    api: vi.mocked(getSystemStatus) as unknown as ReturnType<typeof vi.fn>,
    use: () => {
      const r = useSystemStatus();
      return {
        snapshot: { loading: r.loading, error: r.error, value: r.data },
        trigger: r.refresh,
      };
    },
    response: (tag) => ({ tag }),
    expected: (tag) => ({ tag }),
    autoLoads: true,
    operation: 'fetching system status',
  },
  {
    name: 'useSubsystemHealth',
    api: vi.mocked(getSubsystemHealth) as unknown as ReturnType<typeof vi.fn>,
    use: () => {
      const r = useSubsystemHealth();
      return {
        snapshot: { loading: r.loading, error: r.error, value: r.data },
        trigger: r.refresh,
      };
    },
    response: (tag) => ({ subsystems: [{ tag }] }),
    expected: (tag) => [{ tag }],
    autoLoads: true,
    operation: 'fetching subsystem health',
  },
  {
    name: 'useNotificationChannelHealth',
    api: vi.mocked(getNotificationChannelHealth) as unknown as ReturnType<typeof vi.fn>,
    use: () => {
      const r = useNotificationChannelHealth();
      return {
        snapshot: { loading: r.loading, error: r.error, value: r.data },
        trigger: r.refresh,
      };
    },
    response: (tag) => ({ channels: [{ tag }] }),
    expected: (tag) => [{ tag }],
    autoLoads: true,
    operation: 'fetching notification channel health',
  },
  {
    name: 'useErrorAggregation',
    api: vi.mocked(getErrorAggregation) as unknown as ReturnType<typeof vi.fn>,
    use: () => {
      const r = useErrorAggregation();
      return {
        snapshot: { loading: r.loading, error: r.error, value: r.buckets },
        trigger: r.refresh,
      };
    },
    response: (tag) => ({ buckets: [{ tag }], total_events: 3 }),
    expected: (tag) => [{ tag }],
    autoLoads: true,
    operation: 'fetching error aggregation',
  },
  {
    name: 'useDiagnostics',
    api: vi.mocked(runDiagnostics) as unknown as ReturnType<typeof vi.fn>,
    use: () => {
      const r = useDiagnostics();
      return {
        snapshot: { loading: r.loading, error: r.error, value: r.data },
        trigger: r.run,
      };
    },
    response: (tag) => ({ tag }),
    expected: (tag) => ({ tag }),
    autoLoads: false,
    operation: 'running diagnostics',
  },
];

function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe.each(cases)('$name', (c) => {
  beforeEach(() => {
    c.api.mockReset();
  });

  test('settles with the response, clears loading, and reports no error', async () => {
    c.api.mockResolvedValue(c.response('a'));
    const { result } = renderHook(() => c.use());
    if (!c.autoLoads) {
      expect(c.api).not.toHaveBeenCalled();
      expect(result.current.snapshot.loading).toBe(false);
      await act(async () => {
        await result.current.trigger();
      });
    }
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
    expect(result.current.snapshot.value).toEqual(c.expected('a'));
    expect(result.current.snapshot.error).toBeNull();
  });

  test('reports loading while a request is in flight', async () => {
    const d = deferred<unknown>();
    c.api.mockReturnValue(d.promise);
    const { result } = renderHook(() => c.use());
    if (!c.autoLoads) {
      act(() => {
        void result.current.trigger();
      });
    }
    await waitFor(() => expect(result.current.snapshot.loading).toBe(true));
    await act(async () => {
      d.resolve(c.response('a'));
    });
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
  });

  test('surfaces the failure message and clears loading', async () => {
    c.api.mockRejectedValue(new Error('boom'));
    const { result } = renderHook(() => c.use());
    if (!c.autoLoads) {
      await act(async () => {
        await result.current.trigger();
      });
    }
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
    expect(result.current.snapshot.error).toBe('boom');
    expect(console.error).toHaveBeenCalledWith(`[${c.operation}] Error:`, expect.anything());
  });

  test('a refresh resets the previous error before the new response lands', async () => {
    c.api.mockRejectedValueOnce(new Error('boom'));
    const { result } = renderHook(() => c.use());
    if (!c.autoLoads) {
      await act(async () => {
        await result.current.trigger();
      });
    }
    await waitFor(() => expect(result.current.snapshot.error).toBe('boom'));

    const d = deferred<unknown>();
    c.api.mockReturnValueOnce(d.promise);
    act(() => {
      void result.current.trigger();
    });
    await waitFor(() => expect(result.current.snapshot.loading).toBe(true));
    expect(result.current.snapshot.error).toBeNull();
    await act(async () => {
      d.resolve(c.response('b'));
    });
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
    expect(result.current.snapshot.value).toEqual(c.expected('b'));
    expect(result.current.snapshot.error).toBeNull();
  });

  test('a superseded response neither overwrites the newer one nor clears loading', async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    // The auto-load (or first manual trigger) takes `first`; the next call takes `second`.
    c.api.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => c.use());
    act(() => {
      void result.current.trigger();
    });
    if (!c.autoLoads) {
      act(() => {
        void result.current.trigger();
      });
    }
    await waitFor(() => expect(c.api).toHaveBeenCalledTimes(2));

    // The newer request resolves first...
    await act(async () => {
      second.resolve(c.response('new'));
    });
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
    expect(result.current.snapshot.value).toEqual(c.expected('new'));

    // ...then the stale one lands and must be ignored entirely.
    await act(async () => {
      first.resolve(c.response('old'));
    });
    expect(result.current.snapshot.value).toEqual(c.expected('new'));
    expect(result.current.snapshot.loading).toBe(false);
  });

  test('a superseded failure neither sets an error nor clears loading of the newer request', async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    c.api.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => c.use());
    act(() => {
      void result.current.trigger();
    });
    if (!c.autoLoads) {
      act(() => {
        void result.current.trigger();
      });
    }
    await waitFor(() => expect(c.api).toHaveBeenCalledTimes(2));

    await act(async () => {
      first.reject(new Error('stale-boom'));
    });
    expect(result.current.snapshot.error).toBeNull();
    expect(result.current.snapshot.loading).toBe(true);

    await act(async () => {
      second.resolve(c.response('new'));
    });
    await waitFor(() => expect(result.current.snapshot.loading).toBe(false));
    expect(result.current.snapshot.value).toEqual(c.expected('new'));
    expect(result.current.snapshot.error).toBeNull();
  });

  test('a superseded request resolving last does not clear loading early', async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    c.api.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => c.use());
    act(() => {
      void result.current.trigger();
    });
    if (!c.autoLoads) {
      act(() => {
        void result.current.trigger();
      });
    }
    await waitFor(() => expect(c.api).toHaveBeenCalledTimes(2));

    await act(async () => {
      first.resolve(c.response('old'));
    });
    expect(result.current.snapshot.loading).toBe(true);
    expect(result.current.snapshot.value).not.toEqual(c.expected('old'));
  });
});

describe('fallbacks and parameters', () => {
  test('list-shaped hooks start with an empty list before the first response lands', () => {
    vi.mocked(getSubsystemHealth).mockReturnValue(new Promise(() => {}));
    vi.mocked(getNotificationChannelHealth).mockReturnValue(new Promise(() => {}));
    expect(renderHook(() => useSubsystemHealth()).result.current.data).toEqual([]);
    expect(renderHook(() => useNotificationChannelHealth()).result.current.data).toEqual([]);
  });

  test('useSubsystemHealth falls back to an empty list when the response omits subsystems', async () => {
    vi.mocked(getSubsystemHealth).mockResolvedValue({} as never);
    const { result } = renderHook(() => useSubsystemHealth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual([]);
  });

  test('useNotificationChannelHealth falls back to an empty list when the response omits channels', async () => {
    vi.mocked(getNotificationChannelHealth).mockResolvedValue({} as never);
    const { result } = renderHook(() => useNotificationChannelHealth());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data).toEqual([]);
  });

  test('useErrorAggregation exposes total_events, defaulting buckets and total to empty/0', async () => {
    vi.mocked(getErrorAggregation).mockResolvedValueOnce({
      buckets: [{ tag: 'x' }],
      total_events: 7,
    } as never);
    const { result } = renderHook(() => useErrorAggregation());
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.totalEvents).toBe(7);
    expect(getErrorAggregation).toHaveBeenCalledWith(24);

    vi.mocked(getErrorAggregation).mockResolvedValueOnce({} as never);
    await act(async () => {
      await result.current.refresh();
    });
    expect(result.current.buckets).toEqual([]);
    expect(result.current.totalEvents).toBe(0);
  });

  test('useErrorAggregation refetches when the window changes', async () => {
    vi.mocked(getErrorAggregation).mockResolvedValue({ buckets: [], total_events: 0 } as never);
    const { result, rerender } = renderHook(({ hours }) => useErrorAggregation(hours), {
      initialProps: { hours: 6 },
    });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(getErrorAggregation).toHaveBeenLastCalledWith(6);
    rerender({ hours: 48 });
    await waitFor(() => expect(getErrorAggregation).toHaveBeenLastCalledWith(48));
    expect(getErrorAggregation).toHaveBeenCalledTimes(2);
  });

  test('useDiagnostics keeps the previous result visible while a fresh run is in flight', async () => {
    vi.mocked(runDiagnostics).mockResolvedValueOnce({ tag: 'first' } as never);
    const { result } = renderHook(() => useDiagnostics());
    await act(async () => {
      await result.current.run();
    });
    const d = deferred<unknown>();
    vi.mocked(runDiagnostics).mockReturnValueOnce(d.promise as never);
    act(() => {
      void result.current.run();
    });
    await waitFor(() => expect(result.current.loading).toBe(true));
    expect(result.current.data).toEqual({ tag: 'first' });
    await act(async () => {
      d.resolve({ tag: 'second' });
    });
  });
});
