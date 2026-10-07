import { describe, expect, test } from 'vitest';
import { API_BASE_URL, apiFetch } from '../api/client';
import { errorEnvelope, mockApi, setupMswServer, setupNativeMultipart } from './mswServer';

setupMswServer();
const makeFile = setupNativeMultipart();

describe('mswServer harness', () => {
  test('captures method, url, headers and JSON body through apiFetch', async () => {
    const calls = mockApi('post', '/thing', { ok: true });
    const res = await apiFetch(`${API_BASE_URL}/thing?a=1`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'If-Match': '3' },
      body: JSON.stringify({ x: 1 }),
    });
    expect(await res.json()).toEqual({ ok: true });
    expect(calls).toHaveLength(1);
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/thing?a=1');
    expect(calls[0].search.get('a')).toBe('1');
    expect(calls[0].headers.get('if-match')).toBe('3');
    expect(calls[0].body).toEqual({ x: 1 });
  });

  test('captures multipart form bodies and non-JSON text bodies', async () => {
    const form = new FormData();
    form.append('file', makeFile('hi', 'a.txt'));
    const calls = mockApi('post', '/up');
    await apiFetch(`${API_BASE_URL}/up`, { method: 'POST', body: form });
    expect(calls[0].form).toBeDefined();
    expect(((calls[0].form as FormData).get('file') as File).name).toBe('a.txt');

    const t = mockApi('put', '/txt');
    await apiFetch(`${API_BASE_URL}/txt`, { method: 'PUT', body: 'not json' });
    expect(t[0].body).toBe('not json');
  });

  test('errorEnvelope builds the backend error shape', async () => {
    mockApi('get', '/bad', errorEnvelope(422, 'VALIDATION_ERROR', 'nope', { f: 'req' }));
    const res = await apiFetch(`${API_BASE_URL}/bad`);
    expect(res.status).toBe(422);
    expect(await res.json()).toEqual({
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: { f: 'req' } },
      request_id: 'req-test',
    });
  });
});
