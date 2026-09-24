import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createOccasionObligation,
  deleteOccasionObligation,
  getOccasionObligations,
  type OccasionObligation,
  type OccasionObligationInput,
  updateOccasionObligation,
} from '../api/occasionObligations';
import { useOccasionObligations } from './useOccasionObligations';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/occasionObligations', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/occasionObligations')>();
  return {
    ...actual,
    getOccasionObligations: vi.fn(),
    createOccasionObligation: vi.fn(),
    updateOccasionObligation: vi.fn(),
    deleteOccasionObligation: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getOccasionObligations).mockReset();
  vi.mocked(createOccasionObligation).mockReset();
  vi.mocked(updateOccasionObligation).mockReset();
  vi.mocked(deleteOccasionObligation).mockReset();
});

const obligation: OccasionObligation = {
  id: 'o-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'uid-1',
  kind: 'card',
  label: 'Christmas card',
  lead_time_days: 0,
  active: true,
  sensitivity: 'normal',
};

test('loads obligations on mount', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [obligation],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionObligations('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getOccasionObligations).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 100 });
  expect(result.current.obligations).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => useOccasionObligations(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getOccasionObligations).not.toHaveBeenCalled();
  expect(result.current.obligations).toEqual([]);
});

test('handleSave updates an existing obligation', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [obligation],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionObligations('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: OccasionObligationInput = { entity_id: 'uid-1', kind: 'card', label: 'New label' };
  await act(async () => {
    await result.current.handleSave(obligation, input);
  });

  expect(updateOccasionObligation).toHaveBeenCalledWith('o-1', input);
  expect(createOccasionObligation).not.toHaveBeenCalled();
  expect(getOccasionObligations).toHaveBeenCalledTimes(2);
});

test('handleSave creates when no obligation exists', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [],
    total: 0,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionObligations('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: OccasionObligationInput = { entity_id: 'uid-1', kind: 'card', label: 'New card' };
  await act(async () => {
    await result.current.handleSave(null, input);
  });

  expect(createOccasionObligation).toHaveBeenCalledWith(input);
  expect(updateOccasionObligation).not.toHaveBeenCalled();
  expect(getOccasionObligations).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes and refreshes', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [obligation],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useOccasionObligations('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('o-1');
  });

  expect(deleteOccasionObligation).toHaveBeenCalledWith('o-1');
  expect(getOccasionObligations).toHaveBeenCalledTimes(2);
});

test('delete errors notify through the notifier and rethrow', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [obligation],
    total: 1,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(deleteOccasionObligation).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useOccasionObligations('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete('o-1')).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
  // A failed delete must not refresh (it would clobber the list with a
  // second fetch that races the still-live obligation).
  expect(getOccasionObligations).toHaveBeenCalledTimes(1);
});

test('save errors notify through the notifier and rethrow', async () => {
  vi.mocked(getOccasionObligations).mockResolvedValue({
    occasion_obligations: [],
    total: 0,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(createOccasionObligation).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useOccasionObligations('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    result.current.handleSave(null, { entity_id: 'uid-1', kind: 'card', label: 'x' }),
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getOccasionObligations).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useOccasionObligations('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.obligations).toEqual([]);
});
