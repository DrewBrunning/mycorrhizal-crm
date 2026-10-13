import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createDataDecayPolicy,
  type DataDecayPoliciesResponse,
  type DataDecayPolicy,
  type DataDecayPolicyInput,
  deleteDataDecayPolicy,
  getDataDecayPolicies,
  updateDataDecayPolicy,
  verifyDataDecayPolicy,
} from '../api/dataDecayPolicies';
import { useDataDecayPolicy } from './useDataDecayPolicy';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/dataDecayPolicies', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/dataDecayPolicies')>();
  return {
    ...actual,
    getDataDecayPolicies: vi.fn(),
    createDataDecayPolicy: vi.fn(),
    updateDataDecayPolicy: vi.fn(),
    deleteDataDecayPolicy: vi.fn(),
    verifyDataDecayPolicy: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getDataDecayPolicies).mockReset();
  vi.mocked(createDataDecayPolicy).mockReset();
  vi.mocked(updateDataDecayPolicy).mockReset();
  vi.mocked(deleteDataDecayPolicy).mockReset();
  vi.mocked(verifyDataDecayPolicy).mockReset();
});

const policy: DataDecayPolicy = {
  id: 'pol-1',
  entity_id: 'uid-1',
  interval_days: 365,
  active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

function listResponse(policies: DataDecayPolicy[]): DataDecayPoliciesResponse {
  return { data_decay_policies: policies, total: policies.length, next_cursor: '', limit: 25 };
}

test('loads the policy on mount', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getDataDecayPolicies).toHaveBeenCalledWith('uid-1');
  expect(result.current.policy).toEqual(policy);
  expect(result.current.error).toBeNull();
});

test('takes the first policy when several exist', async () => {
  const second = { ...policy, id: 'pol-2', interval_days: 90 };
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy, second]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.policy).toEqual(policy);
});

test('does not fetch when no entity id is given', async () => {
  const { result } = renderHook(() => useDataDecayPolicy(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getDataDecayPolicies).not.toHaveBeenCalled();
  expect(result.current.policy).toBeNull();
});

test('refresh with an override entity id fetches that entity', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refresh('uid-9');
  });

  expect(getDataDecayPolicies).toHaveBeenLastCalledWith('uid-9');
});

test('handleSave creates when no policy is loaded for the entity', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([]));
  vi.mocked(createDataDecayPolicy).mockResolvedValue(policy);

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: DataDecayPolicyInput = { entity_id: 'uid-2', interval_days: 365 };
  await act(async () => {
    await result.current.handleSave(input);
  });

  expect(createDataDecayPolicy).toHaveBeenCalledWith(input);
  expect(updateDataDecayPolicy).not.toHaveBeenCalled();
  expect(getDataDecayPolicies).toHaveBeenCalledWith('uid-2');
});

test('handleSave updates when the loaded policy matches the entity', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: DataDecayPolicyInput = { entity_id: 'uid-1', interval_days: 90 };
  await act(async () => {
    await result.current.handleSave(input);
  });

  expect(updateDataDecayPolicy).toHaveBeenCalledWith('pol-1', input);
  expect(createDataDecayPolicy).not.toHaveBeenCalled();
});

test('handleDelete deletes the policy and clears it', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete();
  });

  expect(deleteDataDecayPolicy).toHaveBeenCalledWith('pol-1');
  expect(result.current.policy).toBeNull();
});

test('handleDelete is a no-op when no policy is loaded', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete();
  });

  expect(deleteDataDecayPolicy).not.toHaveBeenCalled();
});

test('delete errors notify through the notifier and rethrow', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));
  vi.mocked(deleteDataDecayPolicy).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useDataDecayPolicy('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete()).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getDataDecayPolicies).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.policy).toBeNull();
});

test('handleVerify calls the verify endpoint and stores the returned policy', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));
  const verified = { ...policy, last_verified_at: '2026-06-01T00:00:00Z' };
  vi.mocked(verifyDataDecayPolicy).mockResolvedValue(verified);

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleVerify();
  });

  expect(verifyDataDecayPolicy).toHaveBeenCalledWith('pol-1');
  expect(result.current.policy).toEqual(verified);
});

test('handleVerify is a no-op when no policy is loaded', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([]));

  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleVerify();
  });

  expect(verifyDataDecayPolicy).not.toHaveBeenCalled();
});

test('verify errors notify through the notifier and rethrow', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));
  vi.mocked(verifyDataDecayPolicy).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useDataDecayPolicy('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleVerify()).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

// --- behavior pins (issue #1662) ---

test('a failed load is logged by operation, and the next refresh clears the error', async () => {
  vi.mocked(getDataDecayPolicies).mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.error).toBe('offline'));
  expect(console.error).toHaveBeenCalledWith(
    '[loading data decay policy] Error:',
    expect.anything(),
  );

  vi.mocked(getDataDecayPolicies).mockResolvedValueOnce(listResponse([policy]));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBeNull();
  expect(result.current.policy).toEqual(policy);
});

test('a list that comes back empty (or without the key) clears the policy', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValueOnce(listResponse([policy]));
  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.policy).toEqual(policy));

  vi.mocked(getDataDecayPolicies).mockResolvedValueOnce({} as DataDecayPoliciesResponse);
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.policy).toBeNull();
});

test('saving for a different entity than the loaded policy creates instead of updating', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));
  vi.mocked(createDataDecayPolicy).mockResolvedValue(policy);
  const { result } = renderHook(() => useDataDecayPolicy('uid-1'));
  await waitFor(() => expect(result.current.policy).toEqual(policy));
  await act(async () => {
    await result.current.handleSave({ entity_id: 'uid-2' } as DataDecayPolicyInput);
  });
  expect(createDataDecayPolicy).toHaveBeenCalledWith({ entity_id: 'uid-2' });
  expect(updateDataDecayPolicy).not.toHaveBeenCalled();
  expect(getDataDecayPolicies).toHaveBeenLastCalledWith('uid-2');
});

test('delete and verify failures log their operations, notify, and rethrow', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([policy]));
  vi.mocked(deleteDataDecayPolicy).mockRejectedValue(new Error('d'));
  vi.mocked(verifyDataDecayPolicy).mockRejectedValue(new Error('v'));
  const showError = vi.fn();
  const { result } = renderHook(() => useDataDecayPolicy('uid-1', { showError }));
  await waitFor(() => expect(result.current.policy).toEqual(policy));
  await act(async () => {
    await expect(result.current.handleDelete()).rejects.toThrow('d');
    await expect(result.current.handleVerify()).rejects.toThrow('v');
  });
  expect(showError.mock.calls.map((c) => c[0])).toEqual(['d', 'v']);
  expect(console.error).toHaveBeenCalledWith(
    '[deleting data decay policy] Error:',
    expect.anything(),
  );
  expect(console.error).toHaveBeenCalledWith(
    '[confirming contact info is current] Error:',
    expect.anything(),
  );
  expect(result.current.policy).toEqual(policy);
});

test('refetches for the new entity when the entity id changes', async () => {
  vi.mocked(getDataDecayPolicies).mockResolvedValue(listResponse([]));
  const { rerender } = renderHook(({ id }) => useDataDecayPolicy(id), {
    initialProps: { id: 'uid-1' },
  });
  await waitFor(() => expect(getDataDecayPolicies).toHaveBeenCalledWith('uid-1'));
  rerender({ id: 'uid-2' });
  await waitFor(() => expect(getDataDecayPolicies).toHaveBeenLastCalledWith('uid-2'));
});
