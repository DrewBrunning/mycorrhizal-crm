import { act, cleanup, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { getJobRunHealth, type JobRun, type JobRunHealth, listJobRuns } from '../api/jobRuns';
import { useJobRunHealth, useJobRunHistory } from './useJobRuns';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/jobRuns', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/jobRuns')>();
  return { ...actual, getJobRunHealth: vi.fn(), listJobRuns: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getJobRunHealth).mockReset();
  vi.mocked(listJobRuns).mockReset();
});

const health: JobRunHealth = {
  job_name: 'carddav-sync',
  status: 'healthy',
  last_run_at: '2026-01-01T00:00:00Z',
  last_result: 'success',
  last_trigger: 'scheduled',
  last_duration_ms: 12,
  last_items_processed: 3,
  last_success_at: '2026-01-01T00:00:00Z',
  last_failure_at: null,
  last_error: '',
  incident_first_failure_at: null,
  consecutive_failures: 0,
  duration_sample_size: 1,
  avg_duration_ms: 12,
  max_duration_ms: 12,
};

const run: JobRun = {
  id: 1,
  created_at: '2026-01-01T00:00:00Z',
  job_name: 'carddav-sync',
  trigger: 'scheduled',
  started_at: '2026-01-01T00:00:00Z',
  finished_at: '2026-01-01T00:00:00Z',
  duration_ms: 12,
  result: 'success',
  correlation_id: 'corr-1',
};

// Resolvable-by-hand promise so a test can control which in-flight request
// settles last.
function deferred<T>() {
  let resolve!: (v: T) => void;
  let reject!: (e: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe('useJobRunHealth', () => {
  test('loads the health projection on mount and exposes refresh', async () => {
    vi.mocked(getJobRunHealth).mockResolvedValue({ jobs: [health] });

    const { result } = renderHook(() => useJobRunHealth());
    await act(async () => {});

    expect(getJobRunHealth).toHaveBeenCalledTimes(1);
    expect(result.current.data).toEqual([health]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();

    vi.mocked(getJobRunHealth).mockResolvedValue({ jobs: [] });
    await act(async () => {
      await result.current.refresh();
    });

    expect(getJobRunHealth).toHaveBeenCalledTimes(2);
    expect(result.current.data).toEqual([]);
  });

  test('defaults to an empty job list when the response has no jobs key', async () => {
    vi.mocked(getJobRunHealth).mockResolvedValue({} as { jobs: JobRunHealth[] });

    const { result } = renderHook(() => useJobRunHealth());
    await act(async () => {});

    expect(result.current.data).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  test('sets an error when the fetch fails', async () => {
    vi.mocked(getJobRunHealth).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useJobRunHealth());
    await act(async () => {});

    expect(result.current.error).toBe('boom');
    expect(result.current.data).toEqual([]);
    expect(result.current.loading).toBe(false);
  });

  test('ignores a stale success that settles after a newer refresh', async () => {
    const first = deferred<{ jobs: JobRunHealth[] }>();
    const second = deferred<{ jobs: JobRunHealth[] }>();
    vi.mocked(getJobRunHealth)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useJobRunHealth());
    await act(async () => {});

    // Fire a refresh while the initial fetch is still in flight.
    const refreshPromise = result.current.refresh();

    await act(async () => {
      second.resolve({ jobs: [health] });
      await refreshPromise;
    });
    expect(result.current.data).toEqual([health]);

    // The stale first request settling afterwards must not overwrite it.
    await act(async () => {
      first.resolve({ jobs: [] });
    });
    expect(result.current.data).toEqual([health]);
  });

  test('ignores a stale failure that settles after a newer refresh', async () => {
    const first = deferred<{ jobs: JobRunHealth[] }>();
    const second = deferred<{ jobs: JobRunHealth[] }>();
    vi.mocked(getJobRunHealth)
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useJobRunHealth());
    await act(async () => {});

    const refreshPromise = result.current.refresh();
    await act(async () => {
      second.resolve({ jobs: [health] });
      await refreshPromise;
    });
    expect(result.current.error).toBeNull();

    await act(async () => {
      first.reject(new Error('stale failure'));
    });
    expect(result.current.error).toBeNull();
    expect(result.current.data).toEqual([health]);
  });
});

describe('useJobRunHistory', () => {
  test('loads the run history lazily and exposes reset', async () => {
    vi.mocked(listJobRuns).mockResolvedValue({ job_runs: [run], total: 1 });

    const { result } = renderHook(() => useJobRunHistory());
    expect(result.current.loading).toBe(false);

    await act(async () => {
      await result.current.load({ jobName: 'carddav-sync', limit: 25 });
    });

    expect(listJobRuns).toHaveBeenCalledWith({ jobName: 'carddav-sync', limit: 25 });
    expect(result.current.data).toEqual([run]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();

    act(() => result.current.reset());
    expect(result.current.data).toEqual([]);
    expect(result.current.loading).toBe(false);
    expect(result.current.error).toBeNull();
  });

  test('defaults to an empty list when the response has no job_runs key', async () => {
    vi.mocked(listJobRuns).mockResolvedValue({ job_runs: undefined } as never);

    const { result } = renderHook(() => useJobRunHistory());
    await act(async () => {
      await result.current.load({ jobName: 'x' });
    });

    expect(result.current.data).toEqual([]);
  });

  test('sets an error when the load fails', async () => {
    vi.mocked(listJobRuns).mockRejectedValue(new Error('boom'));

    const { result } = renderHook(() => useJobRunHistory());
    await act(async () => {
      await result.current.load({ jobName: 'x' });
    });

    expect(result.current.error).toBe('boom');
    expect(result.current.loading).toBe(false);
  });

  test('ignores a stale success that settles after a newer load', async () => {
    const first = deferred<{ job_runs: JobRun[]; total: number }>();
    const second = deferred<{ job_runs: JobRun[]; total: number }>();
    vi.mocked(listJobRuns).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useJobRunHistory());

    await act(async () => {
      // Stale request "a" — fires first, settles last.
      result.current.load({ jobName: 'a' });
      const loadPromise = result.current.load({ jobName: 'b' });
      second.resolve({ job_runs: [run], total: 1 });
      await loadPromise;
    });
    expect(result.current.data).toEqual([run]);

    await act(async () => {
      first.resolve({ job_runs: [], total: 0 });
    });
    expect(result.current.data).toEqual([run]);
  });

  test('ignores a stale failure that settles after a newer load', async () => {
    const first = deferred<{ job_runs: JobRun[]; total: number }>();
    const second = deferred<{ job_runs: JobRun[]; total: number }>();
    vi.mocked(listJobRuns).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);

    const { result } = renderHook(() => useJobRunHistory());

    await act(async () => {
      result.current.load({ jobName: 'a' });
      const loadPromise = result.current.load({ jobName: 'b' });
      second.resolve({ job_runs: [run], total: 1 });
      await loadPromise;
    });
    expect(result.current.error).toBeNull();

    await act(async () => {
      first.reject(new Error('stale failure'));
    });
    expect(result.current.error).toBeNull();
    expect(result.current.data).toEqual([run]);
  });
});
