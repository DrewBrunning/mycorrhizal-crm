import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { ApiError } from './client';
import {
  createNote,
  createUnassignedNote,
  deleteNote,
  getContactNotes,
  getNote,
  getUnassignedNotes,
  updateNote,
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
