// Issues #257 + #266: pins web's parsing of the backend's documented
// response contract.
//
// Two checks per fixture, both against the raw fixture JSON (per CLAUDE.md
// frontend trap 8: decoding into a typed struct/interface first makes
// "absent" and "[]" indistinguishable, which is exactly why the existing
// regression test for that bug passed anyway):
//  1. Raw-JSON assertions that the array-shaped keys the trap is about are
//     actually present as arrays on the wire, not omitted.
//  2. Feed that same raw JSON through the real exported adapter
//     (getContacts/getContactDetail/getDashboard) via a stubbed fetch, and
//     assert the parsed result still carries those arrays through. This is
//     the layer that exercises production code, not just the fixture --
//     dashboard.ts's getDashboard() in particular does zero client-side
//     normalization (unlike contactDetail.ts), so a backend regression from
//     always-`[]` to sometimes-absent surfaces here as `undefined`.
//
// (An earlier draft of this file tried a compile-time `raw satisfies
// ResponseType` pin instead. Dropped: GET /contacts and GET /dashboard both
// return wire shapes distinct from their post-adapter TS types by design
// (summaryToLegacyContact's rename, the dashboard composite's raw
// gorm-style embed), and any Card-shaped nested field with a string-literal
// union type (NameComponent.kind, etc.) fails a raw-JSON-import `satisfies`
// check on literal-widening grounds alone, unrelated to any real bug. Not
// worth the false-positive noise for what raw-JSON assertions already
// cover.)
//
// Fixtures live in /testdata/contract-fixtures/ (shared with the Android
// suite -- see that directory's README) and are GENERATED from
// backend/openapi.yaml's response examples by `go run ./cmd/gencontract`,
// not hand-written or hand-captured. When the spec's examples change, the
// drift test backend/contract_fixtures_test.go fails until they are
// regenerated.
import { afterEach, describe, expect, test, vi } from 'vitest';
import activitiesListRaw from '../../../testdata/contract-fixtures/activities-list.json';
import circlesListRaw from '../../../testdata/contract-fixtures/circles-list.json';
import contactDetailRaw from '../../../testdata/contract-fixtures/contact-detail.json';
import contactNotesRaw from '../../../testdata/contract-fixtures/contact-notes.json';
import contactRemindersRaw from '../../../testdata/contract-fixtures/contact-reminders.json';
import contactsListRaw from '../../../testdata/contract-fixtures/contacts-list.json';
import dashboardRaw from '../../../testdata/contract-fixtures/dashboard.json';
import lifeEventsListRaw from '../../../testdata/contract-fixtures/life-events-list.json';
import occasionEventsListRaw from '../../../testdata/contract-fixtures/occasion-events-list.json';
import relationshipEdgesListRaw from '../../../testdata/contract-fixtures/relationship-edges-list.json';
import tagsListRaw from '../../../testdata/contract-fixtures/tags-list.json';
import { getActivities } from './activities';
import { listCircles } from './circles';
import { getContactDetail } from './contactDetail';
import { getContacts } from './contacts';
import { getDashboard } from './dashboard';
import { getLifeEvents } from './lifeEvents';
import { getContactNotes } from './notes';
import { getOccasionEvents } from './occasionEvents';
import { getRelationshipEdges } from './relationshipEdges';
import { getRemindersForContact } from './reminders';
import { listTags } from './tags';

afterEach(() => {
  vi.unstubAllGlobals();
});

function stubFetchOnce(body: unknown): void {
  vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce({ ok: true, json: async () => body }));
}

describe('contract fixtures: GET /contacts', () => {
  test('the raw capture has contacts as an array', () => {
    expect(Array.isArray(contactsListRaw.contacts)).toBe(true);
    expect(contactsListRaw.contacts.length).toBeGreaterThan(0);
  });

  test('getContacts parses a real list response', async () => {
    stubFetchOnce(contactsListRaw);

    const result = await getContacts({});

    expect(Array.isArray(result.contacts)).toBe(true);
    expect(result.contacts.length).toBe(contactsListRaw.contacts.length);
    // archived/is_favorite are documented as never-omitempty (default
    // false, always on the wire) -- pin every row actually carries them as
    // booleans, not undefined.
    for (const contact of result.contacts) {
      expect(typeof contact.archived).toBe('boolean');
      expect(typeof contact.is_favorite).toBe('boolean');
    }
  });
});

describe('contract fixtures: GET /contacts/:id/detail', () => {
  const arrayFields = [
    'notes',
    'activities',
    'completions',
    'reminders',
    'relationship_edges',
    'life_events',
    'agenda',
    'gifts',
    'field_values',
    'external_identities',
    'external_activities',
    'circles',
    'tags',
  ] as const;

  test('the raw capture has every collection block as an array', () => {
    for (const key of arrayFields) {
      expect(Array.isArray(contactDetailRaw[key]), `${key} should be an array`).toBe(true);
    }
    // immich is absent (not null) when the capturing user has no Immich
    // config -- pin the absence itself, not just "falsy".
    expect('immich' in contactDetailRaw).toBe(false);
  });

  test('getContactDetail parses a real, richly-populated response', async () => {
    stubFetchOnce(contactDetailRaw);

    const result = await getContactDetail(1);

    for (const key of arrayFields) {
      expect(Array.isArray(result[key])).toBe(true);
      expect(result[key]).toEqual(contactDetailRaw[key]);
    }
    expect(result.immich).toBeUndefined();
  });
});

describe('contract fixtures: GET /dashboard', () => {
  const arrayFields = [
    'birthdays',
    'random_contacts',
    'upcoming_reminders',
    'overdue',
    'favorites',
    'reach_out_suggestions',
    'contact_sync_conflicts',
  ] as const;

  test('the raw capture has every block as an array, never absent', () => {
    for (const key of arrayFields) {
      expect(Array.isArray(dashboardRaw[key]), `${key} should be an array`).toBe(true);
    }
  });

  test('getDashboard parses a real composite response', async () => {
    stubFetchOnce(dashboardRaw);

    const result = await getDashboard();

    for (const key of arrayFields) {
      expect(Array.isArray(result[key]), `${key} should be an array after parsing`).toBe(true);
      expect(result[key]).toEqual(dashboardRaw[key]);
    }
  });
});

// List endpoints: each fixture is the spec's `example:` for that endpoint's
// 200 response. Two checks per fixture, mirroring the ones above: the raw
// collection key is an array on the wire, and the real API function parses
// it through unchanged (row count and row contents preserved).
describe('contract fixtures: list endpoints', () => {
  const cases: Array<{
    name: string;
    raw: Record<string, unknown>;
    key: string;
    parse: () => Promise<unknown>;
    // Where the parsed rows live: the function returns the envelope, or the
    // bare array (getRemindersForContact unwraps `reminders`).
    unwrap: (parsed: unknown) => unknown;
  }> = [
    {
      name: 'GET /contacts/:id/notes',
      raw: contactNotesRaw,
      key: 'notes',
      parse: () => getContactNotes(11),
      unwrap: (p) => (p as { notes: unknown }).notes,
    },
    {
      name: 'GET /activities',
      raw: activitiesListRaw,
      key: 'activities',
      parse: () => getActivities({}),
      unwrap: (p) => (p as { activities: unknown }).activities,
    },
    {
      name: 'GET /contacts/:id/reminders',
      raw: contactRemindersRaw,
      key: 'reminders',
      parse: () => getRemindersForContact(11),
      unwrap: (p) => p,
    },
    {
      name: 'GET /relationship-edges',
      raw: relationshipEdgesListRaw,
      key: 'relationship_edges',
      parse: () => getRelationshipEdges({ contactId: '458bc9ba-b9a7-4853-a3f8-d9cd907bbc9f' }),
      unwrap: (p) => (p as { relationship_edges: unknown }).relationship_edges,
    },
    {
      name: 'GET /life-events',
      raw: lifeEventsListRaw,
      key: 'life_events',
      parse: () => getLifeEvents(),
      unwrap: (p) => (p as { life_events: unknown }).life_events,
    },
    {
      name: 'GET /circles',
      raw: circlesListRaw,
      key: 'circles',
      parse: () => listCircles(),
      unwrap: (p) => (p as { circles: unknown }).circles,
    },
    {
      name: 'GET /tags',
      raw: tagsListRaw,
      key: 'tags',
      parse: () => listTags(),
      unwrap: (p) => (p as { tags: unknown }).tags,
    },
    {
      name: 'GET /occasion-events',
      raw: occasionEventsListRaw,
      key: 'occasion_events',
      parse: () => getOccasionEvents(),
      unwrap: (p) => (p as { occasion_events: unknown }).occasion_events,
    },
  ];

  for (const c of cases) {
    test(`${c.name}: raw ${c.key} is a non-empty array and parses through`, async () => {
      const rows = c.raw[c.key];
      expect(Array.isArray(rows), `${c.key} should be an array`).toBe(true);
      expect((rows as unknown[]).length).toBeGreaterThan(0);

      stubFetchOnce(c.raw);
      const parsed = c.unwrap(await c.parse());

      expect(Array.isArray(parsed)).toBe(true);
      expect(parsed).toEqual(rows);
    });
  }

  // Trap-8 pins the conformance check found: nullable-on-the-wire fields the
  // TS types used to declare non-null.
  test('an unfiled note carries contact_id: null, not absent', () => {
    const unfiled = contactNotesRaw.notes.find(
      (n: { contact_id?: number | null }) => n.contact_id === null,
    );
    expect(unfiled).toBeDefined();
    expect('contact_id' in (unfiled as object)).toBe(true);
  });

  test('a reminder can carry by_mail: null (nullable column)', () => {
    expect(contactRemindersRaw.reminders[0].by_mail).toBeNull();
    expect(contactRemindersRaw.reminders[0].last_sent).toBeNull();
  });
});

// MAINT-02 (issue #491): the web client must tolerate unknown response
// fields. Adding a response field is the canonical additive (non-breaking)
// change, and a client that rejects unknown fields silently converts every
// additive change into a breaking one — so this property is a client-side
// requirement, asserted here. Each fixture is parsed as-is AND with a
// not-yet-existent field injected at the top level and on a nested contact;
// parsing must succeed and every known field must survive.
describe('MAINT-02: unknown response fields are ignored, never fatal', () => {
  const injectUnknownFields = <T>(value: T): T => {
    if (Array.isArray(value)) {
      return value.map((item) => injectUnknownFields(item)) as T;
    }
    if (value && typeof value === 'object') {
      const out: Record<string, unknown> = { ...(value as Record<string, unknown>) };
      if (!('__maint02_unknown_field__' in out)) {
        out['__maint02_unknown_field__'] = { nested: ['a', 1, null] };
      }
      for (const [k, v] of Object.entries(out)) {
        if (k !== '__maint02_unknown_field__') {
          out[k] = injectUnknownFields(v);
        }
      }
      return out as T;
    }
    return value;
  };

  test('GET /contacts parses a response with an unknown top-level and nested field', async () => {
    stubFetchOnce(injectUnknownFields(contactsListRaw));

    const result = await getContacts({});

    expect(result.contacts.length).toBe(contactsListRaw.contacts.length);
    for (const contact of result.contacts) {
      expect(typeof contact.archived).toBe('boolean');
      expect(typeof contact.is_favorite).toBe('boolean');
    }
  });

  test('GET /contacts/:id/detail parses a response with an unknown field', async () => {
    stubFetchOnce(injectUnknownFields(contactDetailRaw));

    const result = await getContactDetail(1);

    const arrayFields = [
      'notes',
      'activities',
      'completions',
      'reminders',
      'relationship_edges',
      'life_events',
      'agenda',
      'gifts',
      'field_values',
      'external_identities',
      'external_activities',
      'circles',
      'tags',
    ] as const;
    for (const key of arrayFields) {
      expect(Array.isArray(result[key]), `${key} should still be an array`).toBe(true);
    }
  });

  test('GET /dashboard parses a response with an unknown field', async () => {
    stubFetchOnce(injectUnknownFields(dashboardRaw));

    const result = await getDashboard();

    expect(Array.isArray(result.birthdays)).toBe(true);
    expect(Array.isArray(result.upcoming_reminders)).toBe(true);
    expect(Array.isArray(result.favorites)).toBe(true);
  });
});
