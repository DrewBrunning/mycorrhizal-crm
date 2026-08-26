// Bulk contact operations (N5 — N5).
// A single batch endpoint runs one action across many contacts with
// partial-success semantics. Mirrors backend/models/bulk_operation.go and
// controllers/bulk_operation_controller.go.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export const BULK_ACTIONS = [
  'add_circle',
  'remove_circle',
  'add_tag',
  'remove_tag',
  'archive',
  'unarchive',
  'delete',
] as const;
export type BulkAction = (typeof BULK_ACTIONS)[number];

export interface BulkOperationRequest {
  action: BulkAction;
  // Targets are Contact.VCardUID values (membership rows are keyed by it).
  vcard_uids: string[];
  circle_id?: string;
  tag_id?: string;
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
