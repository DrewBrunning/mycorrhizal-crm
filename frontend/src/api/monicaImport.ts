// Monica import assistant — the source-specific half (connect + fetch). The
// review/progress/result contract is shared: see sourceImport.ts. Types
// mirror backend/models/monica_import.go by hand (CLAUDE.md frontend trap
// #4 — no dynamic type-list endpoint).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import {
  cancelSourceImport,
  confirmSourceImport,
  getSourceImportPreview,
  getSourceImportStatus,
  type RowSourceActionInput,
  type SourceImportPreviewResponse,
  type SourceImportResult,
  type SourceImportStatus,
} from './sourceImport';

export const MONICA_IMPORT_BASE = '/contacts/import/monica';

export interface MonicaEntityCounts {
  contacts: number;
  activities: number;
  notes: number;
  reminders: number;
  calls: number;
  tasks: number;
  gifts: number;
  debts: number;
}

export interface MonicaConnectResponse {
  session_id: string;
  totals: MonicaEntityCounts;
  estimated_fetch_seconds: number;
}

export interface MonicaFetchOptions {
  include_relationships: boolean;
  include_extras: boolean;
}

// The Monica API token is a third-party credential: it is sent once here and
// then lives only in the server's in-memory session. Never store it client-side.
export async function connectMonica(
  baseURL: string,
  apiToken: string,
): Promise<MonicaConnectResponse> {
  const response = await apiFetch(`${API_BASE_URL}${MONICA_IMPORT_BASE}/connect`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ base_url: baseURL, api_token: apiToken }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function startMonicaFetch(
  sessionId: string,
  opts: MonicaFetchOptions,
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}${MONICA_IMPORT_BASE}/fetch`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ session_id: sessionId, ...opts }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export const getMonicaImportStatus = (sessionId: string): Promise<SourceImportStatus> =>
  getSourceImportStatus(MONICA_IMPORT_BASE, sessionId);

export const getMonicaImportPreview = (sessionId: string): Promise<SourceImportPreviewResponse> =>
  getSourceImportPreview(MONICA_IMPORT_BASE, sessionId);

export const confirmMonicaImport = (
  sessionId: string,
  actions: RowSourceActionInput[],
): Promise<SourceImportResult> => confirmSourceImport(MONICA_IMPORT_BASE, sessionId, actions);

export const cancelMonicaImport = (sessionId: string): Promise<void> =>
  cancelSourceImport(MONICA_IMPORT_BASE, sessionId);
