import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  getLinkFieldTypes,
  createLinkFieldType,
  updateLinkFieldType,
  deleteLinkFieldType,
  reorderLinkFieldTypes,
  LinkFieldType,
} from './linkFieldTypes';

afterEach(() => {
  vi.unstubAllGlobals();
});

const linkFieldType: LinkFieldType = {
  id: 'lft-1',
  name: 'WhatsApp',
  protocol: 'https://wa.me/{value}',
  category: 'messaging',
  is_default: true,
  position: 0,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

describe('getLinkFieldTypes', () => {
  test('fetches and returns the list', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ link_field_types: [linkFieldType] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getLinkFieldTypes();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/link-field-types');
    expect(init.method).toBeUndefined();
    expect(result).toEqual([linkFieldType]);
  });

  test('defaults to an empty array when link_field_types is absent (defensive against a null-ish response)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({}) });
    vi.stubGlobal('fetch', fetchMock);

    const result = await getLinkFieldTypes();
    expect(result).toEqual([]);
  });
});

describe('createLinkFieldType', () => {
  test('POSTs and unwraps the wrapped link_field_type', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ message: 'Link field type created', link_field_type: linkFieldType }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createLinkFieldType({ name: 'WhatsApp', protocol: 'https://wa.me/{value}', category: 'messaging' });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/link-field-types');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body).name).toBe('WhatsApp');
    expect(result).toEqual(linkFieldType);
  });
});

describe('updateLinkFieldType', () => {
  test('PUTs to the id and returns the raw type (NOT wrapped, unlike create)', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => linkFieldType });
    vi.stubGlobal('fetch', fetchMock);

    const result = await updateLinkFieldType('lft-1', { name: 'WhatsApp', protocol: 'https://wa.me/{value}', category: 'messaging' });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/link-field-types/lft-1');
    expect(init.method).toBe('PUT');
    expect(result).toEqual(linkFieldType);
  });
});

describe('deleteLinkFieldType', () => {
  test('DELETEs the type', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({ ok: true, json: async () => ({ message: 'Link field type deleted' }) });
    vi.stubGlobal('fetch', fetchMock);

    await deleteLinkFieldType('lft-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/link-field-types/lft-1');
    expect(init.method).toBe('DELETE');
  });
});

describe('reorderLinkFieldTypes', () => {
  test('PUTs the full ordered id list and returns the reordered set', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ link_field_types: [linkFieldType] }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await reorderLinkFieldTypes(['lft-1']);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/link-field-types/reorder');
    expect(init.method).toBe('PUT');
    expect(JSON.parse(init.body)).toEqual({ order: ['lft-1'] });
    expect(result).toEqual([linkFieldType]);
  });
});

describe('error handling', () => {
  test('a non-ok response throws a parsed ApiError rather than resolving', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: false,
      status: 409,
      json: async () => ({ error: { code: 'ALREADY_EXISTS', message: 'Link field type already exists' } }),
    });
    vi.stubGlobal('fetch', fetchMock);

    await expect(createLinkFieldType({ name: 'WhatsApp', protocol: '', category: 'messaging' })).rejects.toThrow(
      'Link field type already exists'
    );
  });
});
