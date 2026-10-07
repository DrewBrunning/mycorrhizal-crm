import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { BULK_ACTIONS, runBulkOperation } from './bulkOperations';
import { ApiError } from './client';

setupMswServer();

describe('runBulkOperation', () => {
  test('lists the seven bulk actions the backend accepts', () => {
    expect([...BULK_ACTIONS]).toEqual([
      'add_circle',
      'remove_circle',
      'add_tag',
      'remove_tag',
      'archive',
      'unarchive',
      'delete',
    ]);
  });

  test('POSTs the request body to /contacts/bulk and returns the result', async () => {
    const result = { action: 'add_tag', total: 2, succeeded: 1, failed: 1, failures: [] };
    const calls = mockApi('post', '/contacts/bulk', result);
    const out = await runBulkOperation({ action: 'add_tag', vcard_uids: ['a', 'b'], tag_id: 't1' });
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/contacts/bulk');
    expect(calls[0].headers.get('content-type')).toBe('application/json');
    expect(calls[0].body).toEqual({ action: 'add_tag', vcard_uids: ['a', 'b'], tag_id: 't1' });
    expect(out).toEqual(result);
  });

  test('maps an error envelope to a typed ApiError', async () => {
    mockApi(
      'post',
      '/contacts/bulk',
      errorEnvelope(400, 'VALIDATION_ERROR', 'bad action', { action: 'invalid' }),
    );
    const err = await runBulkOperation({ action: 'delete', vcard_uids: [] }).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(400);
    expect(err.code).toBe('VALIDATION_ERROR');
    expect(err.getDisplayMessage()).toBe('invalid');
  });
});
