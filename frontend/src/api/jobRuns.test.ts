import { afterEach, describe, expect, test, vi } from 'vitest';
import { getJobRunHealth, JOB_RUN_RESULTS, listJobRuns } from './jobRuns';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('getJobRunHealth', () => {
  test('requests /admin/job-runs/health and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        jobs: [
          {
            job_name: 'daily_reminders',
            status: 'failing',
            last_run_at: '2026-08-27T17:40:00Z',
            last_result: 'failure',
            last_trigger: 'scheduled',
            last_duration_ms: 40000,
            last_items_processed: null,
            last_success_at: '2026-08-27T03:04:00Z',
            last_failure_at: '2026-08-27T17:40:00Z',
            last_error: '2 notification send(s) failed',
            incident_first_failure_at: '2026-08-27T03:19:00Z',
            consecutive_failures: 9,
            duration_sample_size: 20,
            avg_duration_ms: 1200,
            max_duration_ms: 40000,
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getJobRunHealth();
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/job-runs/health');
    expect(response.jobs[0].consecutive_failures).toBe(9);
    expect(response.jobs[0].avg_duration_ms).toBe(1200);
  });
});

describe('listJobRuns', () => {
  test('serializes filters into the query string', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ job_runs: [], total: 0 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await listJobRuns({ jobName: 'calendar_sync', result: 'failure', limit: 25 });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/job-runs?');
    expect(url).toContain('job_name=calendar_sync');
    expect(url).toContain('result=failure');
    expect(url).toContain('limit=25');
  });

  test('omits the query string when no filters are given', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ job_runs: [], total: 0 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await listJobRuns();
    const [url] = fetchMock.mock.calls[0];
    expect(url).toMatch(/\/admin\/job-runs$/);
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);
    await expect(listJobRuns()).rejects.toBeDefined();
  });
});

describe('JOB_RUN_RESULTS mirror', () => {
  test('is the frozen backend result vocabulary', () => {
    expect(JOB_RUN_RESULTS).toEqual(['success', 'failure', 'skipped']);
  });
});
