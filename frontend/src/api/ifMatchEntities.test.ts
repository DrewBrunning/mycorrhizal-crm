import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { deleteActivity, updateActivity } from './activities';
import { isPreconditionFailed } from './client';
import { deleteContact } from './contacts';
import { deleteLifeEvent, updateLifeEvent } from './lifeEvents';
import { deleteNote, updateNote } from './notes';
import { deleteReminder, updateReminder } from './reminders';

setupMswServer();

// ADR 0008 (issue #1656): every PUT/DELETE the backend guards with checkIfMatch
// takes the loaded revision and echoes it as a quoted If-Match; with no
// revision the write stays unconditional; a stale revision is a typed 412.
const stale = () => errorEnvelope(412, 'PRECONDITION_FAILED', 'stale', { expected_revision: '9' });

const note = { content: 'c', date: '2026-01-01T00:00:00Z' };
const lifeEvent = { entity_id: 'u1', type: 'moved' };

const cases: Array<{
  name: string;
  method: 'put' | 'delete';
  path: string;
  ok: Record<string, unknown>;
  call: (revision?: number) => Promise<unknown>;
}> = [
  {
    name: 'updateNote',
    method: 'put',
    path: '/notes/1',
    ok: { ID: 1 },
    call: (r) => updateNote(1, note, r),
  },
  { name: 'deleteNote', method: 'delete', path: '/notes/1', ok: {}, call: (r) => deleteNote(1, r) },
  {
    name: 'updateActivity',
    method: 'put',
    path: '/activities/1',
    ok: { ID: 1 },
    call: (r) => updateActivity(1, { title: 't' }, r),
  },
  {
    name: 'deleteActivity',
    method: 'delete',
    path: '/activities/1',
    ok: {},
    call: (r) => deleteActivity(1, r),
  },
  {
    name: 'updateLifeEvent',
    method: 'put',
    path: '/life-events/le-1',
    ok: { id: 'le-1' },
    call: (r) => updateLifeEvent('le-1', lifeEvent, r),
  },
  {
    name: 'deleteLifeEvent',
    method: 'delete',
    path: '/life-events/le-1',
    ok: {},
    call: (r) => deleteLifeEvent('le-1', r),
  },
  {
    name: 'updateReminder',
    method: 'put',
    path: '/reminders/1',
    ok: { reminder: { ID: 1 } },
    call: (r) => updateReminder(1, { message: 'm' }, r),
  },
  {
    name: 'deleteReminder',
    method: 'delete',
    path: '/reminders/1',
    ok: {},
    call: (r) => deleteReminder(1, r),
  },
  {
    name: 'deleteContact',
    method: 'delete',
    path: '/contacts/1',
    ok: {},
    call: (r) => deleteContact(1, r),
  },
];

describe.each(cases)('$name If-Match', ({ method, path, ok, call }) => {
  test('sends the revision as a quoted If-Match header', async () => {
    const calls = mockApi(method, path, ok as never);
    await call(7);
    expect(calls[0].headers.get('if-match')).toBe('"7"');
  });

  test('sends If-Match for revision 0', async () => {
    const calls = mockApi(method, path, ok as never);
    await call(0);
    expect(calls[0].headers.get('if-match')).toBe('"0"');
  });

  test('omits If-Match without a revision (unconditional write)', async () => {
    const calls = mockApi(method, path, ok as never);
    await call();
    expect(calls[0].headers.has('if-match')).toBe(false);
  });

  test('surfaces a 412 as a typed precondition-failed ApiError', async () => {
    mockApi(method, path, stale());
    const err = await call(7).catch((e: unknown) => e);
    expect(isPreconditionFailed(err)).toBe(true);
  });
});
