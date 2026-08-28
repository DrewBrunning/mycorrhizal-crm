// Instance diagnostics API — issue #423, the frontend half of
// GET /admin/diagnostics. Admin-only, read-only, instance-wide: a manual
// "is this install healthy?" sweep the operator runs after an install, an
// upgrade, a migration, or a config change. The backend folds its existing
// single-check paths into one ok/warning/error checklist with a summary, and
// never returns a secret (config values are not echoed; integration base URLs
// and transport errors go to the log only).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export type DiagnosticStatus = 'ok' | 'warning' | 'error';

export interface DiagnosticCheck {
  // Stable snake_case identifier — e.g. config, database, migrations,
  // filesystem, backup, notification_<channel>, integration_<system>,
  // disk_space, background_jobs, version.
  name: string;
  status: DiagnosticStatus;
  message: string;
}

export interface DiagnosticSummary {
  // error dominates warning dominates ok.
  status: DiagnosticStatus;
  ok: number;
  warnings: number;
  errors: number;
}

export interface DiagnosticsResponse {
  timestamp: string;
  summary: DiagnosticSummary;
  checks: DiagnosticCheck[];
}

// GET /admin/diagnostics — always HTTP 200 for an admin; a broken install is
// reported in the checklist, not as a non-2xx.
export async function runDiagnostics(): Promise<DiagnosticsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/diagnostics`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
