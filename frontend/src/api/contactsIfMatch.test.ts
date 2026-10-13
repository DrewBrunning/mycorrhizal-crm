import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { ApiError, ifMatchHeader, isPreconditionFailed } from './client';
import { updateContactRecord } from './contacts';

setupMswServer();

const input = { card: {}, crm: {} };

// ADR 0008: the web client opts in to optimistic concurrency by echoing the
// record's `revision` back as `If-Match` on contact updates.
describe('updateContactRecord If-Match', () => {
  test('sends the revision as a quoted If-Match header', async () => {
    const calls = mockApi('put', '/contacts/42', { id: 42, revision: 8, card: {}, crm: {} });
    const result = await updateContactRecord(42, input, 7);
    expect(calls[0].headers.get('if-match')).toBe('"7"');
    expect(result.revision).toBe(8);
  });

  test('sends If-Match for revision 0', async () => {
    const calls = mockApi('put', '/contacts/42', { id: 42 });
    await updateContactRecord(42, input, 0);
    expect(calls[0].headers.get('if-match')).toBe('"0"');
  });

  test('omits If-Match when no revision is supplied (unconditional write)', async () => {
    const calls = mockApi('put', '/contacts/42', { id: 42 });
    await updateContactRecord(42, input);
    expect(calls[0].headers.has('if-match')).toBe(false);
  });

  test('surfaces a 412 as an ApiError that isPreconditionFailed recognises', async () => {
    mockApi(
      'put',
      '/contacts/42',
      errorEnvelope(412, 'PRECONDITION_FAILED', 'stale', { expected_revision: '9' }),
    );
    const err = await updateContactRecord(42, input, 7).catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(isPreconditionFailed(err)).toBe(true);
    expect((err as ApiError).details?.expected_revision).toBe('9');
  });
});

describe('isPreconditionFailed / ifMatchHeader', () => {
  test('matches by status or code, and rejects other errors', () => {
    expect(isPreconditionFailed(new ApiError('x', 'UNKNOWN_ERROR', 412))).toBe(true);
    expect(isPreconditionFailed(new ApiError('x', 'PRECONDITION_FAILED', 500))).toBe(true);
    expect(isPreconditionFailed(new ApiError('x', 'CONFLICT', 409))).toBe(false);
    expect(isPreconditionFailed(new Error('x'))).toBe(false);
  });

  test('ifMatchHeader quotes the revision', () => {
    expect(ifMatchHeader(3)).toEqual({ 'If-Match': '"3"' });
  });
});
