import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  dismissContactSyncConflict,
  getContactSyncConflicts,
  restoreContactSyncConflict,
} from './contactSyncConflicts';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getContactSyncConflicts', () => {
  test('requests the conflicts endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        sync_conflicts: [
          {
            id: 'c1',
            created_at: '2026-08-24T00:00:00Z',
            updated_at: '2026-08-24T00:00:00Z',
            subscription_id: 5,
            contact_id: 42,
            field: 'phone',
            local_value: '[{"value":"555-0100"}]',
            remote_value: '[]',
            status: 'pending',
            contact_vcard_uid: 'uid-42',
            contact_name: 'Grace Hopper',
            subscription_name: 'Work',
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const { sync_conflicts } = await getContactSyncConflicts();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-sync-conflicts');
    expect(sync_conflicts).toHaveLength(1);
    expect(sync_conflicts[0].field).toBe('phone');
    expect(sync_conflicts[0].contact_name).toBe('Grace Hopper');
  });
});

describe('restoreContactSyncConflict', () => {
  test('POSTs the restore action for the conflict id', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ message: 'ok' }) });
    vi.stubGlobal('fetch', fetchMock);

    await restoreContactSyncConflict('c1');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/contact-sync-conflicts/c1/restore');
    expect(init.method).toBe('POST');
  });

  test('throws on a 409 (already resolved)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: async () => ({ error: { code: 'CONFLICT', message: 'already resolved' } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(restoreContactSyncConflict('c1')).rejects.toThrow();
  });
});

describe('dismissContactSyncConflict', () => {
  test('POSTs the dismiss action for the conflict id', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({ message: 'ok' }) });
    vi.stubGlobal('fetch', fetchMock);

    await dismissContactSyncConflict('c2');

    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toContain('/contact-sync-conflicts/c2/dismiss');
    expect(init.method).toBe('POST');
  });
});
