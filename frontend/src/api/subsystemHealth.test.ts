import { afterEach, describe, expect, test, vi } from 'vitest';
import { getSubsystemHealth, SUBSYSTEMS } from './subsystemHealth';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('getSubsystemHealth', () => {
  test('requests /admin/subsystem-health and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        subsystems: [
          {
            subsystem: 'contact_sync',
            status: 'failing',
            last_attempt_at: '2026-08-27T17:40:00Z',
            last_success_at: '2026-08-27T17:04:00Z',
            last_failure_at: '2026-08-27T17:40:00Z',
            incident_first_failure_at: '2026-08-27T17:19:00Z',
            consecutive_failures: 9,
            last_error: 'carddav auth rejected',
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getSubsystemHealth();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/admin/subsystem-health');
    expect(response.subsystems[0].consecutive_failures).toBe(9);
    expect(response.subsystems[0].incident_first_failure_at).toBe('2026-08-27T17:19:00Z');
  });

  test('throws on a non-ok response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 403,
      json: async () => ({ error: 'forbidden' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getSubsystemHealth()).rejects.toBeDefined();
  });
});

describe('SUBSYSTEMS mirror', () => {
  test('is the frozen backend subsystem vocabulary, in order', () => {
    expect(SUBSYSTEMS).toEqual([
      'contact_sync',
      'calendar_sync',
      'notification',
      'backup',
      'scheduler',
      'webhook',
    ]);
  });
});
