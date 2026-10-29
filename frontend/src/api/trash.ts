// Contact trash API calls -- issue #2002, ADR 0037. Mirrors
// backend/models/trash_dto.go (pinned to the generated types by
// contractConformance.ts).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { ContactSummaryDTO } from './contacts';

export interface TrashedContact {
  contact: ContactSummaryDTO;
  deleted_at: string;
  // When the scheduled purge hard-deletes the contact; null when retention is 0.
  purge_at: string | null;
}

export interface TrashList {
  contacts: TrashedContact[];
  total: number;
  page: number;
  limit: number;
  // DELETED_RETENTION_DAYS; 0 means trashed contacts are kept forever.
  retention_days: number;
}

export interface TrashRestoreSkip {
  table: string;
  reason: string;
  count: number;
}

export interface TrashRestoreResult {
  contact: ContactSummaryDTO;
  skipped: TrashRestoreSkip[];
}

export async function getTrashedContacts(
  params: { page?: number; limit?: number } = {},
): Promise<TrashList> {
  const query = new URLSearchParams({
    page: (params.page ?? 1).toString(),
    limit: (params.limit ?? 25).toString(),
  });
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts?${query.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const data: Partial<TrashList> = await response.json();
  return {
    contacts: data.contacts ?? [],
    total: data.total ?? 0,
    page: data.page ?? 1,
    limit: data.limit ?? 25,
    retention_days: data.retention_days ?? 0,
  };
}

// A 409 (ApiError.status) means a live contact already uses the trashed
// contact's UID; nothing changed.
export async function restoreTrashedContact(id: number): Promise<TrashRestoreResult> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts/${id}/restore`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const data: Partial<TrashRestoreResult> & { contact: ContactSummaryDTO } = await response.json();
  return { contact: data.contact, skipped: data.skipped ?? [] };
}

export async function purgeTrashedContact(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// Returns how many contacts were purged.
export async function emptyTrash(): Promise<number> {
  const response = await apiFetch(`${API_BASE_URL}/trash/contacts`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const data: { purged?: number } = await response.json();
  return data.purged ?? 0;
}
