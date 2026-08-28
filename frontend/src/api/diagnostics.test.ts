import { afterEach, describe, expect, test, vi } from 'vitest';
import { runDiagnostics } from './diagnostics';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('runDiagnostics', () => {
  test('requests /admin/diagnostics and parses the checklist response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        timestamp: '2026-08-27T22:00:00Z',
        summary: { status: 'warning', ok: 16, warnings: 2, errors: 0 },
        checks: [
          { name: 'config', status: 'ok', message: 'configuration is valid' },
          {
            name: 'filesystem',
            status: 'warning',
            message: 'attachments directory is not writable',
          },
          {
            name: 'background_jobs',
            status: 'warning',
            message: 'failing job(s): daily_reminders',
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await runDiagnostics();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/diagnostics');
    expect(response.summary.warnings).toBe(2);
    expect(response.checks).toHaveLength(3);
    expect(response.checks[1].message).toBe('attachments directory is not writable');
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(runDiagnostics()).rejects.toBeDefined();
  });
});
