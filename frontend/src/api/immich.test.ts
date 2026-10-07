import { describe, expect, test } from 'vitest';
import { errorEnvelope, mockApi, setupMswServer } from '../test/mswServer';
import { ApiError } from './client';
import {
  deleteImmichConfig,
  getImmichConfig,
  getImmichContactAssets,
  getImmichContactSummary,
  getImmichPeople,
  immichAssetImageUrl,
  immichThumbnailUrl,
  linkImmichPerson,
  saveImmichConfig,
  syncImmich,
  testImmichConnection,
  unlinkImmichPerson,
} from './immich';

setupMswServer();

const err = () => errorEnvelope(502, 'UPSTREAM', 'immich unreachable');

async function expectUpstream(p: Promise<unknown>) {
  const e = (await p.catch((x) => x)) as ApiError;
  expect(e).toBeInstanceOf(ApiError);
  expect(e.status).toBe(502);
  expect(e.code).toBe('UPSTREAM');
}

describe('immich API', () => {
  test('getImmichConfig GETs /immich/config', async () => {
    const cfg = {
      base_url: 'https://i',
      has_api_key: true,
      sync_enabled: false,
      last_sync_status: '',
      last_sync_error: '',
    };
    const calls = mockApi('get', '/immich/config', cfg);
    expect(await getImmichConfig()).toEqual(cfg);
    expect(calls[0].method).toBe('GET');
    expect(calls[0].url).toBe('/api/v1/immich/config');
    mockApi('get', '/immich/config', err());
    await expectUpstream(getImmichConfig());
  });

  test('saveImmichConfig PUTs the input', async () => {
    const calls = mockApi('put', '/immich/config', { base_url: 'x' });
    await saveImmichConfig({ base_url: 'https://i', api_key: 'k', sync_enabled: true });
    expect(calls[0].method).toBe('PUT');
    expect(calls[0].body).toEqual({ base_url: 'https://i', api_key: 'k', sync_enabled: true });
    mockApi('put', '/immich/config', err());
    await expectUpstream(saveImmichConfig({ base_url: 'x' }));
  });

  test('deleteImmichConfig DELETEs', async () => {
    const calls = mockApi('delete', '/immich/config', new Response(null, { status: 204 }));
    await expect(deleteImmichConfig()).resolves.toBeUndefined();
    expect(calls[0].method).toBe('DELETE');
    mockApi('delete', '/immich/config', err());
    await expectUpstream(deleteImmichConfig());
  });

  test('testImmichConnection POSTs and returns a diagnosed failure as data', async () => {
    const result = { ok: false, stage: 'auth', message: 'bad key' };
    const calls = mockApi('post', '/immich/test-connection', result);
    expect(await testImmichConnection()).toEqual(result);
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/immich/test-connection');
    mockApi('post', '/immich/test-connection', err());
    await expectUpstream(testImmichConnection());
  });

  test('getImmichPeople returns the people array, [] when absent', async () => {
    const calls = mockApi('get', '/immich/people', { people: [{ id: 'p', name: 'Ann' }] });
    expect(await getImmichPeople()).toEqual([{ id: 'p', name: 'Ann' }]);
    expect(calls[0].url).toBe('/api/v1/immich/people');
    mockApi('get', '/immich/people', {});
    expect(await getImmichPeople()).toEqual([]);
    mockApi('get', '/immich/people', err());
    await expectUpstream(getImmichPeople());
  });

  test('linkImmichPerson POSTs person_id and person_name', async () => {
    const calls = mockApi('post', '/immich/contacts/u1/link', {});
    await linkImmichPerson('u1', 'p9', 'Ann');
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/immich/contacts/u1/link');
    expect(calls[0].body).toEqual({ person_id: 'p9', person_name: 'Ann' });
    mockApi('post', '/immich/contacts/u1/link', err());
    await expectUpstream(linkImmichPerson('u1', 'p', 'n'));
  });

  test('unlinkImmichPerson DELETEs the link', async () => {
    const calls = mockApi(
      'delete',
      '/immich/contacts/u1/link',
      new Response(null, { status: 204 }),
    );
    await unlinkImmichPerson('u1');
    expect(calls[0].method).toBe('DELETE');
    expect(calls[0].url).toBe('/api/v1/immich/contacts/u1/link');
    mockApi('delete', '/immich/contacts/u1/link', err());
    await expectUpstream(unlinkImmichPerson('u1'));
  });

  test('getImmichContactSummary returns summary, null when absent', async () => {
    const summary = { person_name: 'Ann', photo_count: 3 };
    const calls = mockApi('get', '/immich/contacts/u1/summary', { summary });
    expect(await getImmichContactSummary('u1')).toEqual(summary);
    expect(calls[0].url).toBe('/api/v1/immich/contacts/u1/summary');
    mockApi('get', '/immich/contacts/u1/summary', {});
    expect(await getImmichContactSummary('u1')).toBeNull();
    mockApi('get', '/immich/contacts/u1/summary', err());
    await expectUpstream(getImmichContactSummary('u1'));
  });

  test('syncImmich POSTs /immich/sync', async () => {
    const calls = mockApi('post', '/immich/sync', {});
    await syncImmich();
    expect(calls[0].method).toBe('POST');
    expect(calls[0].url).toBe('/api/v1/immich/sync');
    mockApi('post', '/immich/sync', err());
    await expectUpstream(syncImmich());
  });

  test('getImmichContactAssets returns assets, [] when absent', async () => {
    const calls = mockApi('get', '/immich/contacts/u1/assets', {
      assets: [{ id: 'a', occurred_at: 't' }],
    });
    expect(await getImmichContactAssets('u1')).toEqual([{ id: 'a', occurred_at: 't' }]);
    expect(calls[0].url).toBe('/api/v1/immich/contacts/u1/assets');
    mockApi('get', '/immich/contacts/u1/assets', {});
    expect(await getImmichContactAssets('u1')).toEqual([]);
    mockApi('get', '/immich/contacts/u1/assets', err());
    await expectUpstream(getImmichContactAssets('u1'));
  });

  test('URL builders point at the proxied endpoints', () => {
    expect(immichThumbnailUrl('u1')).toMatch(/\/api\/v1\/immich\/contacts\/u1\/thumbnail$/);
    expect(immichAssetImageUrl('u1', 'a2')).toMatch(
      /\/api\/v1\/immich\/contacts\/u1\/assets\/a2\/image$/,
    );
  });
});
