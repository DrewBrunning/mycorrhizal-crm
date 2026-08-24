// CardDAV sync conflict API calls -- issue #395: a remote CardDAV change
// overwrote a local edit by the sync's documented full-replace policy, and
// this notice (which field, the overwritten local value, the remote value
// that replaced it) is the only record of what was lost. The user can
// restore the local value or dismiss the notice.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Field keys -- MUST be kept in sync by hand with
// backend/models/contact_sync_conflict.go's SyncConflictField* constants and
// the backend's sync-conflict snapshot (this repo has no dynamic type-list
// endpoint; every enum is a hardcoded frontend mirror).
export type SyncConflictField =
  | 'firstname'
  | 'lastname'
  | 'middlename'
  | 'prefix'
  | 'suffix'
  | 'nickname'
  | 'organization'
  | 'department'
  | 'job_title'
  | 'role'
  | 'email'
  | 'phone'
  | 'address'
  | 'url'
  | 'impp'
  | 'birthday'
  | 'anniversary'
  | 'circles'
  | 'how_we_met'
  | 'work_information'
  | 'contact_information';

export interface ContactSyncConflict {
  id: string;
  created_at: string;
  updated_at: string;
  subscription_id: number;
  contact_id: number;
  field: SyncConflictField;
  local_value: string;
  remote_value: string;
  status: 'pending' | 'dismissed';
  contact_vcard_uid: string;
  contact_name: string;
  photo_thumbnail?: string;
  subscription_name: string;
}

export interface ContactSyncConflictsResponse {
  sync_conflicts: ContactSyncConflict[];
}

export async function getContactSyncConflicts(): Promise<ContactSyncConflictsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contact-sync-conflicts`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function restoreContactSyncConflict(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contact-sync-conflicts/${id}/restore`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function dismissContactSyncConflict(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contact-sync-conflicts/${id}/dismiss`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
