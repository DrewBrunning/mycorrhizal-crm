// Per-subsystem last-known-good state API — issue #427, the frontend half of
// GET /admin/subsystem-health. Admin-only, read-only, instance-wide. The
// backend derives this on read by folding the operational-event stream
// (system_events, #424); there is nothing to write.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// The tracked subsystems, mirroring backend/services/subsystem_health.go's
// subsystemDefs and backend/openapi.yaml's SubsystemHealth.subsystem enum
// EXACTLY. No dynamic list endpoint exists — this is a hand-maintained mirror
// of the backend (CLAUDE.md frontend trap #4). Keep the three in sync, and in
// this order (the API preserves it).
export type Subsystem =
  | 'contact_sync'
  | 'calendar_sync'
  | 'notification'
  | 'backup'
  | 'scheduler'
  | 'webhook';

export const SUBSYSTEMS: Subsystem[] = [
  'contact_sync',
  'calendar_sync',
  'notification',
  'backup',
  'scheduler',
  'webhook',
];

// healthy = the last completed/failed event was a success; failing = it was a
// failure; unknown = the subsystem has produced no terminal event yet.
// scheduler and webhook emit only a failure event today, so they never reach
// `healthy` until a success-side event exists (#422).
export type SubsystemStatus = 'healthy' | 'failing' | 'unknown';

export interface SubsystemHealth {
  subsystem: Subsystem;
  status: SubsystemStatus;
  last_attempt_at: string | null;
  last_success_at: string | null;
  last_failure_at: string | null;
  // First failure of the current unbroken run — non-null exactly when
  // consecutive_failures > 0.
  incident_first_failure_at: string | null;
  consecutive_failures: number;
  // Sanitized error of the most recent failure; empty unless status is failing.
  last_error: string;
}

export interface SubsystemHealthResponse {
  subsystems: SubsystemHealth[];
}

// GET /admin/subsystem-health — one entry per tracked subsystem, in SUBSYSTEMS
// order.
export async function getSubsystemHealth(): Promise<SubsystemHealthResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/subsystem-health`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
