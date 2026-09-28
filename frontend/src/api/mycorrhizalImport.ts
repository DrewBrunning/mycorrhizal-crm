// Mycorrhizal account-bundle import source — the source-specific half (upload
// + fetch). The review/progress/result contract is shared: see sourceImport.ts.
// Types mirror backend/models/mycorrhizal_import.go by hand (CLAUDE.md frontend
// trap #4 — no dynamic type-list endpoint).
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

export const MYCORRHIZAL_IMPORT_BASE = '/import/mycorrhizal';

export interface MycorrhizalBundleCounts {
  contacts: number;
  relationships: number;
  notes: number;
  reminders: number;
  reminder_completions: number;
  activities: number;
  life_events: number;
  gifts: number;
  preferences: number;
  conversation_agenda: number;
  cadence_policies: number;
  data_decay_policies: number;
  households: number;
  circles: number;
  tags: number;
  custom_field_definitions: number;
  custom_field_values: number;
  occasions: number;
  occasion_events: number;
}

export interface MycorrhizalUploadResponse {
  session_id: string;
  version: number;
  totals: MycorrhizalBundleCounts;
}

// Uploads a Mycorrhizal account bundle (produced by GET /export/account). The
// server validates the format/version, parses it in memory, and returns the
// per-section totals. A wrong version is rejected with 422.
export async function uploadMycorrhizalBundle(file: File): Promise<MycorrhizalUploadResponse> {
  const form = new FormData();
  form.append('file', file);
  const response = await apiFetch(`${API_BASE_URL}${MYCORRHIZAL_IMPORT_BASE}/upload`, {
    method: 'POST',
    body: form,
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function startMycorrhizalFetch(sessionId: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}${MYCORRHIZAL_IMPORT_BASE}/fetch`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ session_id: sessionId }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export const getMycorrhizalImportStatus = (sessionId: string): Promise<SourceImportStatus> =>
  getSourceImportStatus(MYCORRHIZAL_IMPORT_BASE, sessionId);

export const getMycorrhizalImportPreview = (
  sessionId: string,
): Promise<SourceImportPreviewResponse> =>
  getSourceImportPreview(MYCORRHIZAL_IMPORT_BASE, sessionId);

export const confirmMycorrhizalImport = (
  sessionId: string,
  actions: RowSourceActionInput[],
): Promise<void> => confirmSourceImport(MYCORRHIZAL_IMPORT_BASE, sessionId, actions);

export const cancelMycorrhizalImport = (sessionId: string): Promise<void> =>
  cancelSourceImport(MYCORRHIZAL_IMPORT_BASE, sessionId);
