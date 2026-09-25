import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createDataDecayPolicy,
  type DataDecayPolicy,
  deleteDataDecayPolicy,
  getDataDecayPolicies,
  getOverdueDataDecayPolicies,
  updateDataDecayPolicy,
  verifyDataDecayPolicy,
} from './dataDecayPolicies';

afterEach(() => {
  vi.unstubAllGlobals();
});

const policy: DataDecayPolicy = {
  id: 'policy-1',
  entity_id: 'alice-uid',
  interval_days: 365,
  last_verified_at: '2026-01-10T00:00:00Z',
  active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  health: {
    next_due: '2027-01-10T00:00:00Z',
    overdue_by: 0,
  },
};

describe('getDataDecayPolicies', () => {
  test('fetches by entity and returns the policy list', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data_decay_policies: [policy], total: 1, next_cursor: '', limit: 25 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getDataDecayPolicies('alice-uid');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/data-decay-policies');
    expect(url).toContain('entity_id=alice-uid');
    expect(init.method).toBeUndefined();
    expect(response.data_decay_policies[0]).toEqual(policy);
    expect(response.data_decay_policies[0].health?.overdue_by).toBe(0);
  });
});

describe('getOverdueDataDecayPolicies', () => {
  test('returns the overdue list verbatim (null-safe shape on the wire)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ overdue: [] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const response = await getOverdueDataDecayPolicies();
    expect(response.overdue).toEqual([]);
  });
});

describe('createDataDecayPolicy', () => {
  test('POSTs and unwraps the wrapped data_decay_policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Data decay policy created', data_decay_policy: policy }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createDataDecayPolicy({
      entity_id: 'alice-uid',
      interval_days: 365,
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/data-decay-policies');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body).interval_days).toBe(365);
    expect(result).toEqual(policy);
  });
});

describe('updateDataDecayPolicy', () => {
  test('PUTs to the policy id and returns the raw policy (NOT wrapped)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => policy,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateDataDecayPolicy('policy-1', {
      entity_id: 'alice-uid',
      interval_days: 90,
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/data-decay-policies/policy-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body).interval_days).toBe(90);
    expect(result).toEqual(policy);
  });
});

describe('deleteDataDecayPolicy', () => {
  test('DELETEs the policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Data decay policy deleted' }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await deleteDataDecayPolicy('policy-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/data-decay-policies/policy-1');
    expect(init.method).toBe('DELETE');
  });
});

describe('verifyDataDecayPolicy', () => {
  test('POSTs to the verify sub-route with no body and returns the raw policy', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => policy,
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await verifyDataDecayPolicy('policy-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/data-decay-policies/policy-1/verify');
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
    expect(result).toEqual(policy);
  });
});
