import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  createPreference,
  deletePreference,
  getPreferences,
  type Preference,
  type PreferenceInput,
  updatePreference,
} from '../api/preferences';
import { usePreferences } from './usePreferences';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/preferences', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/preferences')>();
  return {
    ...actual,
    getPreferences: vi.fn(),
    createPreference: vi.fn(),
    updatePreference: vi.fn(),
    deletePreference: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getPreferences).mockReset();
  vi.mocked(createPreference).mockReset();
  vi.mocked(updatePreference).mockReset();
  vi.mocked(deletePreference).mockReset();
});

const preference: Preference = {
  id: 'p-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  entity_id: 'uid-1',
  category: 'food',
  value: 'pizza',
  sensitivity: 'normal',
};

test('loads preferences on mount', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getPreferences).toHaveBeenCalledWith({ entityId: 'uid-1', limit: 200 });
  expect(result.current.preferences).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('does not fetch without an entity id', async () => {
  const { result } = renderHook(() => usePreferences(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getPreferences).not.toHaveBeenCalled();
  expect(result.current.preferences).toEqual([]);
});

test('handleSave updates an existing preference', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: PreferenceInput = { entity_id: 'uid-1', category: 'food', value: 'sushi' };
  await act(async () => {
    await result.current.handleSave(preference, input);
  });

  expect(updatePreference).toHaveBeenCalledWith('p-1', input);
  expect(createPreference).not.toHaveBeenCalled();
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('handleSave creates when no preference exists', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [],
    total: 0,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  const input: PreferenceInput = { entity_id: 'uid-1', category: 'food', value: 'pizza' };
  await act(async () => {
    await result.current.handleSave(null, input);
  });

  expect(createPreference).toHaveBeenCalledWith(input);
  expect(updatePreference).not.toHaveBeenCalled();
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes and refreshes', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [preference],
    total: 1,
    next_cursor: '',
    limit: 200,
  });

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('p-1');
  });

  expect(deletePreference).toHaveBeenCalledWith('p-1');
  expect(getPreferences).toHaveBeenCalledTimes(2);
});

test('save errors notify through the notifier and rethrow', async () => {
  vi.mocked(getPreferences).mockResolvedValue({
    preferences: [],
    total: 0,
    next_cursor: '',
    limit: 200,
  });
  vi.mocked(createPreference).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => usePreferences('uid-1', { showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(
    result.current.handleSave(null, { entity_id: 'uid-1', category: 'food', value: 'x' }),
  ).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getPreferences).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => usePreferences('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.preferences).toEqual([]);
});
