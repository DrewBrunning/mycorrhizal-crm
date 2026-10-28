// Bulk contact operations (N5 — N5).
// A single batch endpoint runs one action across many contacts with
// partial-success semantics. Mirrors backend/models/bulk_operation.go and
// controllers/bulk_operation_controller.go.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// The explicit allowlist of core fields "update_fields" may write (issue
// #1996). Mirrors models.BulkEditableFields; multi-valued fields (addresses,
// emails, phones) are deliberately absent.
export const BULK_EDITABLE_FIELDS = [
  'organization',
  'department',
  'job_title',
  'role',
  'gender',
  'how_we_met',
  'work_information',
  'contact_information',
] as const;
export type BulkEditableField = (typeof BULK_EDITABLE_FIELDS)[number];

export const BULK_ACTIONS = [
  'add_circle',
  'remove_circle',
  'add_tag',
  'remove_tag',
  'archive',
  'unarchive',
  'delete',
  'update_fields',
] as const;
export type BulkAction = (typeof BULK_ACTIONS)[number];

export interface BulkOperationRequest {
  action: BulkAction;
  // Targets are Contact.VCardUID values (membership rows are keyed by it).
  vcard_uids: string[];
  circle_id?: string;
  tag_id?: string;
  // update_fields only: a present `set` key is written (non-empty), a `clear`
  // key is emptied, anything in neither is untouched.
  set?: Partial<Record<BulkEditableField, string>>;
  clear?: BulkEditableField[];
  custom_set?: { field_definition_id: string; value: unknown }[];
  custom_clear?: string[];
}

export interface BulkFailureEntry {
  vcard_uid: string;
  reason: string;
}

export interface BulkOperationResult {
  action: BulkAction;
  total: number;
  succeeded: number;
  failed: number;
  failures: BulkFailureEntry[];
}

// POST /contacts/bulk
export async function runBulkOperation(input: BulkOperationRequest): Promise<BulkOperationResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/bulk`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
