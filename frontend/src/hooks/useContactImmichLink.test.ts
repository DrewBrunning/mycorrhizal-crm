import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  getImmichConfig,
  getImmichContactSummary,
  type ImmichConfigResponse,
  type ImmichPersonSummary,
  linkImmichPerson,
  syncImmich,
  unlinkImmichPerson,
} from '../api/immich';
import { useContactImmichLink } from './useContactImmichLink';

vi.mock('../api/immich', () => ({
  getImmichConfig: vi.fn(),
  getImmichContactSummary: vi.fn(),
  linkImmichPerson: vi.fn(),
  unlinkImmichPerson: vi.fn(),
  syncImmich: vi.fn(),
}));

const config = (has_api_key: boolean): ImmichConfigResponse => ({
  base_url: 'https://immich.example',
  has_api_key,
  sync_enabled: true,
  last_sync_status: '',
  last_sync_error: '',
});

const summary: ImmichPersonSummary = {
  identity: {
    id: 'ei-1',
    entity_id: 'alice-uid',
    system: 'immich',
    external_id: 'person-1',
    sync_status: 'synced',
  },
  person_name: 'Alice',
  photo_count: 12,
};

let refreshExternalLinks: ReturnType<typeof vi.fn<(uid: string) => Promise<void>>>;

beforeEach(() => {
  refreshExternalLinks = vi.fn(async () => {});
  vi.mocked(getImmichConfig).mockResolvedValue(config(true));
  vi.mocked(getImmichContactSummary).mockResolvedValue(summary);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

test('reports Immich configured from the config fetch', async () => {
  const { result } = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await waitFor(() => expect(result.current.configured).toBe(true));
});

test('a config without a key, or a failed config fetch, reads as not configured', async () => {
  vi.mocked(getImmichConfig).mockResolvedValueOnce(config(false));
  const a = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await waitFor(() => expect(getImmichConfig).toHaveBeenCalled());
  expect(a.result.current.configured).toBe(false);

  vi.mocked(getImmichConfig).mockRejectedValueOnce(new Error('x'));
  const b = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await waitFor(() => expect(getImmichConfig).toHaveBeenCalledTimes(2));
  expect(b.result.current.configured).toBe(false);
});

test('refreshSummary loads the summary, preferring an override uid', async () => {
  const { result } = renderHook(() => useContactImmichLink(undefined, refreshExternalLinks));
  await act(() => result.current.refreshSummary());
  expect(getImmichContactSummary).not.toHaveBeenCalled();

  await act(() => result.current.refreshSummary('fresh-uid'));
  expect(getImmichContactSummary).toHaveBeenCalledWith('fresh-uid');
  expect(result.current.summary).toEqual(summary);
  expect(result.current.summaryLoading).toBe(false);
});

test('a failed summary fetch reads as not linked', async () => {
  const { result } = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await act(() => result.current.refreshSummary());
  vi.mocked(getImmichContactSummary).mockRejectedValue(new Error('x'));
  await act(() => result.current.refreshSummary());
  expect(result.current.summary).toBeNull();
  expect(result.current.summaryLoading).toBe(false);
});

test('link records the person, then refreshes identities and the summary', async () => {
  const { result } = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await act(() => result.current.handleLink({ id: 'person-1', name: 'Alice' } as never));
  expect(linkImmichPerson).toHaveBeenCalledWith('alice-uid', 'person-1', 'Alice');
  expect(refreshExternalLinks).toHaveBeenCalledWith('alice-uid');
  expect(result.current.summary).toEqual(summary);
});

test('unlink clears the summary and refreshes identities', async () => {
  const { result } = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await act(() => result.current.refreshSummary());
  await act(() => result.current.handleUnlink());
  expect(unlinkImmichPerson).toHaveBeenCalledWith('alice-uid');
  expect(result.current.summary).toBeNull();
  expect(refreshExternalLinks).toHaveBeenCalledWith('alice-uid');
});

test('sync toggles syncing and refreshes; a failed sync still clears the flag and rethrows', async () => {
  const { result } = renderHook(() => useContactImmichLink('alice-uid', refreshExternalLinks));
  await act(() => result.current.handleSync());
  expect(syncImmich).toHaveBeenCalled();
  expect(refreshExternalLinks).toHaveBeenCalledWith('alice-uid');
  expect(result.current.syncing).toBe(false);

  vi.mocked(syncImmich).mockRejectedValue(new Error('down'));
  await act(async () => {
    await expect(result.current.handleSync()).rejects.toThrow('down');
  });
  expect(result.current.syncing).toBe(false);
});

test('mutations are no-ops without a contact uid', async () => {
  const { result } = renderHook(() => useContactImmichLink(undefined, refreshExternalLinks));
  await act(async () => {
    await result.current.handleLink({ id: 'p', name: 'n' } as never);
    await result.current.handleUnlink();
    await result.current.handleSync();
  });
  expect(linkImmichPerson).not.toHaveBeenCalled();
  expect(unlinkImmichPerson).not.toHaveBeenCalled();
  expect(syncImmich).not.toHaveBeenCalled();
  expect(refreshExternalLinks).not.toHaveBeenCalled();
});
