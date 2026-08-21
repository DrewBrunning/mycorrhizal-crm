import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useExternalLinks } from './useExternalLinks';
import {
  getExternalIdentities,
  getExternalActivities,
  ExternalIdentity,
  ExternalActivity,
} from '../api/externalLinks';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/externalLinks', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/externalLinks')>();
  return { ...actual, getExternalIdentities: vi.fn(), getExternalActivities: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(getExternalIdentities).mockReset();
  vi.mocked(getExternalActivities).mockReset();
});

function identity(id: string): ExternalIdentity {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    system: 'nextcloud',
    external_id: 'nc-1',
    sync_status: 'synced',
  };
}

function activity(id: string): ExternalActivity {
  return {
    id,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    entity_id: 'uid-1',
    source_system: 'immich',
    external_id: 'im-1',
    type: 'photo',
    occurred_at: '2026-01-01T00:00:00Z',
    provenance: 'external',
    sync_state: 'synced',
  };
}

test('loads identities and enrichment activities for the contact', async () => {
  vi.mocked(getExternalIdentities).mockResolvedValue({
    external_identities: [identity('e-1')],
    total: 1,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(getExternalActivities).mockResolvedValue({
    external_activities: [activity('a-1')],
    total: 1,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useExternalLinks('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(getExternalIdentities).toHaveBeenCalledWith({ contactId: 'uid-1', limit: 100 });
  expect(getExternalActivities).toHaveBeenCalledWith({ contactId: 'uid-1', limit: 100 });
  expect(result.current.identities).toHaveLength(1);
  expect(result.current.activities).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('does not fetch without a contact uid', async () => {
  const { result } = renderHook(() => useExternalLinks(undefined));
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(getExternalIdentities).not.toHaveBeenCalled();
  expect(getExternalActivities).not.toHaveBeenCalled();
});

test('refresh accepts an override uid', async () => {
  vi.mocked(getExternalIdentities).mockResolvedValue({
    external_identities: [],
    total: 0,
    next_cursor: '',
    limit: 100,
  });
  vi.mocked(getExternalActivities).mockResolvedValue({
    external_activities: [],
    total: 0,
    next_cursor: '',
    limit: 100,
  });

  const { result } = renderHook(() => useExternalLinks('uid-1'));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.refresh('uid-9');
  });

  expect(getExternalIdentities).toHaveBeenLastCalledWith({ contactId: 'uid-9', limit: 100 });
  expect(getExternalActivities).toHaveBeenLastCalledWith({ contactId: 'uid-9', limit: 100 });
});

test('sets error when the fetch fails', async () => {
  vi.mocked(getExternalIdentities).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useExternalLinks('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.error).toBe('boom');
  expect(result.current.identities).toEqual([]);
  expect(result.current.activities).toEqual([]);
});
