import { describe, expect, test, vi } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { ApiError } from './client';
import {
  createNote,
  createUnassignedNote,
  deleteNote,
  deleteNoteImage,
  getContactNotes,
  getNote,
  getNoteImages,
  getUnassignedNotes,
  noteImageSrc,
  updateNote,
  uploadNoteImage,
} from './notes';

setupMswServer();

const note = { ID: 1, content: 'hi', date: '2026-08-01', CreatedAt: '', UpdatedAt: '' };
const err404 = () => errorEnvelope(404, 'NOT_FOUND', 'Note not found');

async function expectApiError(p: Promise<unknown>, status = 404) {
  const e = (await p.catch((x) => x)) as ApiError;
  expect(e).toBeInstanceOf(ApiError);
  expect(e.status).toBe(status);
  expect(e.code).toBe('NOT_FOUND');
}

describe('notes API requests', () => {
  test('getContactNotes GETs /contacts/:id/notes', async () => {
    const calls = mockApi('get', '/contacts/3/notes', { notes: [note] });
    expect((await getContactNotes(3)).notes).toEqual([note]);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/contacts/3/notes');
  });

  test('getContactNotes maps errors', async () => {
    mockApi('get', '/contacts/3/notes', err404());
    await expectApiError(getContactNotes('3'));
  });

  test('getUnassignedNotes defaults to limit=25 and nothing else', async () => {
    const calls = mockApi('get', '/notes', { notes: [], total: 0 });
    await getUnassignedNotes();
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/notes?limit=25');
  });

  test('getUnassignedNotes sends cursor, trimmed search, fromDate, toDate and limit', async () => {
    const calls = mockApi('get', '/notes', { notes: [] });
    await getUnassignedNotes({
      cursor: 'abc',
      limit: 10,
      search: '  coffee  ',
      fromDate: '2026-01-01',
      toDate: '2026-02-01',
    });
    expect(calls[0].search.get('limit')).toBe('10');
    expect(calls[0].search.get('cursor')).toBe('abc');
    expect(calls[0].search.get('search')).toBe('coffee');
    expect(calls[0].search.get('fromDate')).toBe('2026-01-01');
    expect(calls[0].search.get('toDate')).toBe('2026-02-01');
  });

  test('getUnassignedNotes omits a blank search and absent cursor/dates', async () => {
    const calls = mockApi('get', '/notes', { notes: [] });
    await getUnassignedNotes({ search: '   ' });
    expect([...calls[0].search.keys()]).toEqual(['limit']);
  });

  test('getUnassignedNotes maps errors', async () => {
    mockApi('get', '/notes', err404());
    await expectApiError(getUnassignedNotes());
  });

  test('getNote GETs /notes/:id', async () => {
    const calls = mockApi('get', '/notes/8', note);
    expect(await getNote(8)).toEqual(note);
    expect(calls[0].url).toBe('/api/v1/notes/8');
  });

  test('getNote maps errors', async () => {
    mockApi('get', '/notes/8', err404());
    await expectApiError(getNote('8'));
  });

  test('createNote POSTs the body to /contacts/:id/notes', async () => {
    const calls = mockApi('post', '/contacts/3/notes', note);
    await createNote(3, { content: 'hi', date: '2026-08-01' });
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/contacts/3/notes');
    expect(calls[0].headers.get('content-type')).toBe('application/json');
    expect(calls[0].body).toEqual({ content: 'hi', date: '2026-08-01' });
  });

  test('createNote maps errors', async () => {
    mockApi('post', '/contacts/3/notes', err404());
    await expectApiError(createNote('3', { content: 'x', date: 'd' }));
  });

  test('createUnassignedNote POSTs to /notes keeping an explicit null contact_id', async () => {
    const calls = mockApi('post', '/notes', note);
    await createUnassignedNote({ content: 'c', date: 'd', contact_id: null });
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/notes');
    expect(calls[0].body).toEqual({ content: 'c', date: 'd', contact_id: null });
  });

  test('createUnassignedNote maps errors', async () => {
    mockApi('post', '/notes', err404());
    await expectApiError(createUnassignedNote({ content: 'c', date: 'd' }));
  });

  test('updateNote PUTs the body to /notes/:id', async () => {
    const calls = mockApi('put', '/notes/4', note);
    await updateNote(4, { content: 'new', date: 'd', contact_id: 9 });
    expect(calls[0].method).toBe('PUT');
    expect(calls[0].url).toBe('/api/v1/notes/4');
    expect(calls[0].body).toEqual({ content: 'new', date: 'd', contact_id: 9 });
  });

  test('updateNote maps errors', async () => {
    mockApi('put', '/notes/4', err404());
    await expectApiError(updateNote('4', { content: 'x', date: 'd' }));
  });

  test('deleteNote DELETEs /notes/:id', async () => {
    const calls = mockApi('delete', '/notes/4', new Response(null, { status: 204 }));
    await expect(deleteNote(4)).resolves.toBeUndefined();
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].url).toBe('/api/v1/notes/4');
  });

  test('deleteNote maps errors', async () => {
    mockApi('delete', '/notes/4', err404());
    await expectApiError(deleteNote('4'));
  });
});

describe('note image API requests', () => {
  const image = {
    id: 9,
    uuid: 'u',
    note_id: 4,
    mime_type: 'image/jpeg',
    width: 10,
    height: 10,
    size_bytes: 100,
    sensitivity: 'normal',
    taken_at: null,
    taken_date: null,
    created_at: '2026-01-01T00:00:00Z',
    file_url: '/api/v1/notes/4/images/9/file',
    thumbnail_url: '/api/v1/notes/4/images/9/thumbnail',
  };

  test('lists and deletes note images through the owner-nested routes', async () => {
    const getCalls = mockApi('get', '/notes/4/images', { images: [image], total: 1 });
    expect((await getNoteImages(4)).images).toEqual([image]);
    expect(getCalls[0].url).toBe('/api/v1/notes/4/images');

    const deleteCalls = mockApi('delete', '/notes/4/images/9', { message: 'Image deleted' });
    await deleteNoteImage(4, 9);
    expect(deleteCalls[0].method).toBe('DELETE');
  });

  test('maps note image errors', async () => {
    mockApi('get', '/notes/4/images', err404());
    await expectApiError(getNoteImages(4));
    mockApi('delete', '/notes/4/images/9', err404());
    await expectApiError(deleteNoteImage(4, 9));
  });

  test('noteImageSrc prepends only the API origin', () => {
    expect(noteImageSrc('/api/v1/notes/4/images/9/file')).toBe('/api/v1/notes/4/images/9/file');
  });
});
describe('note image upload', () => {
  test('POSTs multipart without a JSON content-type and returns the image', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ image: { id: 9, note_id: 4 } }),
    });
    vi.stubGlobal('fetch', fetchMock);
    const file = new File(['x'], 'photo.jpg', { type: 'image/jpeg' });
    expect((await uploadNoteImage(4, file)).id).toBe(9);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/notes/4/images');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    vi.unstubAllGlobals();
  });

  test('maps upload errors', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce(errorEnvelope(400, 'VALIDATION_ERROR', 'bad')),
    );
    await expect(uploadNoteImage(4, new File(['x'], 'photo.jpg'))).rejects.toMatchObject({
      status: 400,
    });
    vi.unstubAllGlobals();
  });
});
