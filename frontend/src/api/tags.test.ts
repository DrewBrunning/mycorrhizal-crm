import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  listTags,
  createTag,
  updateTag,
  deleteTag,
  addContactTag,
  removeContactTag,
  Tag,
  ContactTag,
} from './tags';

afterEach(() => {
  vi.unstubAllGlobals();
});

const tag: Tag = {
  id: 'tag-1',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  name: 'Friend',
};

const contactTag: ContactTag = {
  id: 1,
  tag_id: 'tag-1',
  contact_vcard_uid: 'alice-uid',
};

const errorResponse = () => ({
  ok: false,
  status: 400,
  statusText: 'Bad Request',
  json: async () => ({
    error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'Required' } },
    request_id: 'req-1',
  }),
});

describe('listTags', () => {
  test('GETs the tags URL with limit, cursor, and include_contacts', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        tags: [tag],
        total: 1,
        next_cursor: 'CURSOR-1',
        limit: 100,
        contacts: [contactTag],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await listTags({ cursor: 'PREV', limit: 100, include_contacts: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags');
    expect(url).toContain('limit=100');
    expect(url).toContain('cursor=PREV');
    expect(url).toContain('include_contacts=true');
    expect(init.method).toBeUndefined();
    expect(result.tags).toEqual([tag]);
    expect(result.contacts).toEqual([contactTag]);
  });

  test('uses the default limit and omits include_contacts when not requested', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ tags: [], total: 0, next_cursor: '', limit: 100 }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await listTags();

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('limit=100');
    expect(url).not.toContain('include_contacts');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(listTags()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('createTag', () => {
  test('POSTs the tag name and returns the created tag', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Tag created', tag }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createTag('Friend');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ name: 'Friend' });
    expect(result.tag).toEqual(tag);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(createTag('Friend')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('updateTag', () => {
  test('PUTs the tag name and returns the updated tag', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => tag });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateTag('tag-1', 'Renamed');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags/tag-1');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ name: 'Renamed' });
    expect(result).toEqual(tag);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(updateTag('tag-1', 'x')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('deleteTag', () => {
  test('DELETEs the tag URL', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await deleteTag('tag-1');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags/tag-1');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(deleteTag('tag-1')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('addContactTag', () => {
  test('POSTs the contact payload and returns the created tag membership', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => contactTag });
    vi.stubGlobal('fetch', fetchMock);

    const result = await addContactTag('tag-1', 'alice-uid');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags/tag-1/contacts');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ contact_vcard_uid: 'alice-uid' });
    expect(result).toEqual(contactTag);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(addContactTag('tag-1', 'alice-uid')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('removeContactTag', () => {
  test('DELETEs the tag membership URL with the vcard uid', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, status: 204 });
    vi.stubGlobal('fetch', fetchMock);

    await removeContactTag('tag-1', 'alice-uid');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/tags/tag-1/contacts/alice-uid');
    expect(init.method).toBe('DELETE');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));
    await expect(removeContactTag('tag-1', 'alice-uid')).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});
