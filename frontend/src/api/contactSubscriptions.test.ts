import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  type ContactSubscription,
  createContactSubscription,
  deleteContactSubscription,
  getContactSubscriptions,
  syncContactSubscription,
  updateContactSubscription,
} from './contactSubscriptions';

afterEach(() => {
  vi.unstubAllGlobals();
});

const subscription: ContactSubscription = {
  id: 1,
  name: 'Work book',
  url: 'https://dav.example.com/addressbooks/alice/contacts/',
  username: 'alice',
  has_password: true,
  sync_enabled: true,
  last_synced_at: '2026-08-01T00:00:00Z',
  last_sync_status: 'success',
  last_sync_error: '',
  created_at: '2026-07-01T00:00:00Z',
  last_attempt_at: '2026-08-01T00:00:00Z',
  last_success_at: '2026-08-01T00:00:00Z',
  last_failure_at: null,
  consecutive_failures: 0,
  incident_first_failure_at: null,
  last_run_duration_ms: 900,
  last_run_stats: { created: 1, updated: 2, archived: 0, skipped: 3 },
  terminal_failure_at: null,
  terminal_reason: '',
  pending_conflicts: 0,
};

const okResponse = (body: unknown) => ({
  ok: true,
  status: 200,
  text: async () => JSON.stringify(body),
});

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  text: async () => JSON.stringify({ error: { message: 'Invalid URL' } }),
});

describe('getContactSubscriptions', () => {
  test('GETs /contact-subscriptions and unwraps the array', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(okResponse({ contact_subscriptions: [subscription] }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getContactSubscriptions();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-subscriptions');
    expect(init.method).toBe('GET');
    expect(result).toEqual([subscription]);
  });

  test('returns an empty array when the key is absent', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    expect(await getContactSubscriptions()).toEqual([]);
  });

  test('throws the parsed error on a non-ok response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getContactSubscriptions()).rejects.toThrow('Invalid URL');
  });
});

describe('mutations', () => {
  test('createContactSubscription POSTs the payload', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(subscription));
    vi.stubGlobal('fetch', fetchMock);

    await createContactSubscription({ name: 'Work book', url: subscription.url });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-subscriptions');
    expect(init.method).toBe('POST');
  });

  test('updateContactSubscription PUTs to the id path', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(subscription));
    vi.stubGlobal('fetch', fetchMock);

    await updateContactSubscription(7, { name: 'x', url: subscription.url });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-subscriptions/7');
    expect(init.method).toBe('PUT');
  });

  test('deleteContactSubscription DELETEs the id path', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'ok' }));
    vi.stubGlobal('fetch', fetchMock);

    await deleteContactSubscription(3);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-subscriptions/3');
    expect(init.method).toBe('DELETE');
  });

  test('syncContactSubscription POSTs to the /sync path and returns the result', async () => {
    const result = { message: 'ok', created: 1, updated: 2, archived: 0, skipped: 3 };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(result));
    vi.stubGlobal('fetch', fetchMock);

    expect(await syncContactSubscription(9)).toEqual(result);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contact-subscriptions/9/sync');
    expect(init.method).toBe('POST');
  });
});
