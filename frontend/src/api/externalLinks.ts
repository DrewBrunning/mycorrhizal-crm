// ExternalIdentity / ExternalActivity API calls — T14
// (docs/fork-plan/tickets/32-T14-external-link-substrate.md): the generic
// integration substrate. ExternalIdentity = "this contact IS this thing in
// that external system"; ExternalActivity = something that happened in an
// external system, linkable into a contact's timeline. System-agnostic — no
// integration-specific fields live here (they go in metadata/payload).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface ExternalIdentity {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  system: string;
  external_id: string;
  url?: string;
  metadata?: Record<string, unknown>;
  sync_status: 'idle' | 'syncing' | 'synced' | 'error';
  last_synced_at?: string | null;
}

export interface ExternalIdentityInput {
  entity_id: string;
  system: string;
  external_id: string;
  url?: string;
  metadata?: Record<string, unknown>;
  sync_status?: 'idle' | 'syncing' | 'synced' | 'error';
}

export interface ExternalActivity {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  source_system: string;
  external_id: string;
  type: string;
  occurred_at: string;
  payload?: Record<string, unknown>;
  provenance: 'external' | 'user';
  sync_state: 'synced' | 'pending';
}

export interface ExternalActivityInput {
  entity_id: string;
  source_system: string;
  external_id: string;
  type: string;
  occurred_at: string;
  payload?: Record<string, unknown>;
  provenance?: 'external' | 'user';
  sync_state?: 'synced' | 'pending';
}

export interface ExternalIdentitiesResponse {
  external_identities: ExternalIdentity[];
  total: number;
  next_cursor: string;
  limit: number;
}

export interface ExternalActivitiesResponse {
  external_activities: ExternalActivity[];
  total: number;
  next_cursor: string;
  limit: number;
}

export async function getExternalIdentities(params?: {
  contactId?: string;
  system?: string;
  cursor?: string;
  limit?: number;
}): Promise<ExternalIdentitiesResponse> {
  const { contactId, system, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (contactId) queryParams.append('contact_id', contactId);
  if (system) queryParams.append('system', system);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(`${API_BASE_URL}/external-identities?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST is wrapped in {external_identity: ...} (create asymmetry, same as
// relationship-edges).
export async function createExternalIdentity(input: ExternalIdentityInput): Promise<ExternalIdentity> {
  const response = await apiFetch(`${API_BASE_URL}/external-identities`, {
    method: 'POST', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.external_identity;
}

// PUT returns the raw identity, not wrapped.
export async function updateExternalIdentity(id: string, input: ExternalIdentityInput): Promise<ExternalIdentity> {
  const response = await apiFetch(`${API_BASE_URL}/external-identities/${id}`, {
    method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteExternalIdentity(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/external-identities/${id}`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function getExternalActivities(params?: {
  contactId?: string;
  system?: string;
  cursor?: string;
  limit?: number;
}): Promise<ExternalActivitiesResponse> {
  const { contactId, system, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (contactId) queryParams.append('contact_id', contactId);
  if (system) queryParams.append('system', system);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(`${API_BASE_URL}/external-activities?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function createExternalActivity(input: ExternalActivityInput): Promise<ExternalActivity> {
  const response = await apiFetch(`${API_BASE_URL}/external-activities`, {
    method: 'POST', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.external_activity;
}

export async function deleteExternalActivity(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/external-activities/${id}`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
