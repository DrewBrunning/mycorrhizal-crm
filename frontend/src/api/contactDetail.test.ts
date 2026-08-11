import { describe, test, expect, vi, afterEach } from 'vitest';
import { getContactDetail } from './contactDetail';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getContactDetail', () => {
  test('requests the contact-detail endpoint and normalizes empty blocks', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        contact: { id: 1, uid: 'alice-uid' },
        user: { enabled_contact_fields: ['organization'] },
        // Every collection deliberately omitted, mirroring what a fresh
        // contact with no data would send if the server ever regressed to
        // omitempty (CLAUDE.md frontend trap 8) -- the client-side
        // normalization must not depend on the server getting this right.
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const detail = await getContactDetail(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/1/detail');
    expect(detail.contact.uid).toBe('alice-uid');
    expect(detail.user.enabled_contact_fields).toEqual(['organization']);
    for (const key of [
      'notes', 'activities', 'completions', 'reminders',
      'relationship_edges', 'life_events', 'agenda', 'gifts',
      'field_values', 'external_identities', 'external_activities',
      'circles', 'tags',
    ] as const) {
      expect(detail[key]).toEqual([]);
    }
    expect(detail.immich).toBeUndefined();
  });

  test('passes through populated blocks, including the two name-resolution enrichments', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        contact: { id: 1, uid: 'alice-uid' },
        user: { enabled_contact_fields: [] },
        notes: [],
        activities: [],
        completions: [],
        reminders: [],
        relationship_edges: [
          { edge: { id: 'e1', source_id: 'alice-uid', target_id: 'bob-uid', type: 'spouse_of' }, other_party_name: 'Bob Marley' },
        ],
        life_events: [
          { id: 'le1', entity_id: 'alice-uid', type: 'moved', related_entity_ids: ['bob-uid'], related_entity_names: { 'bob-uid': 'Bob Marley' } },
        ],
        agenda: [],
        gifts: [],
        field_values: [],
        external_identities: [],
        external_activities: [],
        circles: [{ id: 'c1', name: 'Book Club' }],
        tags: [{ id: 't1', name: 'VIP' }],
        immich: { summary: null },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const detail = await getContactDetail(1);

    expect(detail.relationship_edges[0].other_party_name).toBe('Bob Marley');
    expect(detail.life_events[0].related_entity_names['bob-uid']).toBe('Bob Marley');
    expect(detail.circles[0].name).toBe('Book Club');
    expect(detail.tags[0].name).toBe('VIP');
    expect(detail.immich).toEqual({ summary: null });
  });
});
