import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  getNextcloudConfig,
  linkNextcloudItem,
  unlinkNextcloudItem,
  type WebDAVItem,
} from '../api/nextcloud';
import {
  getPaperlessConfig,
  linkPaperlessDocument,
  type PaperlessDocument,
  unlinkPaperlessDocument,
} from '../api/paperless';
import { getSeafileConfig, linkSeafileItem, unlinkSeafileItem } from '../api/seafile';
import { useContactFileLinks } from './useContactFileLinks';

vi.mock('../api/paperless', () => ({
  getPaperlessConfig: vi.fn(),
  linkPaperlessDocument: vi.fn(),
  unlinkPaperlessDocument: vi.fn(),
}));
vi.mock('../api/seafile', () => ({
  getSeafileConfig: vi.fn(),
  linkSeafileItem: vi.fn(),
  unlinkSeafileItem: vi.fn(),
}));
vi.mock('../api/nextcloud', () => ({
  getNextcloudConfig: vi.fn(),
  linkNextcloudItem: vi.fn(),
  unlinkNextcloudItem: vi.fn(),
}));

let refreshExternalLinks: ReturnType<typeof vi.fn<(uid: string) => Promise<void>>>;

beforeEach(() => {
  refreshExternalLinks = vi.fn(async () => {});
  vi.mocked(getPaperlessConfig).mockResolvedValue({ has_api_token: true } as never);
  vi.mocked(getSeafileConfig).mockResolvedValue({ has_api_token: true } as never);
  vi.mocked(getNextcloudConfig).mockResolvedValue({ has_app_password: true } as never);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

test('every configured system is enabled', async () => {
  const { result } = renderHook(() => useContactFileLinks('alice-uid', refreshExternalLinks));
  await waitFor(() =>
    expect(result.current.configured).toEqual({ paperless: true, seafile: true, nextcloud: true }),
  );
});

test('one failed config fetch hides only that system; a missing credential hides it too', async () => {
  vi.mocked(getSeafileConfig).mockRejectedValue(new Error('500'));
  vi.mocked(getNextcloudConfig).mockResolvedValue({ has_app_password: false } as never);
  const { result } = renderHook(() => useContactFileLinks('alice-uid', refreshExternalLinks));
  await waitFor(() => expect(result.current.configured.paperless).toBe(true));
  expect(result.current.configured).toEqual({ paperless: true, seafile: false, nextcloud: false });
});

test('link handlers call their integration then refresh identities', async () => {
  const { result } = renderHook(() => useContactFileLinks('alice-uid', refreshExternalLinks));
  await act(async () => {
    await result.current.handleLinkPaperless({ id: 42 } as PaperlessDocument);
    await result.current.handleLinkSeafile({
      repo_id: 'r1',
      path: '/a.pdf',
      name: 'a.pdf',
      type: 'file',
      size: 10,
      mtime: 5,
    } as never);
    await result.current.handleLinkNextcloud({
      path: '/b.pdf',
      name: 'b.pdf',
      type: 'file',
      size: 20,
      modified_at: '2024-01-01',
      file_id: 7,
      extra: 'dropped',
    } as unknown as WebDAVItem);
  });
  expect(linkPaperlessDocument).toHaveBeenCalledWith('alice-uid', 42);
  expect(linkSeafileItem).toHaveBeenCalledWith('alice-uid', {
    repo_id: 'r1',
    path: '/a.pdf',
    name: 'a.pdf',
    type: 'file',
    size: 10,
    mtime: 5,
  });
  expect(linkNextcloudItem).toHaveBeenCalledWith('alice-uid', {
    path: '/b.pdf',
    name: 'b.pdf',
    type: 'file',
    size: 20,
    modified_at: '2024-01-01',
    file_id: 7,
  });
  expect(refreshExternalLinks).toHaveBeenCalledTimes(3);
});

test.each([
  ['paperless', unlinkPaperlessDocument],
  ['seafile', unlinkSeafileItem],
  ['nextcloud', unlinkNextcloudItem],
] as const)('unlinking %s calls only its own endpoint', async (system, unlink) => {
  const { result } = renderHook(() => useContactFileLinks('alice-uid', refreshExternalLinks));
  await act(() => result.current.handleUnlink(system, 'ei-1'));
  expect(unlink).toHaveBeenCalledWith('alice-uid', 'ei-1');
  const all = [unlinkPaperlessDocument, unlinkSeafileItem, unlinkNextcloudItem];
  for (const other of all.filter((f) => f !== unlink)) {
    expect(other).not.toHaveBeenCalled();
  }
  expect(refreshExternalLinks).toHaveBeenCalledWith('alice-uid');
});

test('every handler is a no-op without a contact uid', async () => {
  const { result } = renderHook(() => useContactFileLinks(undefined, refreshExternalLinks));
  await act(async () => {
    await result.current.handleLinkPaperless({ id: 1 } as PaperlessDocument);
    await result.current.handleLinkSeafile({} as never);
    await result.current.handleLinkNextcloud({} as WebDAVItem);
    await result.current.handleUnlink('paperless', 'x');
  });
  expect(linkPaperlessDocument).not.toHaveBeenCalled();
  expect(linkSeafileItem).not.toHaveBeenCalled();
  expect(linkNextcloudItem).not.toHaveBeenCalled();
  expect(unlinkPaperlessDocument).not.toHaveBeenCalled();
  expect(refreshExternalLinks).not.toHaveBeenCalled();
});
