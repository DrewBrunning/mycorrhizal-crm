import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  type CadencePolicy,
  createCadencePolicy,
  deleteCadencePolicy,
  getCadencePolicies,
  getOverdueCadences,
  updateCadencePolicy,
} from './cadencePolicies';

afterEach(() => {
  vi.unstubAllGlobals();
});

const policy: CadencePolicy = {
  id: 'policy-1',
  entity_id: 'alice-uid',
  target_interval_days: 30,
  qualifying_types: ['call', 'visit'],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  health: {
    has_qualifying_interaction: true,
    last_interaction: '2026-01-10T00:00:00Z',
    next_due: '2026-02-09T00:00:00Z',
    overdue_by: 0,
  },
};

describe('getCadencePolicies', () => {
  test('fetches by entity and returns the policy list', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ cadence_policies: [policy], total: 1, next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getCadencePolicies('alice-uid');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/cadence-policies');
    expect(url).toContain('entity_id=alice-uid');
    expect(init.method).toBeUndefined();
    expect(response.cadence_policies[0]).toEqual(policy);
    expect(response.cadence_policies[0].health?.overdue_by).toBe(0);
  });
});

describe('getOverdueCadences', () => {
  test('returns the overdue list verbatim (null-safe shape on the wire)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ overdue: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getOverdueCadences();
    expect(response.overdue).toEqual([]);
  });
});

describe('createCadencePolicy', () => {
  test('POSTs and unwraps the wrapped cadence_policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Cadence policy created', cadence_policy: policy }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createCadencePolicy({
      entity_id: 'alice-uid',
      target_interval_days: 30,
      qualifying_types: ['call'],
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/cadence-policies');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body).target_interval_days).toBe(30);
    expect(result).toEqual(policy);
  });
});

describe('updateCadencePolicy', () => {
  test('PUTs to the policy id and returns the raw policy (NOT wrapped)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => policy,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateCadencePolicy('policy-1', {
      entity_id: 'alice-uid',
      target_interval_days: 60,
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/cadence-policies/policy-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body).target_interval_days).toBe(60);
    expect(result).toEqual(policy);
  });
});

describe('deleteCadencePolicy', () => {
  test('DELETEs the policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Cadence policy deleted' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await deleteCadencePolicy('policy-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/cadence-policies/policy-1');
    expect(init.method).toBe('DELETE');
  });
});
