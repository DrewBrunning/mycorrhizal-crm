import { describe, test, expect, vi, afterEach } from 'vitest';
import { getAuditEvents, undoAuditEvent, AUDIT_ENTITY_TYPES } from './audit';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getAuditEvents', () => {
  test('requests /audit with the default limit and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        audit_events: [
          {
            id: 7,
            created_at: '2026-08-01T10:00:00Z',
            entity_type: 'contact',
            entity_id: 'uid-123',
            operation: 'update',
          },
        ],
        total: 1,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getAuditEvents();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/audit?');
    expect(url).toContain('limit=100');
    expect(response.total).toBe(1);
    expect(response.audit_events[0].entity_id).toBe('uid-123');
  });

  test('sends the entity_type and entity_id filters when supplied', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ audit_events: [], total: 0 }) });
    vi.stubGlobal('fetch', fetchMock);

    await getAuditEvents({ entity_type: 'note', entity_id: '42', limit: 250 });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('entity_type=note');
    expect(url).toContain('entity_id=42');
    expect(url).toContain('limit=250');
  });

  test('omits empty filters rather than sending blank params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ audit_events: [], total: 0 }) });
    vi.stubGlobal('fetch', fetchMock);

    await getAuditEvents({ entity_id: '' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).not.toContain('entity_id=');
  });

  test('throws an ApiError with the backend message when the request fails', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 500,
      json: async () => ({ error: { code: 'DATABASE_ERROR', message: 'Failed to list audit events' }, request_id: 'req-1' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getAuditEvents()).rejects.toMatchObject({
      name: 'ApiError',
      status: 500,
      message: 'Failed to list audit events',
    });
  });
});

describe('undoAuditEvent', () => {
  test('POSTs to /audit/:id/undo and returns the success message', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Contact restored to its previous state' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await undoAuditEvent(7);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/audit/7/undo');
    expect(init.method).toBe('POST');
    expect(result.message).toBe('Contact restored to its previous state');
  });

  test('preserves the status code so the UI can distinguish 400/404/410', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 410,
      json: async () => ({ error: { code: 'GONE', message: 'past retention' } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(undoAuditEvent(7)).rejects.toMatchObject({ status: 410 });
  });
});

// The entity-type mirror must stay in sync with backend/models/audit.go's
// AuditEntity* constants and openapi.yaml's enum -- a drift here silently
// breaks the filter dropdown for a whole entity.
test('AUDIT_ENTITY_TYPES mirrors the backend enum exactly', () => {
  expect(AUDIT_ENTITY_TYPES).toEqual([
    'contact',
    'note',
    'activity',
    'life_event',
    'gift',
    'circle',
    'tag',
    'household',
    'reminder',
    // Auth/admin lifecycle entities (issue #381).
    'user',
    'auth',
    'api_token',
  ]);
});
