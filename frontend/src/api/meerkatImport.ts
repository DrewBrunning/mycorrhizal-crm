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

export async function startMeerkatFetch(sessionId: string, sourceUserId?: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}${MEERKAT_IMPORT_BASE}/fetch`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      session_id: sessionId,
      source_user_id: sourceUserId ?? null,
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
