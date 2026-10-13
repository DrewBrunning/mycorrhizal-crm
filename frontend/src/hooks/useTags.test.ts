import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import {
  type ContactTag,
  createTag,
  deleteTag,
  listTags,
  type Tag,
  type TagListResponse,
  updateTag,
} from '../api/tags';
import { useTags } from './useTags';

// This codebase's vitest setup does not auto-cleanup between tests.
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock('../api/tags', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../api/tags')>();
  return {
    ...actual,
    listTags: vi.fn(),
    createTag: vi.fn(),
    updateTag: vi.fn(),
    deleteTag: vi.fn(),
  };
});

beforeEach(() => {
  vi.spyOn(console, 'error').mockImplementation(() => {});
  vi.mocked(listTags).mockReset();
  vi.mocked(createTag).mockReset();
  vi.mocked(updateTag).mockReset();
  vi.mocked(deleteTag).mockReset();
});

const tag: Tag = {
  id: 'tag-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  name: 'friend',
};

const contactTag: ContactTag = { id: 1, tag_id: 'tag-1', contact_vcard_uid: 'uid-1' };

function listResponse(tags: Tag[], contacts: ContactTag[]): TagListResponse {
  return { tags, contacts, total: tags.length, next_cursor: '', limit: 200 };
}

test('loads tags and contact links on mount', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([tag], [contactTag]));

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(listTags).toHaveBeenCalledWith({ limit: 200, include_contacts: true });
  expect(result.current.tags).toHaveLength(1);
  expect(result.current.contacts).toHaveLength(1);
  expect(result.current.error).toBeNull();
});

test('maps contact uids to their tag names', async () => {
  const secondTag: Tag = { ...tag, id: 'tag-2', name: 'colleague' };
  vi.mocked(listTags).mockResolvedValue(
    listResponse(
      [tag, secondTag],
      [contactTag, { id: 2, tag_id: 'tag-2', contact_vcard_uid: 'uid-1' }],
    ),
  );

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.tagNamesByUid.get('uid-1')).toEqual(['friend', 'colleague']);
});

test('skips contact tags whose tag is missing', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([], [contactTag]));

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.tagNamesByUid.size).toBe(0);
});

test('handleCreate creates a tag and returns it', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([], []));
  vi.mocked(createTag).mockResolvedValue({ message: 'created', tag });

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleCreate('friend')).resolves.toEqual(tag);
  expect(createTag).toHaveBeenCalledWith('friend');
});

test('handleUpdate renames and refreshes', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([tag], []));

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleUpdate('tag-1', 'best friend');
  });

  expect(updateTag).toHaveBeenCalledWith('tag-1', 'best friend');
  expect(listTags).toHaveBeenCalledTimes(2);
});

test('handleDelete deletes and refreshes', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([tag], []));

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  await act(async () => {
    await result.current.handleDelete('tag-1');
  });

  expect(deleteTag).toHaveBeenCalledWith('tag-1');
  expect(listTags).toHaveBeenCalledTimes(2);
});

test('mutation errors notify through the notifier and rethrow', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([tag], []));
  vi.mocked(deleteTag).mockRejectedValue(new Error('boom'));
  const showError = vi.fn();

  const { result } = renderHook(() => useTags({ showError }));
  await waitFor(() => expect(result.current.loading).toBe(false));

  await expect(result.current.handleDelete('tag-1')).rejects.toThrow('boom');
  expect(showError).toHaveBeenCalledWith('boom');
});

test('sets error when the fetch fails', async () => {
  vi.mocked(listTags).mockRejectedValue(new Error('boom'));

  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));

  expect(result.current.error).toBe('boom');
  expect(result.current.tags).toEqual([]);
});

// --- behavior pins (issue #1662) ---

function deferred<T>() {
  let resolve!: (v: T) => void;
  const promise = new Promise<T>((res) => {
    resolve = res;
  });
  return { promise, resolve };
}

test('starts empty, requests up to 200 tags with their contacts, and reports loading', async () => {
  const d = deferred<TagListResponse>();
  vi.mocked(listTags).mockReturnValue(d.promise);
  const { result } = renderHook(() => useTags());
  expect(result.current.tags).toEqual([]);
  expect(result.current.contacts).toEqual([]);
  expect(result.current.tagNamesByUid.size).toBe(0);
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(listTags).toHaveBeenCalledWith({ limit: 200, include_contacts: true });
  await act(async () => {
    d.resolve(listResponse([tag], [contactTag]));
  });
  await waitFor(() => expect(result.current.loading).toBe(false));
});

test('tolerates a response without tags or contacts', async () => {
  vi.mocked(listTags).mockResolvedValue({} as TagListResponse);
  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.loading).toBe(false));
  expect(result.current.tags).toEqual([]);
  expect(result.current.contacts).toEqual([]);
});

test('a failed load logs its operation, and a refresh clears the error', async () => {
  vi.mocked(listTags).mockRejectedValueOnce(new Error('offline'));
  const { result } = renderHook(() => useTags());
  await waitFor(() => expect(result.current.error).toBe('offline'));
  expect(console.error).toHaveBeenCalledWith('[fetching tags] Error:', expect.anything());

  const d = deferred<TagListResponse>();
  vi.mocked(listTags).mockReturnValueOnce(d.promise);
  act(() => {
    void result.current.refresh();
  });
  await waitFor(() => expect(result.current.loading).toBe(true));
  expect(result.current.error).toBeNull();
  await act(async () => {
    d.resolve(listResponse([], []));
  });
});

test('tagNamesByUid collects every tag name per contact and tracks reloaded data', async () => {
  const second: Tag = { ...tag, id: 'tag-2', name: 'colleague' };
  vi.mocked(listTags).mockResolvedValueOnce(
    listResponse(
      [tag, second],
      [contactTag, { id: 2, tag_id: 'tag-2', contact_vcard_uid: 'uid-1' }],
    ),
  );
  const { result } = renderHook(() => useTags());
  await waitFor(() =>
    expect(result.current.tagNamesByUid.get('uid-1')).toEqual(['friend', 'colleague']),
  );

  vi.mocked(listTags).mockResolvedValueOnce(listResponse([tag], [contactTag]));
  await act(async () => {
    await result.current.refresh();
  });
  expect(result.current.tagNamesByUid.get('uid-1')).toEqual(['friend']);
});

test('rename, delete and create log their own operation and use the latest notifier', async () => {
  vi.mocked(listTags).mockResolvedValue(listResponse([tag], []));
  vi.mocked(createTag).mockRejectedValue(new Error('c'));
  vi.mocked(updateTag).mockRejectedValue(new Error('u'));
  vi.mocked(deleteTag).mockRejectedValue(new Error('d'));
  const first = { showError: vi.fn() };
  const second = { showError: vi.fn() };
  const { result, rerender } = renderHook(({ n }) => useTags(n), { initialProps: { n: first } });
  await waitFor(() => expect(result.current.loading).toBe(false));
  rerender({ n: second });
  await act(async () => {
    await expect(result.current.handleCreate('x')).rejects.toThrow('c');
    await expect(result.current.handleUpdate('tag-1', 'x')).rejects.toThrow('u');
    await expect(result.current.handleDelete('tag-1')).rejects.toThrow('d');
  });
  expect(first.showError).not.toHaveBeenCalled();
  expect(second.showError.mock.calls.map((c) => c[0])).toEqual(['c', 'u', 'd']);
  for (const op of ['creating tag', 'updating tag', 'deleting tag']) {
    expect(console.error).toHaveBeenCalledWith(`[${op}] Error:`, expect.anything());
  }
  expect(listTags).toHaveBeenCalledTimes(1);
});
