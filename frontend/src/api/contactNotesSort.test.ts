import { expect, test } from 'vitest';
import '../i18n/config';
import { mockApi, setupMswServer } from '../test/mswServer';
import { getContactNotes } from './notes';

setupMswServer();

// Issue #2797: GET /contacts/{id}/notes takes sort=updated_at|date like GET /notes. The cursor is
// sort-specific, so a caller that pages must send the same sort on every page.
const page = {
  notes: [
    {
      ID: 7,
      content: 'Met at the conference',
      date: '2026-03-02T00:00:00Z',
      CreatedAt: '2026-03-02T09:00:00Z',
      UpdatedAt: '2026-03-02T09:00:00Z',
    },
  ],
  next_cursor: '',
  limit: 25,
};

test('getContactNotes sends no query string by default (server default order)', async () => {
  const calls = mockApi('get', '/contacts/42/notes', page);
  await getContactNotes(42);
  expect(calls[0].url).toBe('/api/v1/contacts/42/notes');
});

test('getContactNotes sends sort=date', async () => {
  const calls = mockApi('get', '/contacts/42/notes', page);
  const result = await getContactNotes(42, { sort: 'date' });
  expect(calls[0].search.get('sort')).toBe('date');
  expect(result.notes).toHaveLength(1);
});

test('getContactNotes keeps sort=date on a later page together with its cursor and limit', async () => {
  const calls = mockApi('get', '/contacts/42/notes', page);
  await getContactNotes(42, { sort: 'date', cursor: 'opaque-date-cursor', limit: 10 });
  expect(calls[0].search.get('sort')).toBe('date');
  expect(calls[0].search.get('cursor')).toBe('opaque-date-cursor');
  expect(calls[0].search.get('limit')).toBe('10');
});
