import { test, expect, vi, afterEach, beforeEach } from 'vitest';
import { renderHook, cleanup, waitFor, act } from '@testing-library/react';
import { useTags } from './useTags';
import { listTags, createTag, updateTag, deleteTag, Tag, ContactTag, TagListResponse } from '../api/tags';

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
      [contactTag, { id: 2, tag_id: 'tag-2', contact_vcard_uid: 'uid-1' }]
    )
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
