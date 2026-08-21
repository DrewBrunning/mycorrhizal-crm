import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  suggestHouseholdRelationships,
  listHouseholds,
  createHousehold,
  updateHousehold,
  deleteHousehold,
  addHouseholdMember,
  removeHouseholdMember,
  updateHouseholdMember,
  suggestAddressHouseholds,
  acceptAddressHouseholdSuggestion,
  dismissAddressHouseholdSuggestion,
  formatSuggestionAddress,
  Household,
  HouseholdMember,
  AddressHouseholdSuggestion,
  HOUSEHOLD_TYPES,
  HOUSEHOLD_ROLES,
} from './households';
import { getRelationshipEdges } from './relationshipEdges';
import { RelationshipEdge } from './relationshipEdges';

afterEach(() => {
  vi.unstubAllGlobals();
});

// T1: the trigger endpoint and
// the review-inbox query must agree on the shape of a generated suggestion —
// the trigger's output is exactly what feeds RelationshipEdgeList's suggested
// section on each member's contact page, for the first time against real data.

const generatedSuggestion: RelationshipEdge = {
  id: 'edge-1',
  source_id: 'alice-uid',
  target_id: 'bob-uid',
  type: 'spouse_of',
  directional: false,
  source: 'household-inferred',
  confidence: 0.8,
  status: 'suggested',
  sensitivity: 'normal',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('suggestHouseholdRelationships', () => {
  test('POSTs to the trigger endpoint and returns the generated edges', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Relationship suggestions generated',
        household_id: 'h1',
        suggested_edges: [generatedSuggestion],
        total: 1,
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await suggestHouseholdRelationships('h1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1/suggest-relationships');
    expect(init.method).toBe('POST');
    expect(result.total).toBe(1);
    expect(result.suggested_edges[0]).toEqual(generatedSuggestion);
    expect(result.suggested_edges[0].status).toBe('suggested');
  });
});

describe('review loop', () => {
  test('a generated suggestion surfaces through the status=suggested inbox query that feeds RelationshipEdgeList', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => ({
          relationship_edges: [generatedSuggestion],
          total: 1,
          page: 1,
          limit: 100,
        }),
      })
    );

    const response = await getRelationshipEdges({ contactId: 'alice-uid', status: 'suggested' });

    expect(response.relationship_edges[0]).toEqual(generatedSuggestion);
    expect(response.relationship_edges[0].status).toBe('suggested');
    expect(response.relationship_edges[0].source).toBe('household-inferred');
  });
});

const householdFixture: Household = {
  id: 'h1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  name: 'Miller Family',
  type: 'family_unit',
};

const memberFixture: HouseholdMember = {
  id: 1,
  household_id: 'h1',
  member_vcard_uid: 'alice-uid',
  role: 'adult',
  since: '2026-01-01',
};

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

function errorResponse() {
  return {
    ok: false,
    status: 404,
    statusText: 'Not Found',
    json: async () => ({
      error: { code: 'NOT_FOUND', message: 'Household not found' },
      request_id: 'req-1',
    }),
  };
}

describe('listHouseholds', () => {
  test('GETs /households with limit and include_members params', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({
        households: [householdFixture],
        total: 1,
        next_cursor: '',
        limit: 100,
        members: [memberFixture],
      })
    );
    vi.stubGlobal('fetch', fetchMock);

    const result = await listHouseholds({ limit: 100, include_members: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households?');
    expect(url).toContain('limit=100');
    expect(url).toContain('include_members=true');
    expect(init.method).toBeUndefined();
    expect(result.households[0]).toEqual(householdFixture);
    expect(result.members?.[0]).toEqual(memberFixture);
  });

  test('defaults to limit=100 and appends cursor when present', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(
      okResponse({ households: [], total: 0, next_cursor: 'c2', limit: 100 })
    );
    vi.stubGlobal('fetch', fetchMock);

    await listHouseholds({ cursor: 'c1' });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('limit=100');
    expect(url).toContain('cursor=c1');
    expect(url).not.toContain('include_members');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(listHouseholds()).rejects.toMatchObject({ code: 'NOT_FOUND', status: 404 });
  });
});

describe('createHousehold', () => {
  test('POSTs the household input and returns result.household', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ household: householdFixture }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await createHousehold({ name: 'Miller Family', type: 'family_unit' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ name: 'Miller Family', type: 'family_unit' });
    expect(result).toEqual(householdFixture);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(createHousehold({ name: 'x', type: 'other' })).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('updateHousehold', () => {
  test('PUTs the household input to /households/:id and returns the updated household', async () => {
    const updated = { ...householdFixture, name: 'Miller-Reyes' };
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(updated));
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateHousehold('h1', { name: 'Miller-Reyes', type: 'family_unit' });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ name: 'Miller-Reyes', type: 'family_unit' });
    expect(result).toEqual(updated);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(updateHousehold('h1', { name: 'x', type: 'other' })).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('deleteHousehold', () => {
  test('DELETEs /households/:id', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await deleteHousehold('h1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(deleteHousehold('h1')).rejects.toMatchObject({ code: 'NOT_FOUND', status: 404 });
  });
});

describe('addHouseholdMember', () => {
  test('POSTs the member input to /households/:id/members and returns result.member', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ member: memberFixture }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await addHouseholdMember('h1', {
      member_vcard_uid: 'alice-uid',
      role: 'adult',
      since: '2026-01-01',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1/members');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      member_vcard_uid: 'alice-uid',
      role: 'adult',
      since: '2026-01-01',
    });
    expect(result).toEqual(memberFixture);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(addHouseholdMember('h1', { member_vcard_uid: 'alice-uid' })).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('removeHouseholdMember', () => {
  test('DELETEs /households/:id/members/:vcard_uid', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await removeHouseholdMember('h1', 'alice-uid');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1/members/alice-uid');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(removeHouseholdMember('h1', 'alice-uid')).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('updateHouseholdMember', () => {
  test('PATCHes the role to /households/:id/members/:vcard_uid', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await updateHouseholdMember('h1', 'alice-uid', 'child');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/h1/members/alice-uid');
    expect(init.method).toBe('PATCH');
    expect(JSON.parse(init.body)).toEqual({ role: 'child' });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(updateHouseholdMember('h1', 'alice-uid', 'child')).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('suggestAddressHouseholds', () => {
  test('POSTs to /households/suggest-addresses and returns the suggestions', async () => {
    const suggestions = [
      {
        address_hash: 'hash-1',
        member_hash: 'mh-1',
        member_vcard_uids: ['alice-uid', 'bob-uid'],
        address: { components: [{ kind: 'name', value: '123 Main St' }], full: undefined },
      },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ suggestions, total: 1 }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await suggestAddressHouseholds();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/suggest-addresses');
    expect(init.method).toBe('POST');
    expect(result.total).toBe(1);
    expect(result.suggestions[0].member_vcard_uids).toEqual(['alice-uid', 'bob-uid']);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(suggestAddressHouseholds()).rejects.toMatchObject({ code: 'NOT_FOUND', status: 404 });
  });
});

describe('acceptAddressHouseholdSuggestion', () => {
  test('POSTs member uids plus name/type and returns the created household', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ household: householdFixture }));
    vi.stubGlobal('fetch', fetchMock);

    const result = await acceptAddressHouseholdSuggestion(['alice-uid', 'bob-uid'], {
      name: 'Miller Family',
      type: 'family_unit',
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/suggestions/accept');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({
      member_vcard_uids: ['alice-uid', 'bob-uid'],
      name: 'Miller Family',
      type: 'family_unit',
    });
    expect(result).toEqual(householdFixture);
  });

  test('omits name/type when no input is given', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse({ household: householdFixture }));
    vi.stubGlobal('fetch', fetchMock);

    await acceptAddressHouseholdSuggestion(['alice-uid']);

    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(init.body)).toEqual({ member_vcard_uids: ['alice-uid'] });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(acceptAddressHouseholdSuggestion(['alice-uid'])).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('dismissAddressHouseholdSuggestion', () => {
  test('POSTs the member uids to /households/suggestions/dismiss', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse());
    vi.stubGlobal('fetch', fetchMock);

    await dismissAddressHouseholdSuggestion(['alice-uid', 'bob-uid']);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/households/suggestions/dismiss');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ member_vcard_uids: ['alice-uid', 'bob-uid'] });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(dismissAddressHouseholdSuggestion(['alice-uid'])).rejects.toMatchObject({
      code: 'NOT_FOUND',
      status: 404,
    });
  });
});

describe('formatSuggestionAddress', () => {
  test('falls back to the full text when present', () => {
    expect(
      formatSuggestionAddress({
        full: '123 Main St, Springfield IL 62704',
        components: [{ kind: 'locality', value: 'Other' }],
      })
    ).toBe('123 Main St, Springfield IL 62704');
  });

  test('assembles components in name/locality/region/postcode/country order', () => {
    expect(
      formatSuggestionAddress({
        components: [
          { kind: 'postcode', value: '62704' },
          { kind: 'country', value: 'USA' },
          { kind: 'locality', value: 'Springfield' },
          { kind: 'region', value: 'IL' },
          { kind: 'name', value: '123 Main St' },
        ],
      })
    ).toBe('123 Main St, Springfield, IL, 62704, USA');
  });

  test('filters out blank components', () => {
    expect(
      formatSuggestionAddress({
        components: [
          { kind: 'name', value: '123 Main St' },
          { kind: 'locality', value: '' },
          { kind: 'region', value: '   ' },
          { kind: 'country', value: 'USA' },
        ],
      })
    ).toBe('123 Main St, USA');
  });

  test('uses the first occurrence when a kind repeats', () => {
    expect(
      formatSuggestionAddress({
        components: [
          { kind: 'name', value: 'First St' },
          { kind: 'name', value: 'Second St' },
        ],
      })
    ).toBe('First St');
  });

  test('returns an empty string for an empty address', () => {
    expect(formatSuggestionAddress(undefined as unknown as AddressHouseholdSuggestion['address'])).toBe('');
    expect(formatSuggestionAddress({})).toBe('');
  });
});

describe('household constants', () => {
  test('HOUSEHOLD_TYPES mirrors the backend oneof validator', () => {
    expect(HOUSEHOLD_TYPES).toEqual(['family_unit', 'roommates', 'other']);
  });

  test('HOUSEHOLD_ROLES lists the conventional role tokens', () => {
    expect(HOUSEHOLD_ROLES).toEqual(['adult', 'child', 'pet', 'roommate']);
  });
});
