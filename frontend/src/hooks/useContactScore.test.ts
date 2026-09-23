import { cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type ContactScoreResponse, getContactScore } from '../api/contactScore';
import { useContactScore } from './useContactScore';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/contactScore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contactScore')>();
  return {
    ...actual,
    getContactScore: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getContactScore).mockReset();
});

const score: ContactScoreResponse = {
  contact_id: 1,
  score: 72,
  band: 'moss',
  recency: { value: 80, weight: 35, reason: 'Last qualifying interaction 5 day(s) ago' },
  frequency: { value: 60, weight: 20, reason: 'Roughly on pace' },
  closeness: { value: 85, weight: 20, reason: 'Marked as close' },
  reach_out: { value: 100, weight: 15, reason: 'No reach-out overdue' },
  last_updated: { value: 90, weight: 10, reason: 'Profile recently updated' },
};

test('loads the score on mount', async () => {
  vi.mocked(getContactScore).mockResolvedValue(score);

  const { result } = renderHook(() => useContactScore(1));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactScore).toHaveBeenCalledWith(1);
  expect(result.current.score).toEqual(score);
  expect(result.current.error).toBeNull();
});

test('does not fetch when no contact id is given', async () => {
  const { result } = renderHook(() => useContactScore(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getContactScore).not.toHaveBeenCalled();
  expect(result.current.score).toBeNull();
});

test('refetches when the contact id changes', async () => {
  vi.mocked(getContactScore).mockResolvedValue(score);

  const { result, rerender } = renderHook(({ id }) => useContactScore(id), {
    initialProps: { id: 1 },
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(getContactScore).toHaveBeenLastCalledWith(1);

  rerender({ id: 2 });
  await waitFor(() => expect(getContactScore).toHaveBeenLastCalledWith(2));
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getContactScore).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useContactScore(1));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.score).toBeNull();
});

test('refreshScore re-fetches on demand', async () => {
  vi.mocked(getContactScore).mockResolvedValue(score);

  const { result } = renderHook(() => useContactScore(1));
  await waitFor(() => expect(result.current.loading).toBe(false));

  vi.mocked(getContactScore).mockClear();
  await result.current.refreshScore();

  expect(getContactScore).toHaveBeenCalledWith(1);
});
