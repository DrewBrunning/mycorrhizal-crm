// System event (operational-event timeline) API — issue #424, the frontend
// half of GET /admin/system-events. Admin-only, read-only, instance-wide.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// Mirrors backend/models/system_event.go's SysEvent* tokens, migration
// 000038's CHECK constraint, and backend/openapi.yaml's SystemEvent.event_type
// enum EXACTLY. No dynamic type-list endpoint exists anywhere in this codebase
// — every enum is a hand-maintained mirror of the backend validators
// (CLAUDE.md frontend trap #4). Keep all four in sync.
export type SystemEventType =
  | 'application_started'
  | 'application_stopped'
  | 'migration_started'
  | 'migration_completed'
  | 'migration_failed'
  | 'job_started'
  | 'job_completed'
  | 'job_failed'
  | 'sync_started'
  | 'sync_completed'
  | 'sync_failed'
  | 'notification_sent'
  | 'notification_failed'
  | 'backup_completed'
  | 'backup_failed'
  | 'restore_test_completed'
  | 'integration_failed';

export const SYSTEM_EVENT_TYPES: SystemEventType[] = [
  'application_started',
  'application_stopped',
  'migration_started',
  'migration_completed',
  'migration_failed',
  'job_started',
  'job_completed',
  'job_failed',
  'sync_started',
  'sync_completed',
  'sync_failed',
  'notification_sent',
  'notification_failed',
  'backup_completed',
  'backup_failed',
  'restore_test_completed',
  'integration_failed',
];

export type SystemEventSeverity = 'info' | 'warn' | 'error';
export const SYSTEM_EVENT_SEVERITIES: SystemEventSeverity[] = ['info', 'warn', 'error'];

// The component values the backend producers emit today (logger.Component* in
// backend/logger/fields.go). `component` is a free-form string server-side, so
// this list is only the filter dropdown's suggestions — an unknown value from
// a future producer still renders fine in the table.
export const SYSTEM_EVENT_COMPONENTS: string[] = [
  'app',
  'scheduler',
  'migration',
  'contact_sync',
  'calendar_sync',
  'notification',
  'webhook',
  'backup',
];

export type SystemEventResult = 'success' | 'failure' | 'skipped';

export interface SystemEvent {
  id: number;
  created_at: string;
  occurred_at: string;
  event_type: SystemEventType;
  severity: SystemEventSeverity;
  component: string;
  operation?: string;
  duration_ms?: number;
  result?: SystemEventResult;
  correlation_id: string;
  error?: string;
  detail?: string;
  user_id?: number;
}

export interface SystemEventsResponse {
  system_events: SystemEvent[];
  total: number;
}

export interface GetSystemEventsParams {
  component?: string;
  severity?: SystemEventSeverity | '';
  event_type?: SystemEventType | '';
  correlation_id?: string;
  since?: string;
  until?: string;
  limit?: number;
}

// GET /admin/system-events — reverse-chronological by occurrence. Filter by
// correlation_id to pull every event in one chain of work (the "view related"
// drill-down). All filtering is server-side.
export async function getSystemEvents(
  params: GetSystemEventsParams = {},
): Promise<SystemEventsResponse> {
  const { limit = 100 } = params;
  const q = new URLSearchParams({ limit: limit.toString() });
  if (params.component) q.append('component', params.component);
  if (params.severity) q.append('severity', params.severity);
  if (params.event_type) q.append('event_type', params.event_type);
  if (params.correlation_id) q.append('correlation_id', params.correlation_id);
  if (params.since) q.append('since', params.since);
  if (params.until) q.append('until', params.until);

  const response = await apiFetch(`${API_BASE_URL}/admin/system-events?${q.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
