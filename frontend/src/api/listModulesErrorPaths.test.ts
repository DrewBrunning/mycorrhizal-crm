import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createConversationAgenda,
  deleteConversationAgenda,
  discussConversationAgenda,
  getConversationAgenda,
  updateConversationAgenda,
} from './conversationAgenda';
import { getConnections, getGraph } from './graph';
import {
  createOccasionObligation,
  deleteOccasionObligation,
  downloadOccasionCardListCSV,
  getGiftShoppingList,
  getOccasionObligations,
  getUpcomingOccasions,
  updateOccasionObligation,
} from './occasionObligations';

// Every one of these calls must surface the backend's parsed error rather
// than returning the error body as if it were data. These paths were only
// ever exercised incidentally (by ContactDetailPage's old catch-all-404 page
// test); the per-file coverage ratchet flagged them once that test started
// routing each endpoint explicitly.

afterEach(() => {
  vi.unstubAllGlobals();
});

function failWith(status: number, message: string) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: false,
    status,
    json: async () => ({ error: { message, code: 'TEST_ERROR' } }),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

function okWith(body: unknown) {
  const fetchMock = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => body });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

const calls: Array<[string, () => Promise<unknown>]> = [
  ['getConversationAgenda', () => getConversationAgenda()],
  ['createConversationAgenda', () => createConversationAgenda({ entity_id: 'u', content: 'c' })],
  [
    'updateConversationAgenda',
    () => updateConversationAgenda('a1', { entity_id: 'u', content: 'c' }),
  ],
  ['discussConversationAgenda', () => discussConversationAgenda('a1')],
  ['deleteConversationAgenda', () => deleteConversationAgenda('a1')],
  ['getGraph', () => getGraph()],
  ['getConnections', () => getConnections({ from: 'u' })],
  ['getOccasionObligations', () => getOccasionObligations()],
  [
    'createOccasionObligation',
    () => createOccasionObligation({ entity_id: 'u', kind: 'card' } as never),
  ],
  [
    'updateOccasionObligation',
    () => updateOccasionObligation('o1', { entity_id: 'u', kind: 'card' } as never),
  ],
  ['deleteOccasionObligation', () => deleteOccasionObligation('o1')],
  ['getUpcomingOccasions', () => getUpcomingOccasions()],
  ['getGiftShoppingList', () => getGiftShoppingList()],
  ['downloadOccasionCardListCSV', () => downloadOccasionCardListCSV()],
];

describe.each(calls)('%s', (_name, call) => {
  test('rejects with the parsed backend error on a non-OK response', async () => {
    failWith(500, 'boom');
    await expect(call()).rejects.toMatchObject({ message: 'boom', status: 500 });
  });
});

describe('list query-param branches', () => {
  test('getConversationAgenda sends the default limit and nothing else when called bare', async () => {
    const fetchMock = okWith({ conversation_agenda: [] });
    await getConversationAgenda();
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('limit=100');
    expect(url).not.toContain('entity_id=');
    expect(url).not.toContain('cursor=');
  });

  test('getConversationAgenda forwards cursor and a custom limit', async () => {
    const fetchMock = okWith({ conversation_agenda: [] });
    await getConversationAgenda({ cursor: 'c2', limit: 5 });
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('limit=5');
    expect(url).toContain('cursor=c2');
  });

  test('getOccasionObligations forwards entity, cursor and limit', async () => {
    const fetchMock = okWith({ occasion_obligations: [] });
    await getOccasionObligations({ entityId: 'u', cursor: 'c3', limit: 7 });
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('entity_id=u');
    expect(url).toContain('cursor=c3');
    expect(url).toContain('limit=7');
  });

  test('getGiftShoppingList passes a 90-day window and include_sensitive', async () => {
    const fetchMock = okWith({ items: [] });
    await getGiftShoppingList({ days: 90, includeSensitive: true });
    const url = String(fetchMock.mock.calls[0][0]);
    expect(url).toContain('days=90');
    expect(url).toContain('include_sensitive=true');
  });

  test('getConnections forwards depth and relation only when given', async () => {
    const fetchMock = okWith({ connections: [] });
    await getConnections({ from: 'u', depth: 2, relation: 'friend_of' });
    await getConnections({ from: 'u' });
    const withOpts = String(fetchMock.mock.calls[0][0]);
    const bare = String(fetchMock.mock.calls[1][0]);
    expect(withOpts).toContain('depth=2');
    expect(withOpts).toContain('relation=friend_of');
    expect(bare).not.toContain('depth=');
    expect(bare).not.toContain('relation=');
  });
});
