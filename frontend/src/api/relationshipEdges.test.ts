import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import {
  acceptRelationshipEdge,
  createRelationshipEdge,
  deleteRelationshipEdge,
  getDisplayLabel,
  getEffectiveType,
  getOtherPartyId,
  getRelationshipEdges,
  listSuggestedRelationshipEdges,
  RELATIONSHIP_EDGE_TYPE_TOKENS,
  RELATIONSHIP_EDGE_TYPES,
  type RelationshipEdge,
  type RelationshipEdgeType,
  rejectRelationshipEdge,
  suggestRelationshipEdges,
  toBackendType,
  updateRelationshipEdge,
} from './relationshipEdges';

// the direction-
// resolution logic below is the highest-risk code in this WP -- getting it
// backwards would silently create/display incorrectly-labeled relationships.
// Every asymmetric pair is asserted explicitly in both directions; every
// symmetric token is asserted to read identically regardless of viewed side.

function makeEdge(overrides: Partial<RelationshipEdge> = {}): RelationshipEdge {
  return {
    id: 'edge-1',
    source_id: 'alice',
    target_id: 'bob',
    type: 'parent_of',
    directional: true,
    source: 'user-confirmed',
    confidence: 1.0,
    status: 'confirmed',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

describe('getEffectiveType / getDisplayLabel', () => {
  test('parent_of/child_of: viewed=target reads type directly, viewed=source reads the inverse', () => {
    // Alice is Bob's parent (source=alice, type=parent_of, target=bob).
    const edge = makeEdge({ source_id: 'alice', target_id: 'bob', type: 'parent_of' });
    // Viewing Bob's page: the other party (Alice) is his parent.
    expect(getEffectiveType(edge, 'bob')).toBe('parent_of');
    expect(getDisplayLabel(edge, 'bob')).toBe('relationships.types.parent_of');
    // Viewing Alice's page: the other party (Bob) is her child.
    expect(getEffectiveType(edge, 'alice')).toBe('child_of');
    expect(getDisplayLabel(edge, 'alice')).toBe('relationships.types.child_of');
  });

  test('mentor_of/mentee_of: both directions', () => {
    // Alice mentors Bob (source=alice, type=mentee_of would mean alice is
    // the mentee -- construct explicitly: type describes source's role, so
    // "alice is bob's mentor" is type: mentor_of, source: alice, target: bob).
    const edge = makeEdge({ source_id: 'alice', target_id: 'bob', type: 'mentor_of' });
    // Viewing Bob's page: the other party (Alice) is his mentor.
    expect(getEffectiveType(edge, 'bob')).toBe('mentor_of');
    // Viewing Alice's page: the other party (Bob) is her mentee.
    expect(getEffectiveType(edge, 'alice')).toBe('mentee_of');
  });

  test('owns/owned_by: both directions, curated labels not derived from token name', () => {
    // Alice owns Bob (a pet) -- source=alice, type=owns, target=bob.
    const edge = makeEdge({ source_id: 'alice', target_id: 'bob', type: 'owns' });
    // Viewing Bob's (the pet's) page: the other party (Alice) is the owner.
    expect(getEffectiveType(edge, 'bob')).toBe('owns');
    expect(getDisplayLabel(edge, 'bob')).toBe('relationships.types.owns');
    // Viewing Alice's page: the other party (Bob) is her pet.
    expect(getEffectiveType(edge, 'alice')).toBe('owned_by');
    expect(getDisplayLabel(edge, 'alice')).toBe('relationships.types.owned_by');
  });

  const symmetricTokens: RelationshipEdgeType[] = [
    'spouse_of',
    'sibling_of',
    'friend_of',
    'roommate_of',
    'coworker_of',
    'partner_of',
    'co_parent_of',
    'gets_along_with',
    'conflicts_with',
    'related_to',
  ];
  test.each(symmetricTokens)(
    '%s is symmetric: label identical regardless of viewed side',
    (token) => {
      const edge = makeEdge({ source_id: 'alice', target_id: 'bob', type: token });
      expect(getEffectiveType(edge, 'alice')).toBe(token);
      expect(getEffectiveType(edge, 'bob')).toBe(token);
      expect(RELATIONSHIP_EDGE_TYPES[token].symmetric).toBe(true);
    },
  );

  test('unregistered type falls back to related_to without throwing', () => {
    const edge = makeEdge({ type: 'some_future_token_this_frontend_mirror_does_not_know_about' });
    expect(() => getEffectiveType(edge, 'bob')).not.toThrow();
    expect(getEffectiveType(edge, 'bob')).toBe('related_to');
    // Viewed=source with an unregistered type: metaFor's own fallback makes
    // this related_to's inverse, which is itself related_to.
    expect(getEffectiveType(edge, 'alice')).toBe('related_to');
  });

  test('every registered token has a valid inverse entry (self-consistency)', () => {
    for (const token of RELATIONSHIP_EDGE_TYPE_TOKENS) {
      const meta = RELATIONSHIP_EDGE_TYPES[token];
      expect(RELATIONSHIP_EDGE_TYPES[meta.inverse]).toBeDefined();
      // Symmetric tokens must be their own inverse.
      if (meta.symmetric) {
        expect(meta.inverse).toBe(token);
      }
    }
  });
});

describe('toBackendType', () => {
  test('identity when viewed is not source (create-mode path)', () => {
    for (const token of RELATIONSHIP_EDGE_TYPE_TOKENS) {
      expect(toBackendType(token, false)).toBe(token);
    }
  });

  test('round-trips against getEffectiveType for every token, both directions', () => {
    for (const token of RELATIONSHIP_EDGE_TYPE_TOKENS) {
      const edge = makeEdge({ source_id: 'alice', target_id: 'bob', type: token });

      const effectiveAtTarget = getEffectiveType(edge, 'bob'); // viewed is target
      expect(toBackendType(effectiveAtTarget, false)).toBe(token);

      const effectiveAtSource = getEffectiveType(edge, 'alice'); // viewed is source
      expect(toBackendType(effectiveAtSource, true)).toBe(token);
    }
  });
});

describe('getOtherPartyId', () => {
  test('returns target when viewed is source, and vice versa', () => {
    const edge = makeEdge({ source_id: 'alice', target_id: 'bob' });
    expect(getOtherPartyId(edge, 'alice')).toBe('bob');
    expect(getOtherPartyId(edge, 'bob')).toBe('alice');
  });
});

// --- fetch-backed CRUD/suggestion endpoints -----------------------------------

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

describe('getRelationshipEdges', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const listBody = {
    relationship_edges: [makeEdge()],
    total: 1,
    next_cursor: 'cursor-2',
    limit: 100,
  };

  test('sends contact_id, limit, status, type and cursor when given', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(listBody));

    const result = await getRelationshipEdges({
      contactId: 'bob',
      status: 'suggested',
      type: 'parent_of',
      cursor: 'cursor-1',
      limit: 10,
    });

    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges?');
    expect(url).toContain('contact_id=bob');
    expect(url).toContain('limit=10');
    expect(url).toContain('status=suggested');
    expect(url).toContain('type=parent_of');
    expect(url).toContain('cursor=cursor-1');
    expect(init.headers).toEqual({ 'Content-Type': 'application/json' });
    expect(result).toEqual(listBody);
  });

  test('defaults limit to 100 and omits status/type/cursor when absent', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(listBody));
    await getRelationshipEdges({ contactId: 'bob' });
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('limit=100');
    expect(url).not.toContain('status=');
    expect(url).not.toContain('type=');
    expect(url).not.toContain('cursor=');
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(getRelationshipEdges({ contactId: 'bob' })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('createRelationshipEdge', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('POSTs the input and unwraps { relationship_edge }', async () => {
    const edge = makeEdge();
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(
      okResponse({ relationship_edge: edge }),
    );
    const input = { target_id: 'bob', type: 'parent_of' as const };
    const result = await createRelationshipEdge(input);

    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(edge);
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(
      createRelationshipEdge({ target_id: 'bob', type: 'parent_of' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR' });
  });
});

describe('updateRelationshipEdge', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('PUTs to /relationship-edges/:id and returns the raw (unwrapped) edge', async () => {
    const edge = makeEdge();
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(edge));
    const input = { target_id: 'bob', type: 'parent_of' as const };
    const result = await updateRelationshipEdge('edge-1', input);

    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges/edge-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual(input);
    expect(result).toEqual(edge);
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(
      updateRelationshipEdge('edge-1', { target_id: 'bob', type: 'parent_of' }),
    ).rejects.toMatchObject({ code: 'VALIDATION_ERROR' });
  });
});

describe('deleteRelationshipEdge / rejectRelationshipEdge', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('DELETEs /relationship-edges/:id', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse({}));
    await deleteRelationshipEdge('edge-1');
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges/edge-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(deleteRelationshipEdge('edge-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });

  test('rejectRelationshipEdge is literally deleteRelationshipEdge', () => {
    expect(rejectRelationshipEdge).toBe(deleteRelationshipEdge);
  });
});

describe('acceptRelationshipEdge', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('PATCHes /relationship-edges/:id/accept and returns the raw edge', async () => {
    const edge = makeEdge({ status: 'confirmed' });
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(edge));
    const result = await acceptRelationshipEdge('edge-1');
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges/edge-1/accept');
    expect(init.method).toBe('PATCH');
    expect(result).toEqual(edge);
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(acceptRelationshipEdge('edge-1')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });
});

describe('suggestRelationshipEdges', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test('POSTs /relationship-edges/suggest and returns the response', async () => {
    const body = { message: 'ok', suggested_edges: [makeEdge()], total: 1 };
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(body));
    const result = await suggestRelationshipEdges();
    const [url, init] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('/relationship-edges/suggest');
    expect(init.method).toBe('POST');
    expect(result).toEqual(body);
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(suggestRelationshipEdges()).rejects.toMatchObject({ code: 'VALIDATION_ERROR' });
  });
});

describe('listSuggestedRelationshipEdges', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn());
  });
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  const listBody = {
    relationship_edges: [makeEdge({ status: 'suggested' })],
    total: 1,
    next_cursor: '',
    limit: 100,
  };

  test('always sends status=suggested, plus cursor and limit when given', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(listBody));
    await listSuggestedRelationshipEdges({ cursor: 'cursor-1', limit: 5 });
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('status=suggested');
    expect(url).toContain('cursor=cursor-1');
    expect(url).toContain('limit=5');
  });

  test('defaults limit to 100 and omits cursor with no params', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(okResponse(listBody));
    await listSuggestedRelationshipEdges();
    const [url] = (fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(url).toContain('limit=100');
    expect(url).not.toContain('cursor=');
  });

  test('throws an ApiError when the response is not ok', async () => {
    (fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(errorResponse());
    await expect(listSuggestedRelationshipEdges()).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
    });
  });
});
