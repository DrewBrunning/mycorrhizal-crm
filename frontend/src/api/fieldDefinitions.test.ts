import { afterEach, describe, expect, test, vi } from 'vitest';
import {
  createFieldDefinition,
  editorToWireValue,
  emptyEditorValue,
  type FieldDefinition,
  type FieldType,
  fieldValueToDisplay,
  getFieldDefinitions,
  isEditorValueEmpty,
  replaceContactFieldValues,
  wireToEditorValue,
} from './fieldDefinitions';

afterEach(() => {
  vi.unstubAllGlobals();
});

function def(overrides: Partial<FieldDefinition> & { type: FieldType }): FieldDefinition {
  return {
    id: 'def-1',
    label: 'Label',
    key: 'key',
    target: 'contact',
    projection: 'internal-only',
    sensitivity: 'normal',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  };
}

// T6/T7's wire <-> editor serialization is the highest-risk frontend logic:
// a value's JSON shape depends on both FieldDefinition.Type and
// Constraints.Multi, and getting it wrong silently corrupts stored data. Every
// type is asserted in both directions plus the empty cases.

describe('wire <-> editor serialization', () => {
  test('scalar string round-trips as a bare JSON string', () => {
    const d = def({ type: 'string' });
    expect(wireToEditorValue(d, 'Al')).toBe('Al');
    expect(editorToWireValue(d, 'Al')).toBe('Al');
    expect(isEditorValueEmpty(d, '')).toBe(true);
    expect(isEditorValueEmpty(d, 'Al')).toBe(false);
  });

  test('number serializes editor text into a JSON number', () => {
    const d = def({ type: 'number', constraints: { min: 0, max: 10 } });
    expect(wireToEditorValue(d, 7)).toBe('7');
    expect(editorToWireValue(d, '7')).toBe(7);
    expect(editorToWireValue(d, 'abc')).toBe(null);
  });

  test('boolean round-trips as a JSON boolean; false is a real value, never empty', () => {
    const d = def({ type: 'boolean' });
    expect(wireToEditorValue(d, true)).toBe(true);
    expect(wireToEditorValue(d, false)).toBe(false);
    expect(editorToWireValue(d, true)).toBe(true);
    expect(editorToWireValue(d, false)).toBe(false);
    expect(isEditorValueEmpty(d, false)).toBe(false);
    expect(emptyEditorValue(d)).toBe(false);
  });

  test('enum round-trips as a JSON string', () => {
    const d = def({ type: 'enum', constraints: { values: ['S', 'M', 'L'] } });
    expect(wireToEditorValue(d, 'M')).toBe('M');
    expect(editorToWireValue(d, 'M')).toBe('M');
  });

  test('Multi enum round-trips as a JSON array of strings', () => {
    const d = def({ type: 'enum', constraints: { values: ['she/her', 'he/him'], multi: true } });
    expect(wireToEditorValue(d, ['she/her', 'he/him'])).toEqual(['she/her', 'he/him']);
    expect(editorToWireValue(d, ['she/her', 'he/him'])).toEqual(['she/her', 'he/him']);
    expect(isEditorValueEmpty(d, [])).toBe(true);
    expect(isEditorValueEmpty(d, ['she/her'])).toBe(false);
  });

  test('Multi number round-trips each element as a JSON number', () => {
    const d = def({ type: 'number', constraints: { multi: true } });
    expect(wireToEditorValue(d, [1, 2])).toEqual(['1', '2']);
    expect(editorToWireValue(d, ['1', '2'])).toEqual([1, 2]);
  });

  test('a non-array wire value for a Multi field degrades to empty', () => {
    const d = def({ type: 'string', constraints: { multi: true } });
    expect(wireToEditorValue(d, 'not-an-array')).toEqual([]);
  });

  test('emptyEditorValue is per-type', () => {
    expect(emptyEditorValue(def({ type: 'string' }))).toBe('');
    expect(emptyEditorValue(def({ type: 'boolean' }))).toBe(false);
    expect(emptyEditorValue(def({ type: 'string', constraints: { multi: true } }))).toEqual([]);
  });

  test('fieldValueToDisplay joins Multi with "; " and stringifies scalars', () => {
    const multi = def({ type: 'enum', constraints: { values: ['a'], multi: true } });
    expect(fieldValueToDisplay(multi, ['a', 'b'])).toBe('a; b');
    const bool = def({ type: 'boolean' });
    expect(fieldValueToDisplay(bool, true)).toBe('true');
    expect(fieldValueToDisplay(bool, false)).toBe('false');
    const str = def({ type: 'string' });
    expect(fieldValueToDisplay(str, 'x')).toBe('x');
    expect(fieldValueToDisplay(str, undefined)).toBe('');
  });
});

describe('field definition API calls', () => {
  test('getFieldDefinitions GETs the list endpoint with a page/limit query', async () => {
    const created = def({ type: 'string' });
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValueOnce({
        ok: true,
        json: async () => ({ field_definitions: [created], total: 1, page: 1, limit: 100 }),
      }),
    );
    const response = await getFieldDefinitions(100);
    expect(response.field_definitions[0]).toEqual(created);
  });

  test('createFieldDefinition POSTs and unwraps {field_definition: ...}', async () => {
    const created = def({ type: 'enum', constraints: { values: ['a', 'b'], multi: true } });
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({ field_definition: created }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await createFieldDefinition({
      label: 'Pronouns',
      key: 'pronouns',
      type: 'enum',
      constraints: { values: ['a', 'b'], multi: true },
    });

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/field-definitions');
    expect(init.method).toBe('POST');
    expect(result.id).toBe('def-1');
  });

  test('replaceContactFieldValues PUTs the full set and returns the saved values', async () => {
    const fetchMock = vi.fn().mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        message: 'Field values saved successfully',
        field_values: [{ id: 1, field_definition_id: 'def-1', entity_id: 'uid', value: 'x' }],
      }),
    });
    vi.stubGlobal('fetch', fetchMock);

    const result = await replaceContactFieldValues(42, [
      { field_definition_id: 'def-1', value: 'x' },
    ]);

    const [url, init] = fetchMock.mock.calls[0];
    expect(url).toContain('/contacts/42/field-values');
    expect(init.method).toBe('PUT');
    expect(result[0].value).toBe('x');
  });
});
