// Audit event API calls -- T60 (docs/fork-plan/tickets/79-T60-audit-trail-ui.md),
// the frontend half of T18's already-shipped backend (GET /audit, POST
// /audit/:id/undo).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

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
  | 'reminder';

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
];

// Mirrors models/audit.go's AuditOp* tokens.
export type AuditOperation = 'create' | 'update' | 'delete';

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
export async function getAuditEvents(params: GetAuditEventsParams = {}): Promise<AuditEventsResponse> {
  const { entity_type, entity_id, limit = 100 } = params;
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entity_type) queryParams.append('entity_type', entity_type);
  if (entity_id) queryParams.append('entity_id', entity_id);

  const response = await apiFetch(`${API_BASE_URL}/audit?${queryParams.toString()}`, { headers: getAuthHeaders() });
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
