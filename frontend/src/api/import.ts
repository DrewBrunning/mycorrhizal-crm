import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// Column mapping between CSV column and contact field
export interface ColumnMapping {
  csv_column: string;
  contact_field: string;
  // Multi-value entry index (0-based). Ties a value column to its label/parts within the
  // same logical entry (e.g. "E-mail 1 - Value" + "E-mail 1 - Label")
  group: number;
}

// Response from CSV upload
export interface ImportUploadResponse {
  session_id: string;
  headers: string[];
  suggested_mappings: ColumnMapping[];
  row_count: number;
  sample_data: string[][];
}

// Match info for duplicate detection
export interface DuplicateMatch {
  existing_contact_id: number;
  existing_firstname: string;
  existing_lastname: string;
  existing_email: string;
  existing_phone: string;
  match_reason: 'name' | 'email' | 'phone';
}

// One scalar field the "Merge" (update) action will overwrite on the matched
// existing contact (T96). Field is a stable camelCase key; label is the
// human-readable fallback.
export interface ImportScalarChange {
  field: string;
  label: string;
  old: string;
  new: string;
}

// One multi-valued entry the "Merge" action will append to the matched
// existing contact. Kind is one of email/phone/address/url/impp.
export interface ImportAddedValue {
  kind: string;
  value: string;
}

// Exactly what "Merge" will change on the matched existing contact: scalars
// overwritten (incoming-wins-when-non-empty) and multi-valued entries
// appended (additive).
export interface ImportMergeDiff {
  updated: ImportScalarChange[];
  added: ImportAddedValue[];
}

// One degradation-policy event from a format adapter, in the same structured
// severity/concept/message shape the export loss reports use (DATA-02, issue
// #442). Mirrors backend contactmodel.Diagnostic.
export interface ImportDiagnostic {
  severity: 'warn' | 'info';
  concept?: string;
  message: string;
}

// Preview row with parsed contact and status
export interface ImportRowPreview {
  row_index: number;
  parsed_contact: Record<string, string>;
  validation_errors: string[];
  duplicate_match: DuplicateMatch | null;
  suggested_action: 'add' | 'skip' | 'update';
  // T96: present exactly when duplicate_match is -- what "Merge" would change.
  merge_diff: ImportMergeDiff | null;
  // T96: when present, the row_index of an earlier row in the SAME import that
  // this row duplicates. Such rows default to "skip".
  batch_duplicate_of: number | null;
  // Issue #514: the unknown X-* vCard/JSContact properties discovered on this
  // row that can be promoted to custom fields. Absent when the row carried none.
  custom_field_candidates?: DiscoveredCustomProperty[];
  // DATA-02 (issue #442): adapter degradation diagnostics for this row
  // (empty/absent for CSV/records-import rows, which don't go through an
  // adapter).
  diagnostics?: ImportDiagnostic[];
}

// One unknown X-* property found on an imported contact that can be promoted
// to a custom field (issue #514). Mirrors backend models.DiscoveredProperty.
export interface DiscoveredCustomProperty {
  // The vCard property name as stored in passthrough (lowercase, e.g.
  // "x-favorite-color").
  name: string;
  // Human-readable rendering of the property's value.
  value: string;
  // When present, an existing FieldDefinition whose vcard:X-<NAME> projection
  // matches this property. The wizard pre-selects "map to this definition"
  // and the confirm path auto-promotes absent an override.
  matched_definition_id?: string;
  matched_definition_label?: string;
}

// Response from preview request
export interface ImportPreviewResponse {
  session_id: string;
  rows: ImportRowPreview[];
  total_rows: number;
  valid_rows: number;
  duplicate_count: number;
  error_count: number;
}

// Action for a specific row
export interface RowImportAction {
  row_index: number;
  action: 'skip' | 'add' | 'update';
}

// One decision from the issue #514 "Custom fields" wizard step: promote a
// discovered X-* property into an existing ("map") or newly-created ("create")
// custom-field definition, or keep it in passthrough ("ignore", the default).
// Mirrors backend models.ImportFieldMapping.
export interface ImportFieldMapping {
  property_name: string;
  action: 'ignore' | 'map' | 'create';
  // Required when action === 'map'.
  field_definition_id?: string;
  // Used when action === 'create': the new definition's label. Defaults to a
  // title-cased derivation of the property name server-side.
  label?: string;
  // Used when action === 'create': the new definition's type. Defaults to
  // 'text' server-side.
  type?: string;
}

// Final import result
export interface ImportResult {
  total_processed: number;
  created: number;
  updated: number;
  skipped: number;
  errors: string[];
}

// One persisted import outcome (issue #651). Mirrors backend models.ImportRun
// and migration 000042's `format` CHECK vocabulary — if a token is added
// backend-side, add it here too (no dynamic type-list endpoint by design).
export interface ImportRun {
  id: number;
  format: 'csv' | 'vcf' | 'jscontact' | 'records';
  total_processed: number;
  created: number;
  updated: number;
  skipped: number;
  error_count: number;
  created_at: string;
}

// Contact fields that can be imported (mirrors backend models.ImportableContactFields)
export const IMPORTABLE_CONTACT_FIELDS = [
  // Name
  'firstname',
  'lastname',
  'middle_name',
  'prefix',
  'suffix',
  'nickname',
  'gender',
  // Dates
  'birthday',
  'anniversary',
  // Multi-value: email / phone
  'email',
  'email_label',
  'phone',
  'phone_label',
  // Multi-value: address parts
  'address_street',
  'address_city',
  'address_region',
  'address_postal',
  'address_country',
  'address_label',
  // Multi-value: web / IM
  'url',
  'url_label',
  'impp',
  'impp_label',
  // Organization
  'organization',
  'department',
  'job_title',
  'role',
  // Free-text
  'how_we_met',
  'work_information',
  'contact_information',
  // Groupings. Distinct targets since T3: 'circles' creates Circle +
  // membership rows, 'tags' creates Tag + assignment rows. Both were missing
  // from this list entirely, so the mapping dropdown could not offer them
  // even though the backend accepted 'circles'.
  'circles',
  'tags',
] as const;

export const REPEATABLE_VALUE_FIELDS = new Set<string>(['email', 'phone', 'url', 'impp']);

// Human-readable labels for contact fields
export const CONTACT_FIELD_LABELS: Record<string, string> = {
  firstname: 'First Name',
  lastname: 'Last Name',
  middle_name: 'Middle Name',
  prefix: 'Name Prefix',
  suffix: 'Name Suffix',
  nickname: 'Nickname',
  gender: 'Gender',
  birthday: 'Birthday',
  anniversary: 'Anniversary',
  email: 'Email',
  email_label: 'Email – Type',
  phone: 'Phone',
  phone_label: 'Phone – Type',
  address_street: 'Address – Street',
  address_city: 'Address – City',
  address_region: 'Address – Region/State',
  address_postal: 'Address – Postal Code',
  address_country: 'Address – Country',
  address_label: 'Address – Type',
  url: 'Website',
  url_label: 'Website – Type',
  impp: 'IM / Social',
  impp_label: 'IM / Social – Type',
  organization: 'Organization',
  department: 'Department',
  job_title: 'Job Title',
  role: 'Role',
  how_we_met: 'How We Met',
  work_information: 'Work Information',
  contact_information: 'Contact Information',
  circles: 'Circles',
  tags: 'Tags',
};

// Upload a CSV file for import
export async function uploadCSVForImport(file: File): Promise<ImportUploadResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await apiFetch(`${API_BASE_URL}/contacts/import/upload`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get import preview with applied mappings
export async function getImportPreview(
  sessionId: string,
  mappings: ColumnMapping[],
): Promise<ImportPreviewResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/preview`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      mappings,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Confirm and execute the import (CSV)
export async function confirmImport(
  sessionId: string,
  actions: RowImportAction[],
): Promise<ImportResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      actions,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Upload a VCF file for import (returns preview directly, no mapping needed)
export async function uploadVCFForImport(file: File): Promise<ImportPreviewResponse> {
  const formData = new FormData();
  formData.append('file', file);

  const response = await apiFetch(`${API_BASE_URL}/contacts/import/vcf/upload`, {
    method: 'POST',
    body: formData,
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Recent import outcomes for the current user, newest first (issue #651).
export async function getImportHistory(): Promise<ImportRun[]> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/history`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Confirm and execute VCF import (with photo processing)
export async function confirmVCFImport(
  sessionId: string,
  actions: RowImportAction[],
  fieldMappings: ImportFieldMapping[] = [],
): Promise<ImportResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/import/vcf/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      actions,
      field_mappings: fieldMappings,
    }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}
