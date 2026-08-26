import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  dismissReachOutSuggestion,
  getReachOutSuggestions,
  type ReachOutSuggestion,
} from './reachOutSuggestions';

afterEach(() => {
  vi.unstubAllGlobals();
});

const suggestion: ReachOutSuggestion = {
  id: 'suggestion-1',
  created_at: '2026-08-01T00:00:00Z',
  updated_at: '2026-08-01T00:00:00Z',
  contact_vcard_uid: 'alice-uid',
  kind: 'organization',
  old_value: 'OldCo',
  new_value: 'NewCo',
  audit_event_id: 1,
  status: 'pending',
  contact_id: 7,
  contact_name: 'Alice Smith',
};

describe('getReachOutSuggestions', () => {
  test('returns the suggestion list verbatim (null-safe shape on the wire)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ suggestions: [suggestion] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getReachOutSuggestions();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/reach-out-suggestions');
    expect(response.suggestions).toEqual([suggestion]);
  });
});

describe('dismissReachOutSuggestion', () => {
  test('POSTs to the suggestion-specific dismiss endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await dismissReachOutSuggestion('suggestion-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/reach-out-suggestions/suggestion-1/dismiss');
    expect(init.method).toBe('POST');
  });
});
