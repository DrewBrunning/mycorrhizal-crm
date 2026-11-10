import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  adoptContactEntityLink,
  createContactEntityLink,
  createEntitiesFromContacts,
  createEntity,
  deleteContactEntityLink,
  deleteEntity,
  dismissEntitySuggestion,
  type Entity,
  entityLinkIdFromElementId,
  getEntity,
  isProjectedElementId,
  linkContactEntityFromExisting,
  listContactEntityLinks,
  listEntities,
  listEntityContacts,
  listEntityFieldValues,
  listEntitySuggestions,
  mergeEntity,
  replaceEntityFieldValues,
  restoreEntity,
  updateContactEntityLink,
  updateEntity,
} from './entities';

afterEach(() => {
  vi.unstubAllGlobals();
});

const entity: Entity = {
  id: 'e1',
  kind: 'organisation',
  name: 'Acme',
  core: {},
  sensitivity: 'normal',
  revision: 3,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

function stubFetch(body: unknown, ok = true, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue({
    ok,
    status,
    json: async () => body,
    headers: new Headers(),
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

describe('projected element ids', () => {
  test('recognises the reserved ent- prefix and extracts the link id', () => {
    expect(isProjectedElementId('ent-abc')).toBe(true);
    expect(isProjectedElementId('abc')).toBe(false);
    expect(isProjectedElementId(undefined)).toBe(false);
    expect(entityLinkIdFromElementId('ent-abc')).toBe('abc');
    expect(entityLinkIdFromElementId('abc')).toBeUndefined();
  });
});

describe('entity CRUD', () => {
  test('listEntities sends kind, q and cursor filters', async () => {
    const fetchMock = stubFetch({ entities: [entity], limit: 25, next_cursor: 'n' });
    const res = await listEntities({ kind: 'place', q: 'cab in', cursor: 'c1', limit: 10 });
    const url = new URL(fetchMock.mock.calls[0][0], 'http://x');
    expect(url.pathname).toMatch(/\/entities$/);
    expect(url.searchParams.get('kind')).toBe('place');
    expect(url.searchParams.get('q')).toBe('cab in');
    expect(url.searchParams.get('cursor')).toBe('c1');
    expect(url.searchParams.get('limit')).toBe('10');
    expect(res.entities[0].name).toBe('Acme');
  });

  test('listEntities omits empty filters and tolerates a null entities array', async () => {
    const fetchMock = stubFetch({ entities: null, limit: 25, next_cursor: '' });
    const res = await listEntities({});
    expect(fetchMock.mock.calls[0][0]).not.toContain('kind=');
    expect(res.entities).toEqual([]);
  });

  test('getEntity / createEntity / updateEntity use the right verbs and If-Match', async () => {
    let fetchMock = stubFetch(entity);
    await getEntity('e1');
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/e1$/);

    fetchMock = stubFetch(entity);
    await createEntity({ kind: 'organisation', name: 'Acme' });
    expect(fetchMock.mock.calls[0][1].method).toBe('POST');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      kind: 'organisation',
      name: 'Acme',
    });

    fetchMock = stubFetch(entity);
    await updateEntity('e1', { kind: 'organisation', name: 'Acme 2' }, 3);
    expect(fetchMock.mock.calls[0][1].method).toBe('PUT');
    expect(fetchMock.mock.calls[0][1].headers['If-Match']).toBe('"3"');
  });

  test('deleteEntity returns the cascade counts and restoreEntity POSTs', async () => {
    let fetchMock = stubFetch({ message: 'ok', link_count: 14, projection_count: 14 });
    const res = await deleteEntity('e1', 3);
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE');
    expect(fetchMock.mock.calls[0][1].headers['If-Match']).toBe('"3"');
    expect(res.projection_count).toBe(14);

    fetchMock = stubFetch({ entity, links_revived: 2, same_name_live: false, skipped: [] });
    await restoreEntity('e1');
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/e1\/restore$/);
    expect(fetchMock.mock.calls[0][1].method).toBe('POST');
  });

  test('a non-ok response throws the parsed ApiError', async () => {
    stubFetch({ error: { message: 'nope', code: 'NOT_FOUND' } }, false, 404);
    await expect(getEntity('missing')).rejects.toMatchObject({ status: 404 });
  });
});

describe('merge, suggestions, helper', () => {
  test('mergeEntity posts the survivor and resolutions to the loser path', async () => {
    const fetchMock = stubFetch({ survivor: entity });
    await mergeEntity('loser', { into: 'surv', resolutions: { url: 'loser' } });
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/loser\/merge$/);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      into: 'surv',
      resolutions: { url: 'loser' },
    });
  });

  test('listEntitySuggestions normalises null arrays and passes mode/kind', async () => {
    const fetchMock = stubFetch({
      pairs: null,
      groups: null,
      mode: 'pairs',
      limit: 25,
      next_cursor: '',
    });
    const res = await listEntitySuggestions({ kind: 'organisation', mode: 'pairs' });
    expect(fetchMock.mock.calls[0][0]).toContain('kind=organisation');
    expect(fetchMock.mock.calls[0][0]).toContain('mode=pairs');
    expect(res.pairs).toEqual([]);
    expect(res.groups).toEqual([]);
  });

  test('dismissEntitySuggestion and createEntitiesFromContacts POST their bodies', async () => {
    let fetchMock = stubFetch({ id_low: 'a', id_high: 'b' });
    await dismissEntitySuggestion('a', 'b');
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/suggestions\/dismiss$/);
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ entity_a: 'a', entity_b: 'b' });

    fetchMock = stubFetch({ created: [], skipped: [] });
    await createEntitiesFromContacts({
      kind: 'organisation',
      groups: [{ key: 'acme', role: 'employee', contact_ids: [1] }],
    });
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/from-contacts$/);
  });
});

describe('entity contacts and field values', () => {
  test('listEntityContacts returns [] for a null contacts array', async () => {
    stubFetch({ contacts: null });
    expect(await listEntityContacts('e1')).toEqual([]);
  });

  test('field values read and full-replace', async () => {
    let fetchMock = stubFetch({ field_values: null });
    expect(await listEntityFieldValues('e1')).toEqual([]);
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/entities\/e1\/field-values$/);

    fetchMock = stubFetch({ field_values: [] });
    await replaceEntityFieldValues('e1', [{ field_definition_id: 'd', value: 'x' }]);
    expect(fetchMock.mock.calls[0][1].method).toBe('PUT');
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({
      field_values: [{ field_definition_id: 'd', value: 'x' }],
    });
  });
});

describe('contact entity links', () => {
  test('list normalises a null links array', async () => {
    const fetchMock = stubFetch({ links: null });
    expect(await listContactEntityLinks(7)).toEqual([]);
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/contacts\/7\/entity-links$/);
  });

  test('create / update / delete / adopt / from-existing hit the documented routes', async () => {
    let fetchMock = stubFetch({ id: 'l1' });
    await createContactEntityLink(7, { entity_id: 'e1', role: 'employee', is_primary: true });
    expect(fetchMock.mock.calls[0][1].method).toBe('POST');

    fetchMock = stubFetch({ id: 'l1' });
    await updateContactEntityLink(7, 'l1', { entity_id: 'e1', role: 'owner' });
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/contacts\/7\/entity-links\/l1$/);
    expect(fetchMock.mock.calls[0][1].method).toBe('PUT');

    fetchMock = stubFetch({});
    await deleteContactEntityLink(7, 'l1');
    expect(fetchMock.mock.calls[0][1].method).toBe('DELETE');

    fetchMock = stubFetch({ adopted: true });
    await adoptContactEntityLink(7, 'l1');
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/contacts\/7\/entity-links\/l1\/adopt$/);
    expect(fetchMock.mock.calls[0][1].method).toBe('POST');

    fetchMock = stubFetch({ entity, entity_created: true });
    await linkContactEntityFromExisting(7, {
      role: 'employee',
      source: { kind: 'organization', id: 'org-1' },
    });
    expect(fetchMock.mock.calls[0][0]).toMatch(/\/contacts\/7\/entity-links\/from-existing$/);
  });
});
