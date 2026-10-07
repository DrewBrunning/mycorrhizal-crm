import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import {
  API_TOKEN_EXPIRY_OPTIONS,
  createApiToken,
  DEFAULT_API_TOKEN_EXPIRY_DAYS,
  DEFAULT_API_TOKEN_SCOPE,
  getApiTokens,
  revokeAllApiTokens,
  revokeApiToken,
  rotateApiToken,
} from './apiTokens';

setupMswServer();

const token = {
  id: 7,
  name: 'laptop',
  created_at: '2026-08-01T00:00:00Z',
  last_used_at: null,
  revoked_at: null,
  expires_at: null,
  scope: 'full',
};

describe('apiTokens', () => {
  test('constants', () => {
    expect(API_TOKEN_EXPIRY_OPTIONS).toEqual([30, 60, 90, 180, 365]);
    expect(DEFAULT_API_TOKEN_EXPIRY_DAYS).toBe(90);
    expect(DEFAULT_API_TOKEN_SCOPE).toBe('full');
  });

  test('getApiTokens GETs /api-tokens and returns the tokens', async () => {
    const calls = mockApi('get', '/api-tokens', { tokens: [token] });
    const res = await getApiTokens();
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/api-tokens');
    expect(res.tokens).toEqual([token]);
  });

  test('getApiTokens defaults to an empty list when the body has no tokens', async () => {
    mockApi('get', '/api-tokens', {});
    expect(await getApiTokens()).toEqual({ tokens: [] });
  });

  test('getApiTokens surfaces the backend message on error', async () => {
    mockApi('get', '/api-tokens', errorEnvelope(500, 'INTERNAL', 'db down'));
    await expect(getApiTokens()).rejects.toThrow('db down');
  });

  test('createApiToken POSTs name, default expiry and default scope', async () => {
    const calls = mockApi('post', '/api-tokens', { ...token, token: 'secret' });
    const res = await createApiToken('laptop');
    expect(calls[0].method).toBe('POST');
    expect(calls[0].headers.get('content-type')).toBe('application/json');
    expect(calls[0].body).toEqual({ name: 'laptop', expires_in_days: 90, scope: 'full' });
    expect(res.token).toBe('secret');
  });

  test('createApiToken sends explicit expiry and scope', async () => {
    const calls = mockApi('post', '/api-tokens', { ...token, token: 's' });
    await createApiToken('dav', 30, 'carddav');
    expect(calls[0].body).toEqual({ name: 'dav', expires_in_days: 30, scope: 'carddav' });
  });

  test('createApiToken maps a validation envelope to its field message', async () => {
    mockApi(
      'post',
      '/api-tokens',
      errorEnvelope(400, 'VALIDATION_ERROR', 'bad', { name: 'Name is required' }),
    );
    await expect(createApiToken('')).rejects.toThrow('Name is required');
  });

  test('revokeApiToken DELETEs /api-tokens/:id', async () => {
    const calls = mockApi('delete', '/api-tokens/7', new Response(null, { status: 204 }));
    await expect(revokeApiToken(7)).resolves.toBeUndefined();
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].url).toBe('/api/v1/api-tokens/7');
  });

  test('revokeApiToken rejects on 404', async () => {
    mockApi('delete', '/api-tokens/9', errorEnvelope(404, 'NOT_FOUND', 'Token not found'));
    await expect(revokeApiToken(9)).rejects.toThrow('Token not found');
  });

  test('revokeAllApiTokens POSTs /api-tokens/revoke-all and returns the count', async () => {
    const calls = mockApi('post', '/api-tokens/revoke-all', { revoked: 3 });
    expect(await revokeAllApiTokens()).toEqual({ revoked: 3 });
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/api-tokens/revoke-all');
  });

  test('rotateApiToken POSTs /api-tokens/:id/rotate and returns the new token', async () => {
    const calls = mockApi('post', '/api-tokens/7/rotate', { ...token, id: 8, token: 'new' });
    const res = await rotateApiToken(7);
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/api-tokens/7/rotate');
    expect(res.token).toBe('new');
  });

  test('rotateApiToken failure throws a typed message', async () => {
    mockApi('post', '/api-tokens/7/rotate', errorEnvelope(409, 'CONFLICT', 'already revoked'));
    const err = await rotateApiToken(7).catch((e) => e);
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe('already revoked');
  });

  test('revokeAllApiTokens failure falls back to the default message on an empty body', async () => {
    mockApi('post', '/api-tokens/revoke-all', new Response('', { status: 500 }));
    await expect(revokeAllApiTokens()).rejects.toThrow('Unable to revoke all API tokens.');
  });
});
