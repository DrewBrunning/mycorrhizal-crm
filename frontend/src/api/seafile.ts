// Seafile API calls — issue #156 (P2b): link contacts to files/folders in a
// self-hosted Seafile library. Connection config is per-user-global; links are
// written as ExternalIdentity (system: "seafile"). All of these go through the
// backend, never straight to Seafile (the API token stays server-side,
// encrypted at rest).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface SeafileConfigResponse {
  base_url: string;
  has_api_token: boolean;
}

export interface SeafileConfigInput {
  base_url: string;
  // Write-only: empty on update keeps the stored token; required on first connect.
  api_token?: string;
}

export interface SeafileConnectionTestResult {
  ok: boolean;
  stage: string;
  message: string;
}

export interface SeafileLibrary {
  id: string;
  name: string;
  type: string;
}

export interface SeafileItem {
  id: string;
  name: string;
  type: 'file' | 'dir';
  size?: number;
  mtime?: number;
  parent_dir?: string;
}

export async function getSeafileConfig(): Promise<SeafileConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/config`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function saveSeafileConfig(input: SeafileConfigInput): Promise<SeafileConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/config`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteSeafileConfig(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/config`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function testSeafileConnection(): Promise<SeafileConnectionTestResult> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/test-connection`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getSeafileLibraries(): Promise<SeafileLibrary[]> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/libraries`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.libraries || [];
}

export async function getSeafileDir(repoId: string, path: string): Promise<SeafileItem[]> {
  const params = new URLSearchParams({ path });
  const response = await apiFetch(
    `${API_BASE_URL}/seafile/libraries/${encodeURIComponent(repoId)}/dir?${params.toString()}`,
    {
      headers: getAuthHeaders(),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.items || [];
}

export async function linkSeafileItem(
  contactUid: string,
  item: {
    repo_id: string;
    path: string;
    name: string;
    type: 'file' | 'dir';
    size?: number;
    mtime?: number;
  },
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/seafile/contacts/${contactUid}/link`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(item),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function unlinkSeafileItem(contactUid: string, identityId: string): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/seafile/contacts/${contactUid}/links/${identityId}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
}
