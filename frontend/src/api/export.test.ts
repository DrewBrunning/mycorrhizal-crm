import { describe, test, expect, vi, afterEach } from 'vitest';
import {
  exportDataAsCsv,
  exportContacts,
  exportContact,
  exportContactsAsVcf,
  EXPORT_FIELD_SECTIONS,
} from './export';

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

let capturedLink: HTMLAnchorElement | null = null;

function stubDownloadEnvironment(): {
  createObjectURL: ReturnType<typeof vi.fn>;
  revokeObjectURL: ReturnType<typeof vi.fn>;
  click: ReturnType<typeof vi.fn>;
} {
  capturedLink = null;
  const createObjectURL = vi.fn(() => 'blob:http://localhost:7300/export-123');
  const revokeObjectURL = vi.fn();
  const click = vi.spyOn(HTMLElement.prototype, 'click').mockImplementation(function (this: HTMLElement) {
    capturedLink = this as HTMLAnchorElement;
  });
  vi.stubGlobal('URL', { ...URL, createObjectURL, revokeObjectURL });
  return { createObjectURL, revokeObjectURL, click };
}

function fileResponse(filename?: string, headers?: Record<string, string>): Record<string, unknown> {
  return {
    ok: true,
    headers: new Headers({
      ...(filename ? { 'Content-Disposition': `attachment; filename="${filename}"` } : {}),
      ...headers,
    }),
    blob: async () => new Blob(['exported-data']),
  };
}

function errorResponse() {
  return {
    ok: false,
    status: 400,
    statusText: 'Bad Request',
    json: async () => ({
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'Required' } },
      request_id: 'req-1',
    }),
  };
}

describe('EXPORT_FIELD_SECTIONS', () => {
  test('mirrors the backend section tokens with their sensitivity flags', () => {
    expect(EXPORT_FIELD_SECTIONS).toHaveLength(16);
    const tokens = EXPORT_FIELD_SECTIONS.map((s) => s.token);
    expect(tokens).toContain('emails');
    expect(tokens).toContain('phones');
    expect(tokens).toContain('related_to');
    expect(EXPORT_FIELD_SECTIONS.find((s) => s.token === 'related_to')?.sensitive).toBe(true);
    expect(EXPORT_FIELD_SECTIONS.find((s) => s.token === 'custom_fields')?.sensitive).toBe(true);
    expect(EXPORT_FIELD_SECTIONS.find((s) => s.token === 'emails')?.sensitive).toBe(false);
  });
});

describe('exportDataAsCsv', () => {
  test('GETs /export and downloads using the Content-Disposition filename', async () => {
    const { createObjectURL, revokeObjectURL, click } = stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse('contacts-2026.csv'));
    vi.stubGlobal('fetch', fetchMock);

    await exportDataAsCsv();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('contacts-2026.csv');
    expect(capturedLink?.href).toBe('blob:http://localhost:7300/export-123');
    expect(click).toHaveBeenCalledTimes(1);
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:http://localhost:7300/export-123');
  });

  test('falls back to the default filename when Content-Disposition is missing', async () => {
    stubDownloadEnvironment();
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(fileResponse()));

    await exportDataAsCsv();

    expect(capturedLink?.download).toBe('mycorrhizal-export.csv');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(exportDataAsCsv()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('exportContacts', () => {
  test('jscontact export sends sections and include_sensitive to /export/jscontact', async () => {
    const { createObjectURL } = stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContacts('jscontact', { sections: ['emails', 'phones'], includeSensitive: true });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/jscontact');
    expect(url).toContain('sections=emails%2Cphones');
    expect(url).toContain('include_sensitive=true');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('mycorrhizal-contacts.jscontact.json');
    expect(createObjectURL).toHaveBeenCalledWith(expect.any(Blob));
  });

  test('vcf4 export hits /export/vcf without a version param', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContacts('vcf4', { sections: ['emails', 'phones'], includeSensitive: false });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/vcf?');
    expect(url).toContain('sections=emails%2Cphones');
    expect(url).not.toContain('version=');
    expect(url).not.toContain('include_sensitive');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('mycorrhizal-contacts.vcf');
  });

  test('vcf3 export appends version=3 and uses the v3 filename', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContacts('vcf3', { sections: ['emails'], includeSensitive: false });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/vcf?');
    expect(url).toContain('version=3');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('mycorrhizal-contacts-v3.vcf');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(exportContacts('vcf4', { sections: ['emails'], includeSensitive: false })).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('exportContact', () => {
  test('defaults to every section token when no selection is given', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContact('vcf4', 'uid-1');

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/vcf?');
    expect(url).toContain('vcard_uid=uid-1');
    expect((url.match(/sections=/g) || []).length).toBe(EXPORT_FIELD_SECTIONS.length);
    expect(url).not.toContain('include_sensitive');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('uid-1.vcf');
  });

  test('scopes to the selected sections and honors include_sensitive for vcf3', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContact('vcf3', 'uid-1', { sections: ['emails'], includeSensitive: true });

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('vcard_uid=uid-1');
    expect(url).toContain('sections=emails');
    expect(url).toContain('include_sensitive=true');
    expect(url).toContain('version=3');
    expect(capturedLink?.download).toBe('uid-1.vcf');
  });

  test('jscontact export adds vcard_uid and names the file after the uid', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContact('jscontact', 'uid-1');

    const [url] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/jscontact');
    expect(url).toContain('vcard_uid=uid-1');
    expect(capturedLink?.download).toBe('uid-1.json');
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(exportContact('vcf4', 'uid-1')).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});

describe('exportContactsAsVcf', () => {
  test('exports every section to /export/vcf as vCard 4.0', async () => {
    stubDownloadEnvironment();
    const fetchMock = vi.fn().mockResolvedValueOnce(fileResponse());
    vi.stubGlobal('fetch', fetchMock);

    await exportContactsAsVcf();

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/api/v1/export/vcf?');
    expect(url).toContain('sections=emails%2Cphones%2Caddresses');
    expect(url).not.toContain('version=');
    expect(init.method).toBe('GET');
    expect(capturedLink?.download).toBe('mycorrhizal-contacts.vcf');
  });

  test('propagates an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(exportContactsAsVcf()).rejects.toMatchObject({ code: 'VALIDATION_ERROR', status: 400 });
  });
});
