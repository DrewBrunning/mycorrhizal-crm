// ContactShare API calls -- P1:
// one-time filtered copy of a contact between two users on the same instance.
// Accept/confirm reuse the existing VCF/JSContact import types (api/import.ts)
// since the backend delegates straight to the same import-session pipeline.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { ImportPreviewResponse, ImportResult, RowImportAction } from './import';

export type ContactShareStatus = 'pending' | 'accepted' | 'declined';

export interface ContactShare {
  id: string;
  created_at: string;
  updated_at: string;
  from_user_id: number;
  to_user_id: number;
  contact_display_name: string;
  status: ContactShareStatus;
  responded_at?: string;
}

export interface ContactShareInput {
  to_user_id: number;
  vcard_uid: string;
  sections: string[];
  include_sensitive?: boolean;
}

export interface ContactShareListResponse {
  contact_shares: ContactShare[];
  // Keyed by the OTHER party's user ID (as a string, since it round-trips
  // through JSON object keys) -> their username.
  usernames: Record<string, string>;
  total: number;
  next_cursor: string;
  limit: number;
}

export interface UserDirectoryEntry {
  id: number;
  username: string;
}

// GET /users/directory. `q` is the typed recipient search: the server's
// SHARE_DIRECTORY_MODE=search matches a username prefix (3+ characters) and returns
// nothing otherwise; SHARE_DIRECTORY_MODE=all (the default) ignores it. A blank query
// sends no parameter at all.
export async function getUserDirectory(q?: string): Promise<UserDirectoryEntry[]> {
  const query = q?.trim();
  const suffix = query ? `?q=${encodeURIComponent(query)}` : '';
  const response = await apiFetch(`${API_BASE_URL}/users/directory${suffix}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.users;
}

export async function createContactShare(input: ContactShareInput): Promise<ContactShare> {
  const response = await apiFetch(`${API_BASE_URL}/contact-shares`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.contact_share;
}

export interface GetContactSharesParams {
  cursor?: string;
  limit?: number;
}

export async function getIncomingContactShares(
  params: GetContactSharesParams = {},
): Promise<ContactShareListResponse> {
  const queryParams = new URLSearchParams();
  if (params.cursor) queryParams.append('cursor', params.cursor);
  if (params.limit) queryParams.append('limit', params.limit.toString());
  const response = await apiFetch(
    `${API_BASE_URL}/contact-shares/incoming?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getOutgoingContactShares(
  params: GetContactSharesParams = {},
): Promise<ContactShareListResponse> {
  const queryParams = new URLSearchParams();
  if (params.cursor) queryParams.append('cursor', params.cursor);
  if (params.limit) queryParams.append('limit', params.limit.toString());
  const response = await apiFetch(
    `${API_BASE_URL}/contact-shares/outgoing?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Preview-only: parses the share's stored payload through the import
// pipeline and returns what confirmContactShare will need. Does NOT change
// the share's status.
export async function acceptContactShare(id: string): Promise<ImportPreviewResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contact-shares/${id}/accept`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Finalizes an accepted share using the recipient's chosen per-row actions
// (same add/update/skip contract as confirmVCFImport). Only on success does
// the share flip to accepted.
export async function confirmContactShare(
  id: string,
  sessionId: string,
  actions: RowImportAction[],
): Promise<ImportResult> {
  const response = await apiFetch(`${API_BASE_URL}/contact-shares/${id}/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ session_id: sessionId, actions }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function declineContactShare(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contact-shares/${id}/decline`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
