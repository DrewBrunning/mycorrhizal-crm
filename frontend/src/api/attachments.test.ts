import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer, setupNativeMultipart } from '../test/mswServer';
import {
  attachmentDownloadUrl,
  deleteAttachment,
  formatAttachmentSize,
  listContactAttachments,
  uploadAttachment,
} from './attachments';
import { ApiError } from './client';

setupMswServer();
const makeFile = setupNativeMultipart();

describe('attachments API', () => {
  test('listContactAttachments GETs /contacts/:id/attachments', async () => {
    const body = { attachments: [], total: 0 };
    const calls = mockApi('get', '/contacts/12/attachments', body);
    expect(await listContactAttachments(12)).toEqual(body);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/contacts/12/attachments');
  });

  test('listContactAttachments accepts a string id and maps errors', async () => {
    mockApi(
      'get',
      '/contacts/abc/attachments',
      errorEnvelope(404, 'NOT_FOUND', 'Contact not found'),
    );
    const err = await listContactAttachments('abc').catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.status).toBe(404);
    expect(err.message).toBe('Contact not found');
  });

  test('uploadAttachment POSTs multipart `file` without a JSON content-type and unwraps .attachment', async () => {
    const attachment = { ID: 3, original_name: 'a.txt' };
    const calls = mockApi('post', '/contacts/5/attachments', { attachment });
    const out = await uploadAttachment(5, makeFile('hello', 'a.txt'));
    expect(out).toEqual(attachment);
    expect(calls[0].method).toBe('POST');
    expect(calls[0].headers.get('content-type')).toMatch(/^multipart\/form-data; boundary=/);
    const sent = calls[0].form?.get('file') as File;
    expect(sent.name).toBe('a.txt');
    expect(await sent.text()).toBe('hello');
  });

  test('uploadAttachment maps a 413 envelope to ApiError', async () => {
    mockApi('post', '/contacts/5/attachments', errorEnvelope(413, 'FILE_TOO_LARGE', 'too big'));
    const err = await uploadAttachment(5, makeFile('x', 'x.txt')).catch((e) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect(err.code).toBe('FILE_TOO_LARGE');
    expect(err.status).toBe(413);
  });

  test('deleteAttachment DELETEs /attachments/:id', async () => {
    const calls = mockApi('delete', '/attachments/9', new Response(null, { status: 204 }));
    await expect(deleteAttachment(9)).resolves.toBeUndefined();
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].url).toBe('/api/v1/attachments/9');
  });

  test('deleteAttachment rejects on error', async () => {
    mockApi('delete', '/attachments/9', errorEnvelope(404, 'NOT_FOUND', 'gone'));
    await expect(deleteAttachment(9)).rejects.toMatchObject({ status: 404, message: 'gone' });
  });

  test('attachmentDownloadUrl is a same-origin API path', () => {
    expect(attachmentDownloadUrl(4)).toMatch(/\/api\/v1\/attachments\/4\/download$/);
  });

  test('formatAttachmentSize picks B / KB / MB at the exact boundaries', () => {
    expect(formatAttachmentSize(0)).toBe('0 B');
    expect(formatAttachmentSize(1023)).toBe('1023 B');
    expect(formatAttachmentSize(1024)).toBe('1.0 KB');
    expect(formatAttachmentSize(1536)).toBe('1.5 KB');
    expect(formatAttachmentSize(1024 * 1024 - 1)).toBe('1024.0 KB');
    expect(formatAttachmentSize(1024 * 1024)).toBe('1.0 MB');
    expect(formatAttachmentSize(5 * 1024 * 1024 + 512 * 1024)).toBe('5.5 MB');
  });
});
