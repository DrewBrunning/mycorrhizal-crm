import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type ExternalActivity,
  type ExternalIdentity,
  getExternalActivities,
  getExternalIdentities,
} from '../api/externalLinks';
import { useExternalLinks } from './useExternalLinks';

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

// --- behavior pins (issue #1662) ---

test('starts empty and idle, requests 100 of each for the contact, and reports loading', async () => {
  let release!: () => void;
  const gate = new Promise<void>((res) => {
    release = res;
  });
  vi.mocked(getExternalIdentities).mockImplementation(async () => {
    await gate;
    return { external_identities: [identity('i-1')] } as never;
  });
  vi.mocked(getExternalActivities).mockResolvedValue({ external_activities: [] } as never);
  const { result } = renderHook(() => useExternalLinks('uid-1'));
  expect(result.current.identities).toEqual([]);
  expect(result.current.activities).toEqual([]);
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(getExternalIdentities).toHaveBeenCalledWith({ contactId: 'uid-1', limit: 100 });
  expect(getExternalActivities).toHaveBeenCalledWith({ contactId: 'uid-1', limit: 100 });
  await act(async () => {
    release();
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.identities.map((i) => i.id)).toEqual(['i-1']);
});

test('tolerates responses missing their lists, logs a failure by operation, and a retry clears it', async () => {
  vi.mocked(getExternalIdentities).mockResolvedValueOnce({} as never);
  vi.mocked(getExternalActivities).mockResolvedValueOnce({} as never);
  const { result } = renderHook(() => useExternalLinks('uid-1'));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.identities).toEqual([]);
  expect(result.current.activities).toEqual([]);

  vi.mocked(getExternalIdentities).mockRejectedValueOnce(new Error('down'));
  vi.mocked(getExternalActivities).mockResolvedValueOnce({} as never);
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBe('down');
  expect(console.error).toHaveBeenCalledWith('[fetching external links] Error:', expect.anything());

  vi.mocked(getExternalIdentities).mockResolvedValueOnce({} as never);
  vi.mocked(getExternalActivities).mockResolvedValueOnce({} as never);
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.error).toBeNull();
});

test('refresh follows the contact uid when it changes', async () => {
  vi.mocked(getExternalIdentities).mockResolvedValue({} as never);
  vi.mocked(getExternalActivities).mockResolvedValue({} as never);
  const { result, rerender } = renderHook(({ uid }) => useExternalLinks(uid), {
    initialProps: { uid: 'a' },
  });
  rerender({ uid: 'b' });
  await act(async () => {
    await result.current.refresh();
  });
  expect(getExternalIdentities).toHaveBeenLastCalledWith({ contactId: 'b', limit: 100 });
});

test('refresh without a contact uid does not fetch or flip loading', async () => {
  const { result } = renderHook(() => useExternalLinks(undefined));
  await act(async () => {
    await result.current.refresh();
  });
  expect(getExternalIdentities).not.toHaveBeenCalled();
  expect(getExternalActivities).not.toHaveBeenCalled();
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});
