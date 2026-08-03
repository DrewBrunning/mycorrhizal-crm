// Immich API calls — T15/T16 (docs/fork-plan/tickets/33-T15-T16-immich.md):
// the first concrete integration on the generic substrate. Connection config
// is per-user-global; links are written as ExternalIdentity (system:
// "immich"); enrichment lands as ExternalActivity. All of these go through
// the backend, never straight to Immich (the API key stays server-side,
// encrypted at rest).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface ImmichConfigResponse {
  base_url: string;
  has_api_key: boolean;
  sync_enabled: boolean;
  last_synced_at?: string | null;
  last_sync_status: string;
  last_sync_error: string;
}

export interface ImmichConfigInput {
  base_url: string;
  // Write-only: empty on update keeps the stored key; required on first connect.
  api_key?: string;
  sync_enabled?: boolean;
}

export interface ImmichPerson {
  id: string;
  name: string;
}

export interface ImmichPersonSummary {
  identity: {
    id: string;
    entity_id: string;
    system: string;
    external_id: string;
    url?: string;
    metadata?: Record<string, unknown>;
    sync_status: string;
    last_synced_at?: string | null;
  };
  person_name: string;
  photo_count: number;
  latest_asset_id?: string;
  latest_at?: string | null;
}

export async function getImmichConfig(): Promise<ImmichConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/immich/config`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function saveImmichConfig(input: ImmichConfigInput): Promise<ImmichConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/immich/config`, {
    method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteImmichConfig(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/immich/config`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function getImmichPeople(): Promise<ImmichPerson[]> {
  const response = await apiFetch(`${API_BASE_URL}/immich/people`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.people || [];
}

export async function linkImmichPerson(contactUid: string, personId: string, personName: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/immich/contacts/${contactUid}/link`, {
    method: 'POST', headers: getAuthHeaders(),
    body: JSON.stringify({ person_id: personId, person_name: personName }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function unlinkImmichPerson(contactUid: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/immich/contacts/${contactUid}/link`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function getImmichContactSummary(contactUid: string): Promise<ImmichPersonSummary | null> {
  const response = await apiFetch(`${API_BASE_URL}/immich/contacts/${contactUid}/summary`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.summary ?? null;
}

export async function syncImmich(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/immich/sync`, {
    method: 'POST', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// The proxied person-thumbnail URL for a contact's Immich link, served with
// the same hardening as /proxy/image (SVG rejected). The backend resolves the
// person through the contact's ExternalIdentity and fetches with the user's
// own connection credentials.
export function immichThumbnailUrl(contactUid: string): string {
  return `${API_BASE_URL}/immich/contacts/${contactUid}/thumbnail`;
}
