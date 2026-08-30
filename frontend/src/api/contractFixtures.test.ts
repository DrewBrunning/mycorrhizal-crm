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
import contactDetailRaw from '../../../testdata/contract-fixtures/contact-detail.json';
import contactsListRaw from '../../../testdata/contract-fixtures/contacts-list.json';
import dashboardRaw from '../../../testdata/contract-fixtures/dashboard.json';
import { getContactDetail } from './contactDetail';
import { getContacts } from './contacts';
import { getDashboard } from './dashboard';

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
