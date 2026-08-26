// Audit event API calls -- T60,
// the frontend half of T18's already-shipped backend (GET /audit, POST
// /audit/:id/undo).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import { downloadFileFromResponse } from './export';

// Mirrors backend/models/audit.go's AuditEntity* tokens and backend/openapi.yaml's
// AuditEvent.entity_type enum exactly. No dynamic type-list endpoint exists
// anywhere in this codebase -- every enum is a hardcoded frontend mirror of the
// backend's validators (CLAUDE.md frontend trap #4). MUST be kept in sync by
// hand if the backend ever adds an audited entity.
export type AuditEntityType =
  | 'contact'
  | 'note'
  | 'activity'
  | 'life_event'
  | 'gift'
  | 'circle'
  | 'tag'
  | 'household'
  | 'reminder'
  // Auth/admin lifecycle entities (issue #381).
  | 'user'
  | 'auth'
  | 'api_token';

export const AUDIT_ENTITY_TYPES: AuditEntityType[] = [
  'contact',
  'note',
  'activity',
  'life_event',
  'gift',
  'circle',
  'tag',
  'household',
  'reminder',
  'user',
  'auth',
  'api_token',
];

// Mirrors models/audit.go's AuditOp* tokens (issue #381 widened the set from
// entity CRUD to include the auth/admin lifecycle vocabulary; issue #411
// added password_reset_requested).
export type AuditOperation =
  | 'create'
  | 'update'
  | 'delete'
  | 'login'
  | 'login_failed'
  | 'register'
  | 'password_change'
  | 'password_reset'
  | 'password_reset_requested'
  | 'totp_enable'
  | 'totp_disable'
  | 'recovery_regenerate'
  | 'revoke'
  | 'role_change';

export interface AuditEvent {
  id: number;
  created_at: string;
  entity_type: AuditEntityType;
  entity_id: string;
  operation: AuditOperation;
  // Redacted JSON of the pre-update/pre-delete state (empty for creates).
  // Opaque infrastructure -- the backend strips credential fields at recording
  // time, and the UI must never render it as user-facing content.
  before_snapshot?: string;
  // SHA-256 hash-chain link (issue #381): each row commits
  // hash(prev_hash || content) so the audit log is tamper-evident.
  hash?: string;
  prev_hash?: string;
}

export interface AuditEventsResponse {
  audit_events: AuditEvent[];
  total: number;
}

export interface GetAuditEventsParams {
  entity_type?: AuditEntityType;
  entity_id?: string;
  limit?: number;
}

// GET /audit -- the caller's immutable events, newest first. The API does all
// the IDOR gating (every row is scoped to the session user server-side).
export async function getAuditEvents(
  params: GetAuditEventsParams = {},
): Promise<AuditEventsResponse> {
  const { entity_type, entity_id, limit = 100 } = params;
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entity_type) queryParams.append('entity_type', entity_type);
  if (entity_id) queryParams.append('entity_id', entity_id);

  const response = await apiFetch(`${API_BASE_URL}/audit?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /audit/:id/undo -- reverts an update event from its before snapshot.
// Contact-only today; the backend returns 400 for any other entity or a delete
// event, and 410 once the event has aged past AUDIT_RETENTION_DAYS.
export async function undoAuditEvent(id: number): Promise<{ message: string }> {
  const response = await apiFetch(`${API_BASE_URL}/audit/${id}/undo`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// GET /audit/export -- issue #416. Unbounded CSV export of the caller's own
// audit trail (unlike getAuditEvents, which caps at 500 rows for the list
// view). before_snapshot is omitted unless includeSnapshots is explicitly
// set: it is credential-redacted but not contact-field-sensitivity-filtered,
// so it is gated behind its own opt-in rather than reusing a sensitivity flag
// that means something narrower everywhere else in this app.
export async function exportAuditLog(includeSnapshots = false): Promise<void> {
  const params = new URLSearchParams();
  if (includeSnapshots) {
    params.set('include_snapshots', 'true');
  }
  const response = await apiFetch(`${API_BASE_URL}/audit/export?${params.toString()}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  await downloadFileFromResponse(response, 'mycorrhizal-audit-log.csv');
}
