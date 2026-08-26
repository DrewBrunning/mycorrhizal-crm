import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type CadencePoliciesResponse,
  type CadencePolicy,
  type CadencePolicyInput,
  createCadencePolicy,
  deleteCadencePolicy,
  getCadencePolicies,
  updateCadencePolicy,
} from '../api/cadencePolicies';
import { useCadencePolicy } from './useCadencePolicy';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/cadencePolicies', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/cadencePolicies')>();
  return {
    ...actual,
    getCadencePolicies: vi.fn(),
    createCadencePolicy: vi.fn(),
    updateCadencePolicy: vi.fn(),
    deleteCadencePolicy: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getCadencePolicies).mockReset();
  vi.mocked(createCadencePolicy).mockReset();
  vi.mocked(updateCadencePolicy).mockReset();
  vi.mocked(deleteCadencePolicy).mockReset();
});

const policy: CadencePolicy = {
  id: 'pol-1',
  entity_id: 'uid-1',
  target_interval_days: 14,
  qualifying_types: [],
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

function listResponse(policies: CadencePolicy[]): CadencePoliciesResponse {
  return { cadence_policies: policies, total: policies.length, next_cursor: '', limit: 25 };
}

test('loads the policy on mount', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getCadencePolicies).toHaveBeenCalledWith('uid-1');
  expect(result.current.policy).toEqual(policy);
  expect(result.current.error).toBeNull();
});

test('takes the first policy when several exist', async () => {
  const second = { ...policy, id: 'pol-2', target_interval_days: 30 };
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([policy, second]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.policy).toEqual(policy);
});

test('does not fetch when no entity id is given', async () => {
  const { result } = renderHook(() => useCadencePolicy(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getCadencePolicies).not.toHaveBeenCalled();
  expect(result.current.policy).toBeNull();
});

test('refresh with an override entity id fetches that entity', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refresh('uid-9');
  });

  expect(getCadencePolicies).toHaveBeenLastCalledWith('uid-9');
});

test('handleSave creates when no policy is loaded for the entity', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([]));
  vi.mocked(createCadencePolicy).mockResolvedValue(policy);

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: CadencePolicyInput = { entity_id: 'uid-2', target_interval_days: 30 };
  await act(async () => {
    await result.current.handleSave(input);
  });

  expect(createCadencePolicy).toHaveBeenCalledWith(input);
  expect(updateCadencePolicy).not.toHaveBeenCalled();
  expect(getCadencePolicies).toHaveBeenCalledWith('uid-2');
});

test('handleSave updates when the loaded policy matches the entity', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: CadencePolicyInput = { entity_id: 'uid-1', target_interval_days: 30 };
  await act(async () => {
    await result.current.handleSave(input);
  });

  expect(updateCadencePolicy).toHaveBeenCalledWith('pol-1', input);
  expect(createCadencePolicy).not.toHaveBeenCalled();
});

test('handleDelete deletes the policy and clears it', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([policy]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete();
  });

  expect(deleteCadencePolicy).toHaveBeenCalledWith('pol-1');
  expect(result.current.policy).toBeNull();
});

test('handleDelete is a no-op when no policy is loaded', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([]));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete();
  });

  expect(deleteCadencePolicy).not.toHaveBeenCalled();
});

test('delete errors notify through the notifier and rethrow', async () => {
  vi.mocked(getCadencePolicies).mockResolvedValue(listResponse([policy]));
  vi.mocked(deleteCadencePolicy).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useCadencePolicy('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete()).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getCadencePolicies).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useCadencePolicy('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.policy).toBeNull();
});
