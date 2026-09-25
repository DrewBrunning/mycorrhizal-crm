import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createLifeEvent,
  deleteLifeEvent,
  getLifeEventSuggestions,
  getLifeEvents,
  isKnownLifeEventCategory,
  LIFE_EVENT_CATEGORIES,
  LIFE_EVENT_TYPES_BY_CATEGORY,
  partialDateDisplay,
  partialDateHasMonthDay,
  partialDateIsYearOnly,
  partialDateRangeDisplay,
  resolveLifeEventSuggestion,
  updateLifeEvent,
} from './lifeEvents';

afterEach(() => {
  vi.unstubAllGlobals();
});

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

function errorResponse() {
  return {
    ok: false,
    status: 400,
    statusText: 'Bad Request',
    json: async () => ({
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'type' } },
      request_id: 'req-1',
    }),
  };
}

const lifeEvent = {
  id: 'event-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'contact-1',
  type: 'job_change',
  category: 'work_education',
  date: { year: 2024, month: 3, day: 1 },
  description: 'Started at Acme',
  source: 'manual',
  remind: false,
};

const createResponse = { message: 'Life event created', life_event: lifeEvent };

const listResponse = {
  life_events: [lifeEvent],
  next_cursor: 'cursor-2',
  limit: 25,
};

describe('getLifeEvents', () => {
  test('GETs /life-events with entity_id, cursor and limit params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(listResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getLifeEvents({ entity_id: 'contact-1', cursor: 'cursor-1', limit: 10 });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/life-events?');
    expect(url).toContain('limit=10');
    expect(url).toContain('cursor=cursor-1');
    expect(url).toContain('entity_id=contact-1');
    expect(init.method).toBeUndefined();
    expect(result).toEqual(listResponse);
  });

  test('defaults to limit=25 when no params are given', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(listResponse));
    vi.stubGlobal('fetch', fetchMock);

    await getLifeEvents();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('limit=25');
    expect(url).not.toContain('entity_id=');
    expect(url).not.toContain('cursor=');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getLifeEvents()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('createLifeEvent', () => {
  test('POSTs the life event data and returns the create response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(createResponse));
    vi.stubGlobal('fetch', fetchMock);

    const input = {
      entity_id: 'contact-1',
      type: 'job_change',
      category: 'work_education',
      description: 'Started at Acme',
    };
    const result = await createLifeEvent(input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/life-events');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(createResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(createLifeEvent({ entity_id: 'contact-1', type: 'moved' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('updateLifeEvent', () => {
  test('PUTs the life event data to /life-events/:id and returns the updated event', async () => {
    const updated = { ...lifeEvent, description: 'Now a senior engineer' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(updated));
    vi.stubGlobal('fetch', fetchMock);

    const input = {
      entity_id: 'contact-1',
      type: 'job_change',
      description: 'Now a senior engineer',
    };
    const result = await updateLifeEvent('event-1', input);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/life-events/event-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(updated);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(
      updateLifeEvent('event-1', { entity_id: 'contact-1', type: 'moved' }),
    ).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteLifeEvent', () => {
  test('DELETEs /life-events/:id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await deleteLifeEvent('event-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/life-events/event-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(deleteLifeEvent('event-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('isKnownLifeEventCategory', () => {
  test('recognizes all five known category tokens', () => {
    expect(isKnownLifeEventCategory('home_living')).toBe(true);
    expect(isKnownLifeEventCategory('health_wellness')).toBe(true);
    expect(isKnownLifeEventCategory('work_education')).toBe(true);
    expect(isKnownLifeEventCategory('travel_experiences')).toBe(true);
    expect(isKnownLifeEventCategory('family_relationships')).toBe(true);
  });

  test('rejects an unknown category token', () => {
    expect(isKnownLifeEventCategory('finances')).toBe(false);
    expect(isKnownLifeEventCategory('')).toBe(false);
  });
});

describe('partialDateDisplay', () => {
  test('formats a full date as YYYY-MM-DD', () => {
    expect(partialDateDisplay({ year: 1990, month: 3, day: 15 })).toBe('1990-03-15');
  });

  test('formats a year-only date as the bare year', () => {
    expect(partialDateDisplay({ year: 1990 })).toBe('1990');
  });

  test('formats month/day without a year as MM/DD', () => {
    expect(partialDateDisplay({ month: 3, day: 15 })).toBe('03/15');
  });

  test('formats a month-only date with a ?? day placeholder', () => {
    expect(partialDateDisplay({ month: 3 })).toBe('03/??');
  });

  test('returns the bare year when a month is present without a day', () => {
    expect(partialDateDisplay({ year: 1990, month: 3 })).toBe('1990');
  });

  test('returns an empty string when the date is missing', () => {
    expect(partialDateDisplay(undefined)).toBe('');
  });

  test('returns an empty string for a completely empty (but defined) date', () => {
    expect(partialDateDisplay({})).toBe('');
  });
});

describe('partialDateRangeDisplay', () => {
  test('joins start and end with an en dash', () => {
    expect(partialDateRangeDisplay({ year: 2019 }, { year: 2024 })).toBe('2019 – 2024');
  });

  test('renders just the start when there is no end', () => {
    expect(partialDateRangeDisplay({ year: 2019 }, undefined)).toBe('2019');
  });

  test('marks an open start', () => {
    expect(partialDateRangeDisplay(undefined, { year: 2024 })).toBe('– 2024');
  });

  test('is empty when neither endpoint exists', () => {
    expect(partialDateRangeDisplay(undefined, undefined)).toBe('');
  });
});

describe('partialDateHasMonthDay', () => {
  test('is true only when both month and day are present', () => {
    expect(partialDateHasMonthDay({ year: 1990, month: 3, day: 15 })).toBe(true);
    expect(partialDateHasMonthDay({ month: 3, day: 15 })).toBe(true);
    expect(partialDateHasMonthDay({ year: 1990 })).toBe(false);
    expect(partialDateHasMonthDay({ month: 3 })).toBe(false);
    expect(partialDateHasMonthDay(undefined)).toBe(false);
  });

  test('is false when day is present but month is not (isolates the month check)', () => {
    expect(partialDateHasMonthDay({ day: 15 })).toBe(false);
    expect(partialDateHasMonthDay({ year: 1990, day: 15 })).toBe(false);
  });
});

describe('partialDateIsYearOnly', () => {
  test('is true only for a bare year', () => {
    expect(partialDateIsYearOnly({ year: 1990 })).toBe(true);
    expect(partialDateIsYearOnly({ year: 1990, month: 3 })).toBe(false);
    expect(partialDateIsYearOnly({ year: 1990, month: 3, day: 15 })).toBe(false);
    expect(partialDateIsYearOnly({ month: 3, day: 15 })).toBe(false);
    expect(partialDateIsYearOnly(undefined)).toBe(false);
  });

  test('is false when year is absent, even if month/day are also absent', () => {
    expect(partialDateIsYearOnly({})).toBe(false);
  });

  test('is false when year and day are present but month is not (isolates the day check)', () => {
    expect(partialDateIsYearOnly({ year: 1990, day: 15 })).toBe(false);
  });
});

describe('life event constants', () => {
  test('LIFE_EVENT_CATEGORIES lists the five categories in order', () => {
    expect(LIFE_EVENT_CATEGORIES).toEqual([
      'home_living',
      'health_wellness',
      'work_education',
      'travel_experiences',
      'family_relationships',
    ]);
  });

  test('LIFE_EVENT_TYPES_BY_CATEGORY has a type list for every category', () => {
    expect(Object.keys(LIFE_EVENT_TYPES_BY_CATEGORY)).toEqual([...LIFE_EVENT_CATEGORIES]);
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.work_education).toContain('job_change');
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.home_living).toContain('moved');
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.family_relationships).toContain('married');
  });

  // Pins every token exactly -- issue #915's ticket-readiness bar plus the
  // sheer number of hand-typed string-literal mutants in this table means a
  // "contains" check alone leaves nearly every token unasserted.
  test('LIFE_EVENT_TYPES_BY_CATEGORY lists every type token exactly, in order', () => {
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.home_living).toEqual([
      'moved',
      'bought_a_home',
      'made_a_home_improvement',
      'went_on_holidays',
      'got_a_new_vehicle',
      'got_a_roommate',
    ]);
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.health_wellness).toEqual([
      'overcame_an_illness',
      'quit_a_habit',
      'started_new_eating_habits',
      'lost_weight',
      'started_wearing_glasses_or_contacts',
      'broke_a_bone',
      'removed_braces',
      'had_surgery',
      'went_to_the_dentist',
    ]);
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.work_education).toEqual([
      'job_change',
      'retired',
      'started_school',
      'studied_abroad',
      'started_volunteering',
      'published_a_paper',
      'started_military_service',
      'graduated',
    ]);
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.travel_experiences).toEqual([
      'started_a_sport',
      'started_a_hobby',
      'learned_a_new_instrument',
      'learned_a_new_language',
      'got_a_tattoo_or_piercing',
      'got_a_license',
      'traveled',
      'got_an_achievement_or_award',
      'changed_beliefs',
      'spoke_for_the_first_time',
      'kissed_for_the_first_time',
    ]);
    expect(LIFE_EVENT_TYPES_BY_CATEGORY.family_relationships).toEqual([
      'started_a_relationship',
      'got_engaged',
      'married',
      'anniversary',
      'expects_a_baby',
      'had_child',
      'added_a_family_member',
      'adopted_pet',
      'ended_a_relationship',
      'lost_a_loved_one',
    ]);
  });
});

describe('getLifeEventSuggestions', () => {
  test('GETs the contact suggestions and returns the array', async () => {
    const suggestion = {
      entity_id: 'contact-1',
      type: 'moved',
      category: 'home_living',
      date: { year: 2019 },
      end_date: { year: 2024 },
      source_kind: 'address',
      source_entry_id: 'addr-1',
    };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ suggestions: [suggestion] }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getLifeEventSuggestions(7);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/7/life-event-suggestions');
    expect(init.method).toBeUndefined();
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(result).toEqual([suggestion]);
  });

  test('returns an empty array when the wire omits suggestions', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(okResponse({})));
    expect(await getLifeEventSuggestions(7)).toEqual([]);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(getLifeEventSuggestions(7)).rejects.toMatchObject({ status: 400 });
  });
});

describe('resolveLifeEventSuggestion', () => {
  test('POSTs the resolution tuple', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ message: 'ok' }));
    vi.stubGlobal('fetch', fetchMock);

    const input = {
      entity_id: 'contact-1',
      source_kind: 'address',
      source_entry_id: 'addr-1',
      event_type: 'moved',
      resolution: 'dismissed' as const,
    };
    await resolveLifeEventSuggestion(input);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/life-event-suggestions/resolve');
    expect(init.method).toBe('POST');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(JSON.parse(init.body)).toEqual(input);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(
      resolveLifeEventSuggestion({
        entity_id: 'c',
        source_kind: 'address',
        source_entry_id: 'a',
        event_type: 'moved',
        resolution: 'accepted',
      }),
    ).rejects.toMatchObject({ status: 400 });
  });
});
