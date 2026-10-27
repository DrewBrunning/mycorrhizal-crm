// Meerkat import assistant — the source-specific half (upload + fetch). The
// review/progress/result contract is shared: see sourceImport.ts. Types
// mirror backend/models/meerkat_import.go by hand (CLAUDE.md frontend trap
// #4 — no dynamic type-list endpoint).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import {
  cancelSourceImport,
  confirmSourceImport,
  getSourceImportPreview,
  getSourceImportStatus,
  type RowSourceActionInput,
  type SourceImportPreviewResponse,
  type SourceImportStatus,
} from './sourceImport';

export const MEERKAT_IMPORT_BASE = '/contacts/import/meerkat';

export interface MeerkatSourceUser {
  id: number;
  username: string;
  email: string;
  name: string;
  contacts: number;
}

export interface MeerkatEntityCounts {
  contacts: number;
  relationships: number;
  notes: number;
  activities: number;
  reminders: number;
}

export interface MeerkatUploadResponse {
  session_id: string;
  source_users: MeerkatSourceUser[];
  default_source_user_id?: number;
  totals: MeerkatEntityCounts;
}

// Uploads the Meerkat SQLite file. The server validates it (magic header,
// size, extension), stages it as a temp file, and returns the source-user
// picker + whole-file totals. The file is never stored in the app database.
export async function uploadMeerkatDatabase(file: File): Promise<MeerkatUploadResponse> {
  const form = new FormData();
  form.append('file', file);
  const response = await apiFetch(`${API_BASE_URL}${MEERKAT_IMPORT_BASE}/upload`, {
    method: 'POST',
    body: form,
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Mirrors MeerkatConnectRequest/Response (backend/models/meerkat_import.go).
// Exactly one of api_token or username+password is sent. These are third-party
// credentials: they go to the server once and live only in its in-memory
// session — never store them client-side.
export interface MeerkatConnectRequest {
  base_url: string;
  api_token?: string;
  username?: string;
  password?: string;
}

export interface MeerkatVersionInfo {
  generation: string;
  supported: boolean;
  // English fallback for a reason_code this client does not translate.
  reason: string;
  // Stable form of reason: 'meerkat_version_older_than_supported' |
  // 'meerkat_version_newer_than_tested'; empty when supported. More may be
  // added, so an unknown code falls back to reason.
  reason_code: string;
}

export interface MeerkatConnectResponse {
  session_id: string;
  version: MeerkatVersionInfo;
  source_user: MeerkatSourceUser;
  totals: MeerkatEntityCounts;
  insecure_transport: boolean;
  estimated_fetch_seconds: number;
}

export async function connectMeerkat(req: MeerkatConnectRequest): Promise<MeerkatConnectResponse> {
  const response = await apiFetch(`${API_BASE_URL}${MEERKAT_IMPORT_BASE}/connect`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(req),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// acceptUntested is the explicit "try anyway" for a Meerkat version the
// adapter was not built against; the server refuses an untested fetch without it.
export async function startMeerkatFetch(
  sessionId: string,
  sourceUserId?: number,
  acceptUntested?: boolean,
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}${MEERKAT_IMPORT_BASE}/fetch`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      source_user_id: sourceUserId ?? null,
      ...(acceptUntested ? { accept_untested: true } : {}),
    }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export const getMeerkatImportStatus = (sessionId: string): Promise<SourceImportStatus> =>
  getSourceImportStatus(MEERKAT_IMPORT_BASE, sessionId);

export const getMeerkatImportPreview = (sessionId: string): Promise<SourceImportPreviewResponse> =>
  getSourceImportPreview(MEERKAT_IMPORT_BASE, sessionId);

export const confirmMeerkatImport = (
  sessionId: string,
  actions: RowSourceActionInput[],
): Promise<void> => confirmSourceImport(MEERKAT_IMPORT_BASE, sessionId, actions);

export const cancelMeerkatImport = (sessionId: string): Promise<void> =>
  cancelSourceImport(MEERKAT_IMPORT_BASE, sessionId);
