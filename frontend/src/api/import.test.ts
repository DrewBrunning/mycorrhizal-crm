import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  CONTACT_FIELD_LABELS,
  type ColumnMapping,
  confirmImport,
  confirmVCFImport,
  getImportHistory,
  getImportPreview,
  IMPORTABLE_CONTACT_FIELDS,
  REPEATABLE_VALUE_FIELDS,
  type RowImportAction,
  uploadCSVForImport,
  uploadVCFForImport,
} from './import';

afterEach(() => {
  vi.unstubAllGlobals();
});

function okResponse(body?: unknown) {
  return { ok: true, json: async () => body };
}

function errorResponse() {
  return {
    ok: false,
    status: 400,
    statusText: 'Bad Request',
    json: async () => ({
      error: { code: 'VALIDATION_ERROR', message: 'nope', details: { name: 'file' } },
      request_id: 'req-1',
    }),
  };
}

const uploadResponse = {
  session_id: 'sess-1',
  headers: ['First Name', 'Email'],
  suggested_mappings: [
    { csv_column: 'First Name', contact_field: 'firstname', group: 0 },
    { csv_column: 'Email', contact_field: 'email', group: 0 },
  ],
  row_count: 2,
  sample_data: [
    ['Alice', 'alice@example.com'],
    ['Bob', 'bob@example.com'],
  ],
};

const previewResponse = {
  session_id: 'sess-1',
  rows: [
    {
      row_index: 0,
      parsed_contact: { firstname: 'Alice' },
      validation_errors: [],
      duplicate_match: null,
      suggested_action: 'add' as const,
      merge_diff: null,
      batch_duplicate_of: null,
    },
  ],
  total_rows: 1,
  valid_rows: 1,
  duplicate_count: 0,
  error_count: 0,
};

const importResult = {
  total_processed: 1,
  created: 1,
  updated: 0,
  skipped: 0,
  errors: [] as string[],
};

const csvFile = new File(['first,email\nAlice,a@b.com'], 'contacts.csv', { type: 'text/csv' });
const vcfFile = new File(['BEGIN:VCARD\nEND:VCARD'], 'contacts.vcf', { type: 'text/vcard' });

describe('uploadCSVForImport', () => {
  test('POSTs a FormData body without auth headers and returns the upload response', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(uploadResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await uploadCSVForImport(csvFile);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/upload');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.headers).toBeUndefined();
    expect(result).toEqual(uploadResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(uploadCSVForImport(csvFile)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getImportPreview', () => {
  test('POSTs the session id and mappings and returns the preview', async () => {
    const mappings: ColumnMapping[] = [
      { csv_column: 'First Name', contact_field: 'firstname', group: 0 },
      { csv_column: 'Email', contact_field: 'email', group: 0 },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(previewResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getImportPreview('sess-1', mappings);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/preview');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ session_id: 'sess-1', mappings });
    expect(result).toEqual(previewResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getImportPreview('sess-1', [])).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('confirmImport', () => {
  test('POSTs the session id and row actions and returns the result', async () => {
    const actions: RowImportAction[] = [{ row_index: 0, action: 'add' }];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(importResult));
    vi.stubGlobal('fetch', fetchMock);

    const result = await confirmImport('sess-1', actions);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/confirm');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ session_id: 'sess-1', actions });
    expect(result).toEqual(importResult);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(confirmImport('sess-1', [])).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('uploadVCFForImport', () => {
  test('POSTs a FormData body without auth headers and returns the preview', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(previewResponse));
    vi.stubGlobal('fetch', fetchMock);

    const result = await uploadVCFForImport(vcfFile);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/vcf/upload');
    expect(init.method).toBe('POST');
    expect(init.body).toBeInstanceOf(FormData);
    expect(init.headers).toBeUndefined();
    expect(result).toEqual(previewResponse);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(uploadVCFForImport(vcfFile)).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('confirmVCFImport', () => {
  test('POSTs the session id and row actions to the vcf confirm endpoint', async () => {
    const actions: RowImportAction[] = [{ row_index: 0, action: 'add' }];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(importResult));
    vi.stubGlobal('fetch', fetchMock);

    const result = await confirmVCFImport('sess-1', actions);

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/vcf/confirm');
    expect(init.method).toBe('POST');
    expect(JSON.parse(init.body)).toEqual({ session_id: 'sess-1', actions, field_mappings: [] });
    expect(result).toEqual(importResult);
  });

  test('sends the issue #514 field mappings when provided', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(importResult));
    vi.stubGlobal('fetch', fetchMock);

    await confirmVCFImport(
      'sess-1',
      [],
      [
        { property_name: 'x-hometown', action: 'create' },
        { property_name: 'x-favorite-color', action: 'ignore' },
      ],
    );

    const [, init] = fetchMock.mock.calls[0];
    expect(JSON.parse(init.body)).toEqual({
      session_id: 'sess-1',
      actions: [],
      field_mappings: [
        { property_name: 'x-hometown', action: 'create' },
        { property_name: 'x-favorite-color', action: 'ignore' },
      ],
    });
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(confirmVCFImport('sess-1', [])).rejects.toMatchObject({
      code: 'VALIDATION_ERROR',
      status: 400,
    });
  });
});

describe('getImportHistory', () => {
  test('GETs the history endpoint and returns the array', async () => {
    const runs = [
      {
        id: 2,
        format: 'vcf',
        total_processed: 5,
        created: 3,
        updated: 1,
        skipped: 1,
        error_count: 0,
        created_at: '2026-08-27T12:00:00Z',
      },
    ];
    const fetchMock = vi.fn().mockResolvedValueOnce(okResponse(runs));
    vi.stubGlobal('fetch', fetchMock);

    const result = await getImportHistory();

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/import/history');
    expect(init.method).toBe('GET');
    expect(result).toEqual(runs);
  });

  test('throws an ApiError when the response is not ok', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValueOnce(errorResponse()));

    await expect(getImportHistory()).rejects.toMatchObject({ status: 400 });
  });
});

describe('import constants', () => {
  test('REPEATABLE_VALUE_FIELDS covers the four multi-value kinds', () => {
    expect(REPEATABLE_VALUE_FIELDS.has('email')).toBe(true);
    expect(REPEATABLE_VALUE_FIELDS.has('phone')).toBe(true);
    expect(REPEATABLE_VALUE_FIELDS.has('url')).toBe(true);
    expect(REPEATABLE_VALUE_FIELDS.has('impp')).toBe(true);
    expect(REPEATABLE_VALUE_FIELDS.has('firstname')).toBe(false);
  });

  test('IMPORTABLE_CONTACT_FIELDS includes the grouping fields and name parts', () => {
    expect(IMPORTABLE_CONTACT_FIELDS).toContain('circles');
    expect(IMPORTABLE_CONTACT_FIELDS).toContain('tags');
    expect(IMPORTABLE_CONTACT_FIELDS).toContain('firstname');
    expect(IMPORTABLE_CONTACT_FIELDS).toContain('email');
  });

  test('CONTACT_FIELD_LABELS provides a human label for the name fields', () => {
    expect(CONTACT_FIELD_LABELS.firstname).toBe('First Name');
    expect(CONTACT_FIELD_LABELS.lastname).toBe('Last Name');
    expect(CONTACT_FIELD_LABELS.circles).toBe('Circles');
  });
});
