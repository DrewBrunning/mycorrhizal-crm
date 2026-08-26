import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createConversationAgenda,
  deleteConversationAgenda,
  discussConversationAgenda,
  getConversationAgenda,
  updateConversationAgenda,
} from './conversationAgenda';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getConversationAgenda', () => {
  test('requests the entity-scoped endpoint and parses the response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        conversation_agenda: [
          { id: 'a1', entity_id: 'alice-uid', content: 'Ask about her mother', discussed_at: null },
        ],
        next_cursor: '',
        limit: 100,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getConversationAgenda({ entityId: 'alice-uid' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/conversation-agenda?');
    expect(url).toContain('entity_id=alice-uid');
    expect(response.conversation_agenda[0].content).toBe('Ask about her mother');
  });
});

describe('createConversationAgenda', () => {
  test('POSTs the input and unwraps the created item', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Conversation agenda item created successfully',
        conversation_agenda: { id: 'a1', entity_id: 'alice-uid', content: 'Ask about the trip' },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createConversationAgenda({
      entity_id: 'alice-uid',
      content: 'Ask about the trip',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/conversation-agenda');
    expect(init.method).toBe('POST');
    expect(result.id).toBe('a1');
  });
});

describe('updateConversationAgenda', () => {
  test('PUTs the content fields as a full replace', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'a1', entity_id: 'alice-uid', content: 'Ask about the new job' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await updateConversationAgenda('a1', {
      entity_id: 'alice-uid',
      content: 'Ask about the new job',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/conversation-agenda/a1');
    expect(init.method).toBe('PUT');
  });
});

describe('discussConversationAgenda', () => {
  test('PATCHes with activity_id when one is supplied', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'a1', discussed_at: '2026-08-03T12:00:00Z', activity_id: 7 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await discussConversationAgenda('a1', 7);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/conversation-agenda/a1/discuss');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body).activity_id).toBe(7);
    expect(result.activity_id).toBe(7);
  });

  test('PATCHes with an empty body when no activity is supplied', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'a1', discussed_at: '2026-08-03T12:00:00Z' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await discussConversationAgenda('a1');

    const [, init] = fetchMock.mock.calls[0];
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body)).toEqual({});
  });
});

describe('deleteConversationAgenda', () => {
  test('DELETEs the item', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await deleteConversationAgenda('a1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/conversation-agenda/a1');
    expect(init.method).toBe('DELETE');
  });
});
