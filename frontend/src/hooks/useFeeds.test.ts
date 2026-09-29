import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import { type Contact, getContactsByUid } from '../api/contacts';
import { type Feed, listFeeds, revokeAllFeeds, revokeFeed, rotateFeed } from '../api/feeds';
import { feedContactName, useFeeds } from './useFeeds';

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/feeds', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/feeds')>();
  return {
    ...actual,
    listFeeds: vi.fn(),
    rotateFeed: vi.fn(),
    revokeFeed: vi.fn(),
    revokeAllFeeds: vi.fn(),
  };
});
vi.mock('../api/contacts', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/contacts')>();
  return { ...actual, getContactsByUid: vi.fn() };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listFeeds).mockReset();
  vi.mocked(rotateFeed).mockReset();
  vi.mocked(revokeFeed).mockReset();
  vi.mocked(revokeAllFeeds).mockReset();
  vi.mocked(getContactsByUid).mockReset();
  vi.mocked(getContactsByUid).mockResolvedValue(new Map());
});

function feed(overrides: Partial<Feed> = {}): Feed {
  return {
    id: 'f1',
    name: 'Feed',
    kind: 'aggregate',
    entity_id: '',
    detail: 'headlines',
    created_at: '2026-01-01T00:00:00Z',
    last_accessed_at: null,
    ...overrides,
  };
}

const alice = { ID: 1, uid: 'alice-uid', firstname: 'Alice', lastname: 'Smith' } as Contact;

test('feedContactName joins first and last, falls back to nickname, else empty', () => {
  expect(feedContactName(alice)).toBe('Alice Smith');
  expect(feedContactName({ ...alice, firstname: '', lastname: '', nickname: 'Al' })).toBe('Al');
  expect(feedContactName({ ...alice, firstname: '', lastname: '' })).toBe('');
});

test('refresh loads feeds and resolves contact-feed names in one lookup', async () => {
  vi.mocked(listFeeds).mockResolvedValue([
    feed({ id: 'a' }),
    feed({ id: 'b', kind: 'contact', entity_id: 'alice-uid' }),
  ]);
  vi.mocked(getContactsByUid).mockResolvedValue(new Map([['alice-uid', alice]]));
  const { result } = renderHook(() => useFeeds());

  await act(async () => {
    await result.current.refresh();
  });

  expect(result.current.feeds).toHaveLength(2);
  expect(result.current.contactNames.get('alice-uid')).toBe('Alice Smith');
  expect(getContactsByUid).toHaveBeenCalledWith(['alice-uid']);
  expect(result.current.loading).toBe(false);
  expect(result.current.error).toBeNull();
});

test('refresh skips the contact lookup when there are no contact feeds', async () => {
  vi.mocked(listFeeds).mockResolvedValue([feed()]);
  const { result } = renderHook(() => useFeeds());
  await act(async () => {
    await result.current.refresh();
  });
  expect(getContactsByUid).not.toHaveBeenCalled();
});

test('a failed refresh records an error and stops loading', async () => {
  vi.mocked(listFeeds).mockRejectedValue(new Error('down'));
  const { result } = renderHook(() => useFeeds());
  await act(async () => {
    await result.current.refresh();
  });
  await waitFor(() => expect(result.current.error).not.toBeNull());
  expect(result.current.loading).toBe(false);
});

test('handleRotate returns the new URL and refreshes the list', async () => {
  vi.mocked(listFeeds).mockResolvedValue([]);
  vi.mocked(rotateFeed).mockResolvedValue({ feed: feed(), url: '/x?token=new' });
  const { result } = renderHook(() => useFeeds());

  let out: { url: string } | undefined;
  await act(async () => {
    out = await result.current.handleRotate('f1');
  });

  expect(rotateFeed).toHaveBeenCalledWith('f1');
  expect(out?.url).toBe('/x?token=new');
  expect(listFeeds).toHaveBeenCalledTimes(1);
});

test('handleRotate notifies and rethrows on failure', async () => {
  const showError = vi.fn();
  vi.mocked(rotateFeed).mockRejectedValue(new Error('nope'));
  const { result } = renderHook(() => useFeeds({ showError }));

  await act(async () => {
    await expect(result.current.handleRotate('f1')).rejects.toThrow('nope');
  });
  expect(showError).toHaveBeenCalled();
  expect(listFeeds).not.toHaveBeenCalled();
});

test('handleRevoke revokes then refreshes; failure notifies and rethrows', async () => {
  const showError = vi.fn();
  vi.mocked(listFeeds).mockResolvedValue([]);
  vi.mocked(revokeFeed).mockResolvedValueOnce(undefined);
  const { result } = renderHook(() => useFeeds({ showError }));

  await act(async () => {
    await result.current.handleRevoke('f1');
  });
  expect(revokeFeed).toHaveBeenCalledWith('f1');
  expect(listFeeds).toHaveBeenCalledTimes(1);

  vi.mocked(revokeFeed).mockRejectedValueOnce(new Error('nope'));
  await act(async () => {
    await expect(result.current.handleRevoke('f2')).rejects.toThrow('nope');
  });
  expect(showError).toHaveBeenCalledTimes(1);
});

test('handleRevokeAll returns the count and refreshes; failure notifies and rethrows', async () => {
  const showError = vi.fn();
  vi.mocked(listFeeds).mockResolvedValue([]);
  vi.mocked(revokeAllFeeds).mockResolvedValueOnce({ revoked: 4 });
  const { result } = renderHook(() => useFeeds({ showError }));

  let count = 0;
  await act(async () => {
    count = await result.current.handleRevokeAll();
  });
  expect(count).toBe(4);
  expect(listFeeds).toHaveBeenCalledTimes(1);

  vi.mocked(revokeAllFeeds).mockRejectedValueOnce(new Error('nope'));
  await act(async () => {
    await expect(result.current.handleRevokeAll()).rejects.toThrow('nope');
  });
  expect(showError).toHaveBeenCalledTimes(1);
});
