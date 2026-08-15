// Custom field v2 API calls -- T6/T7 (T6,
// 12-T7-custom-fields-frontend.md). Mirrors backend/models/field_definition.go
// and dtos.go's FieldDefinitionInput/ContactFieldValuesInput by hand -- no
// dynamic schema endpoint exists anywhere in this codebase, so the type token
// list MUST be kept in sync with the backend's `oneof` validators manually.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Closed FieldDefinition.Type set, mirroring backend/models/field_definition.go's
// FieldType* constants exactly.
export type FieldType =
  | 'string'
  | 'text'
  | 'number'
  | 'boolean'
  | 'date'
  | 'datetime'
  | 'uri'
  | 'email'
  | 'phone'
  | 'enum';

export type FieldDefinitionTarget = 'contact';
export type FieldSensitivity = 'normal' | 'private' | 'secret';

// The complete FieldType token set, mirrored by hand from the backend's
// `oneof=string text number boolean date datetime uri email phone enum`
// validator (see the file-header note about manual sync).
export const FIELD_TYPES: FieldType[] = [
  'string', 'text', 'number', 'boolean', 'date', 'datetime',
  'uri', 'email', 'phone', 'enum',
];

export interface FieldConstraints {
  min?: number;
  max?: number;
  maxLength?: number;
  pattern?: string;
  values?: string[];
  multi?: boolean;
}

export interface FieldDefinition {
  id: string;
  label: string;
  key: string;
  target: FieldDefinitionTarget;
  type: FieldType;
  constraints?: FieldConstraints;
  // "internal-only" (default) or "vcard:X-<NAME>" -- see the backend's
  // validateFieldDefinitionProjection.
  projection: string;
  sensitivity: FieldSensitivity;
  created_at: string;
  updated_at: string;
}

export interface FieldDefinitionInput {
  label: string;
  key: string;
  target?: FieldDefinitionTarget;
  type: FieldType;
  constraints?: FieldConstraints;
  projection?: string;
  sensitivity?: FieldSensitivity;
}

// FieldValue.value is the raw JSON payload (§94.4): a scalar definition holds
// a bare value (string/number/boolean), a Multi definition holds an array.
export interface FieldValue {
  id: number;
  field_definition_id: string;
  entity_id: string;
  value: unknown;
  created_at: string;
  updated_at: string;
}

export interface FieldValueInput {
  field_definition_id: string;
  value: unknown;
}

export interface FieldDefinitionsResponse {
  field_definitions: FieldDefinition[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

export async function getFieldDefinitions(limit = 100): Promise<FieldDefinitionsResponse> {
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  const response = await apiFetch(`${API_BASE_URL}/field-definitions?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function createFieldDefinition(input: FieldDefinitionInput): Promise<FieldDefinition> {
  const response = await apiFetch(`${API_BASE_URL}/field-definitions`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.field_definition;
}

export async function updateFieldDefinition(id: string, input: FieldDefinitionInput): Promise<FieldDefinition> {
  const response = await apiFetch(`${API_BASE_URL}/field-definitions/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteFieldDefinition(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/field-definitions/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function getContactFieldValues(contactId: string | number): Promise<FieldValue[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/field-values`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const data = await response.json();
  return data.field_values || [];
}

// replaceContactFieldValues is full-replace: the payload is the complete
// desired value set for the contact, and values for definitions not present
// are deleted server-side (matching the contact PUT full-overwrite contract).
export async function replaceContactFieldValues(
  contactId: string | number,
  fieldValues: FieldValueInput[]
): Promise<FieldValue[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/field-values`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({ field_values: fieldValues }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const data = await response.json();
  return data.field_values || [];
}

// ---------------------------------------------------------------------------
// Wire <-> editor helpers. The editor state is deliberately string-based
// everywhere except boolean (so inputs stay controlled text boxes) and multi
// (an add/remove list of the scalar editor). These serialize/deserialize to
// the wire JSON shape the backend validates.
// ---------------------------------------------------------------------------

export function isMulti(def: FieldDefinition): boolean {
  return !!def.constraints?.multi;
}

// The value type a definition's FieldValue.value carries on the wire.
export type FieldValueEditorState = string | boolean | string[];

export function emptyEditorValue(def: FieldDefinition): FieldValueEditorState {
  if (isMulti(def)) return [];
  if (def.type === 'boolean') return false;
  return '';
}

// A value is "unset" when the user hasn't typed anything -- callers filter
// these out before submitting so a blank row never becomes a stored value.
// Boolean false is a real value and is never filtered.
export function isEditorValueEmpty(def: FieldDefinition, editor: FieldValueEditorState): boolean {
  if (isMulti(def)) return Array.isArray(editor) && editor.length === 0;
  if (def.type === 'boolean') return false;
  return editor === '' || editor === undefined;
}

// Deserialize a wire value into the editor state.
export function wireToEditorValue(def: FieldDefinition, value: unknown): FieldValueEditorState {
  if (isMulti(def)) {
    if (!Array.isArray(value)) return [];
    return value.map((el) => scalarWireToString(el));
  }
  if (def.type === 'boolean') return value === true;
  return scalarWireToString(value);
}

// Serialize the editor state into the wire JSON value.
export function editorToWireValue(def: FieldDefinition, editor: FieldValueEditorState): unknown {
  if (isMulti(def)) {
    const list = Array.isArray(editor) ? editor : [];
    return list.map((el) => scalarEditorToWire(def, el));
  }
  return scalarEditorToWire(def, editor);
}

// Human-readable rendering of a wire value (used by the read-only display in
// ContactInformation). Multi fields join with "; ", matching the CSV export's
// own separator.
export function fieldValueToDisplay(def: FieldDefinition, value: unknown): string {
  if (isMulti(def)) {
    if (!Array.isArray(value)) return '';
    return value.map(scalarWireToString).filter(Boolean).join('; ');
  }
  if (def.type === 'boolean') return value === true ? 'true' : 'false';
  return scalarWireToString(value);
}

function scalarEditorToWire(def: FieldDefinition, editor: FieldValueEditorState): unknown {
  switch (def.type) {
    case 'number': {
      const n = Number(String(editor));
      return Number.isNaN(n) ? null : n;
    }
    case 'boolean':
      return editor === true;
    default:
      return String(editor);
  }
}

function scalarWireToString(value: unknown): string {
  if (value === null || value === undefined) return '';
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  return String(value);
}
