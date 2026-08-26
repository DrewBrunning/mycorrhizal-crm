// Paperless-ngx API calls — issue #155 (P2a): link contacts to documents in a
// self-hosted Paperless-ngx instance. Connection config is per-user-global;
// links are written as ExternalIdentity (system: "paperless"). All of these go
// through the backend, never straight to Paperless (the API token stays
// server-side, encrypted at rest).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface PaperlessConfigResponse {
  base_url: string;
  has_api_token: boolean;
}

export interface PaperlessConfigInput {
  base_url: string;
  // Write-only: empty on update keeps the stored token; required on first connect.
  api_token?: string;
}

export interface PaperlessDocument {
  id: number;
  title: string;
  file_name?: string;
  created?: string;
  added?: string;
}

export interface PaperlessConnectionTestResult {
  ok: boolean;
  stage: string;
  message: string;
}

export async function getPaperlessConfig(): Promise<PaperlessConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/paperless/config`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function savePaperlessConfig(
  input: PaperlessConfigInput,
): Promise<PaperlessConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/paperless/config`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deletePaperlessConfig(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/paperless/config`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// Diagnoses the currently saved connection — reachability then API token
// validity — reporting which stage failed rather than a generic error. A
// diagnosed failure (ok: false) is still a 200; only a missing/unparseable
// saved connection throws.
export async function testPaperlessConnection(): Promise<PaperlessConnectionTestResult> {
  const response = await apiFetch(`${API_BASE_URL}/paperless/test-connection`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getPaperlessDocuments(query?: string): Promise<PaperlessDocument[]> {
  const params = query ? `?query=${encodeURIComponent(query)}` : '';
  const response = await apiFetch(`${API_BASE_URL}/paperless/documents${params}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.documents || [];
}

export async function linkPaperlessDocument(contactUid: string, documentId: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/paperless/contacts/${contactUid}/link`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ document_id: String(documentId) }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function unlinkPaperlessDocument(
  contactUid: string,
  identityId: string,
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/paperless/contacts/${contactUid}/links/${identityId}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
}
