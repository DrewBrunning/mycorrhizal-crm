import { afterEach, describe, expect, test, vi } from 'vitest';
import { getContactBriefing } from './briefings';

afterEach(() => {
  vi.unstubAllGlobals();
});

describe('getContactBriefing', () => {
  test('requests the contact briefing endpoint and parses the composition', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        contact_id: 1,
        uid: 'alice-uid',
        name: 'Alice Wonder',
        kind: 'human',
        last_activity: { id: 9, title: 'Coffee', type: 'visit', date: '2026-08-02T10:00:00Z' },
        recent_notes: [{ ID: 3, content: 'Talks about her garden', date: '2026-07-30T00:00:00Z' }],
        open_agenda_items: [{ id: 'a1', entity_id: 'alice-uid', content: 'Ask about the surgery' }],
        relationships: [
          {
            edge: { id: 'e1', source_id: 'bob-uid', target_id: 'alice-uid', type: 'spouse_of' },
            other_party_name: 'Bob Marley',
            display_token: 'spouse_of',
          },
        ],
        life_events: [{ id: 'le1', entity_id: 'alice-uid', type: 'graduated', description: 'PhD' }],
        upcoming_reminders: [{ ID: 5, message: 'Send card', remind_at: '2026-08-10T09:00:00Z' }],
        upcoming_dates: [{ label: 'birthday', date: '--01-15', days_until: 10 }],
        cadence: {
          policy: { id: 'p1', entity_id: 'alice-uid', target_interval_days: 30 },
          health: { has_qualifying_interaction: true, overdue_by: 3 },
        },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const briefing = await getContactBriefing(1);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/1/briefing');
    expect(briefing.name).toBe('Alice Wonder');
    expect(briefing.last_activity?.title).toBe('Coffee');
    expect(briefing.open_agenda_items[0].content).toBe('Ask about the surgery');
    expect(briefing.relationships[0].display_token).toBe('spouse_of');
    expect(briefing.upcoming_dates[0].days_until).toBe(10);
    expect(briefing.cadence?.health.overdue_by).toBe(3);
  });
});
