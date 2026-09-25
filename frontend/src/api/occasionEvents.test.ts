import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  addOccasionEventAttendee,
  createOccasionEvent,
  deleteOccasionEvent,
  getInviteeSuggestions,
  getOccasionEvent,
  getOccasionEvents,
  removeOccasionEventAttendee,
  updateOccasionEvent,
  updateOccasionEventAttendee,
} from './occasionEvents';

afterEach(() => {
  vi.unstubAllGlobals();
});

const eventJson = {
  id: 'ev-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  title: 'Summer BBQ',
  starts_at: '2026-07-04T15:00:00Z',
  sensitivity: 'normal',
};

describe('getOccasionEvents', () => {
  test('passes the window and limit as query params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ occasion_events: [eventJson], total: 1, next_cursor: '', limit: 100 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getOccasionEvents({
      from: '2026-07-01T00:00:00Z',
      to: '2026-09-01T00:00:00Z',
    });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events?');
    expect(url).toContain('from=2026-07-01T00%3A00%3A00Z');
    expect(url).toContain('limit=100');
    expect(response.occasion_events[0].title).toBe('Summer BBQ');
  });
});

describe('getOccasionEvent', () => {
  test('returns the event and attendees envelope', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        occasion_event: eventJson,
        attendees: [
          {
            id: 'a-1',
            event_id: 'ev-1',
            entity_id: 'uid-1',
            contact_id: 3,
            contact_name: 'Alice',
            rsvp: 'accepted',
          },
        ],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const detail = await getOccasionEvent('ev-1');
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/ev-1');
    expect(detail.attendees[0].contact_name).toBe('Alice');
  });
});

describe('createOccasionEvent', () => {
  test('POSTs the input and unwraps occasion_event', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'created', occasion_event: eventJson }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const created = await createOccasionEvent({
      title: 'Summer BBQ',
      starts_at: '2026-07-04T15:00:00Z',
    });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events');
    expect(init.method).toBe('POST');
    expect(created.id).toBe('ev-1');
  });
});

describe('updateOccasionEvent', () => {
  test('PUTs to the id-scoped endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ ...eventJson, title: 'Renamed' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const updated = await updateOccasionEvent('ev-1', {
      title: 'Renamed',
      starts_at: '2026-07-04T15:00:00Z',
    });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/ev-1');
    expect(init.method).toBe('PUT');
    expect(updated.title).toBe('Renamed');
  });
});

describe('deleteOccasionEvent', () => {
  test('DELETEs and throws on failure', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({ ok: true, json: async () => ({}) })
      .mockResolvedValueOnce({
        ok: false,
        status: 404,
        json: async () => ({ error: { code: 'NOT_FOUND', message: 'nope' } }),
      });
    vi.stubGlobal('fetch', fetchMock);

    await deleteOccasionEvent('ev-1');
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE');
    await expect(deleteOccasionEvent('missing')).rejects.toThrow();
  });
});

describe('attendee calls', () => {
  test('addOccasionEventAttendee POSTs the entity id and unwraps attendee', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'added',
        attendee: { id: 'a-1', event_id: 'ev-1', entity_id: 'uid-1', rsvp: 'pending' },
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const attendee = await addOccasionEventAttendee('ev-1', { entity_id: 'uid-1' });
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/ev-1/attendees');
    expect(init.method).toBe('POST');
    expect(attendee.rsvp).toBe('pending');
  });

  test('updateOccasionEventAttendee PUTs the rsvp', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ id: 'a-1', event_id: 'ev-1', entity_id: 'uid-1', rsvp: 'declined' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const attendee = await updateOccasionEventAttendee('ev-1', 'uid-1', 'declined');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/ev-1/attendees/uid-1');
    expect(init.method).toBe('PUT');
    expect(attendee.rsvp).toBe('declined');
  });

  test('removeOccasionEventAttendee DELETEs', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await removeOccasionEventAttendee('ev-1', 'uid-1');
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/ev-1/attendees/uid-1');
    expect(init.method).toBe('DELETE');
  });
});

describe('getInviteeSuggestions', () => {
  test('joins circle ids and forwards event_id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        suggestions: [{ contact_id: 1, contact_name: 'Alice', entity_id: 'uid-1' }],
        circle_ids: ['c1', 'c2'],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const suggestions = await getInviteeSuggestions({ circleIds: ['c1', 'c2'], eventId: 'ev-1' });
    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/occasion-events/invitee-suggestions?');
    expect(url).toContain('circle_ids=c1%2Cc2');
    expect(url).toContain('event_id=ev-1');
    expect(suggestions).toHaveLength(1);
  });

  test('returns an empty array when the response omits suggestions', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    await expect(getInviteeSuggestions({ circleIds: ['c1'] })).resolves.toEqual([]);
  });
});
