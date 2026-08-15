// ConversationAgenda API calls -- T21 (T21, docs/adrs/0001-neutral-hub-and-spoke-contact-model.md
// §91.11): "things to bring up next time I see them". Contextual memory for a
// contact, deliberately NOT date-scheduled — resolved by marking it discussed
// (discussConversationAgenda), never by a timer.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface ConversationAgenda {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  content: string;
  reference_url?: string;
  // When the item was marked discussed; undefined means still open.
  discussed_at?: string;
  // The Activity that covered this item, if linked at discuss time.
  activity_id?: number;
  // Change-feed tombstone marker (T17), only true via ?since=.
  deleted?: boolean;
}

export interface ConversationAgendaInput {
  entity_id: string;
  content: string;
  reference_url?: string;
}

export interface ConversationAgendaListResponse {
  conversation_agenda: ConversationAgenda[];
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

// GET /conversation-agenda
export async function getConversationAgenda(params?: {
  entityId?: string;
  cursor?: string;
  limit?: number;
}): Promise<ConversationAgendaListResponse> {
  const { entityId, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entityId) queryParams.append('entity_id', entityId);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(
    `${API_BASE_URL}/conversation-agenda?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /conversation-agenda — create is wrapped in {conversation_agenda: ...}
export async function createConversationAgenda(input: ConversationAgendaInput): Promise<ConversationAgenda> {
  const response = await apiFetch(`${API_BASE_URL}/conversation-agenda`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.conversation_agenda;
}

// PUT /conversation-agenda/:id (full-replace of content fields; the resolved
// discussed_at/activity_id state is owned by discussConversationAgenda and is
// never touched here).
export async function updateConversationAgenda(id: string, input: ConversationAgendaInput): Promise<ConversationAgenda> {
  const response = await apiFetch(`${API_BASE_URL}/conversation-agenda/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// PATCH /conversation-agenda/:id/discuss — the agenda's only resolution.
// activityId is optional: pass an Activity ID to link the interaction that
// covered the item (feeding the timeline); omit to mark discussed without a
// link.
export async function discussConversationAgenda(id: string, activityId?: number): Promise<ConversationAgenda> {
  const response = await apiFetch(`${API_BASE_URL}/conversation-agenda/${id}/discuss`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify(activityId !== undefined ? { activity_id: activityId } : {}),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /conversation-agenda/:id (soft delete)
export async function deleteConversationAgenda(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/conversation-agenda/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
