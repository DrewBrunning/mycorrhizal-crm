// Nextcloud / ownCloud (WebDAV) API calls — issue #157 (P2c): link contacts to
// files/folders over standard WebDAV. Connection config is per-user-global;
// links are written as ExternalIdentity (system: "nextcloud"). All of these go
// through the backend, never straight to the instance (the app password stays
// server-side, encrypted at rest).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface WebDAVConfigResponse {
  base_url: string;
  username: string;
  has_app_password: boolean;
}

export interface WebDAVConfigInput {
  base_url: string;
  username: string;
  // Write-only: empty on update keeps the stored password; required on first
  // connect. Only an app password is accepted — never the account password.
  app_password?: string;
}

export interface WebDAVConnectionTestResult {
  ok: boolean;
  stage: string;
  message: string;
}

export interface WebDAVItem {
  name: string;
  // WebDAV path relative to the user's dav root, starting with "/".
  path: string;
  type: 'file' | 'dir';
  size?: number;
  modified_at?: string;
  file_id?: string;
}

export async function getNextcloudConfig(): Promise<WebDAVConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/config`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function saveNextcloudConfig(input: WebDAVConfigInput): Promise<WebDAVConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/config`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteNextcloudConfig(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/config`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function testNextcloudConnection(): Promise<WebDAVConnectionTestResult> {
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/test-connection`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getNextcloudDir(path?: string): Promise<WebDAVItem[]> {
  const params = path ? `?path=${encodeURIComponent(path)}` : '';
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/dir${params}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.items || [];
}

export async function linkNextcloudItem(
  contactUid: string,
  item: {
    path: string;
    name: string;
    type: 'file' | 'dir';
    size?: number;
    modified_at?: string;
    file_id?: string;
  },
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/nextcloud/contacts/${contactUid}/link`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(item),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function unlinkNextcloudItem(contactUid: string, identityId: string): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/nextcloud/contacts/${contactUid}/links/${identityId}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
}
