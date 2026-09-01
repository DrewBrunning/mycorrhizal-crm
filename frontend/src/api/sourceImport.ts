// Shared client for the "source import" assistants — a live third-party
// system (Monica, issue #549) or an uploaded database (Meerkat, issue #550)
// pulled into a background session, reviewed with a loss report, then
// confirmed through the same staged preview→confirm contract as the file
// imports. Everything here is source-agnostic and parameterised by a
// `basePath`; the Monica-specific connect/fetch calls live in monicaImport.ts.
//
// Phase and category vocabularies mirror backend/models/monica_import.go and
// backend/services/import_source.go by hand — there is no dynamic type-list
// endpoint (CLAUDE.md frontend trap #4). Keep them in sync.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { ImportRowPreview } from './import';

export type SourceImportPhase =
  | 'connecting'
  | 'parsing_database'
  | 'mapping'
  | 'fetching_contacts'
  | 'fetching_activities'
  | 'fetching_notes'
  | 'fetching_reminders'
  | 'fetching_extras'
  | 'fetching_relationships'
  | 'building_preview'
  | 'ready'
  | 'importing'
  | 'importing_photos'
  | 'done'
  | 'failed'
  | 'cancelled';

// The review step's per-contact decision. Mirrors backend RowImportAction's
// `oneof=skip add update`.
export type RowSourceAction = 'add' | 'skip' | 'update';

export type SourceImportIssueCategory =
  | 'unsupported'
  | 'lossy'
  | 'transformed'
  | 'invalid'
  | 'skipped';

// One entry of the pre-confirm loss report (the issue #442 record/field/
// category shape from services.ImportIssue).
export interface SourceImportIssue {
  record: string;
  field: string;
  category: SourceImportIssueCategory;
  message: string;
}

// Per-contact tally of graph entities the import will create for that row.
export interface SourceRelatedCounts {
  activities: number;
  notes: number;
  reminders: number;
  relationships: number;
  gifts: number;
}

export interface SourceImportRowPreview extends ImportRowPreview {
  related: SourceRelatedCounts;
  has_photo: boolean;
}

export interface SourceImportResult {
  total_processed: number;
  created: number;
  updated: number;
  skipped: number;
  errors: string[];
  relationships_created: number;
  notes_created: number;
  activities_created: number;
  reminders_created: number;
  gifts_created: number;
  custom_fields_created: number;
  photos_queued: number;
  photos_saved: number;
  photos_failed: number;
}

export interface SourceImportStatus {
  session_id: string;
  phase: SourceImportPhase;
  phase_done: number;
  phase_total: number;
  error?: string;
  result?: SourceImportResult;
}

export interface SourceImportPreviewResponse {
  session_id: string;
  rows: SourceImportRowPreview[];
  total_rows: number;
  valid_rows: number;
  duplicate_count: number;
  error_count: number;
  totals: SourceRelatedCounts;
  loss_report: SourceImportIssue[];
}

export interface RowSourceActionInput {
  row_index: number;
  action: RowSourceAction;
}

async function getJSON<T>(path: string): Promise<T> {
  const response = await apiFetch(`${API_BASE_URL}${path}`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export function getSourceImportStatus(
  basePath: string,
  sessionId: string,
): Promise<SourceImportStatus> {
  return getJSON(`${basePath}/status?session_id=${encodeURIComponent(sessionId)}`);
}

export function getSourceImportPreview(
  basePath: string,
  sessionId: string,
): Promise<SourceImportPreviewResponse> {
  return getJSON(`${basePath}/preview?session_id=${encodeURIComponent(sessionId)}`);
}

// confirmSourceImport starts the import in the background — the endpoint
// replies 202 with no result body. The caller polls getSourceImportStatus
// until phase "done" and reads the summary from status.result.
export async function confirmSourceImport(
  basePath: string,
  sessionId: string,
  actions: RowSourceActionInput[],
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}${basePath}/confirm`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ session_id: sessionId, actions }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function cancelSourceImport(basePath: string, sessionId: string): Promise<void> {
  // Best-effort: a failed cancel just leaves the session to expire on its own.
  await apiFetch(`${API_BASE_URL}${basePath}/cancel?session_id=${encodeURIComponent(sessionId)}`, {
    method: 'POST',
    headers: getAuthHeaders(),
  }).catch(() => undefined);
}
