// Gift API calls -- T20b (docs/fork-plan/tickets/28-T20b-gift-tracking.md,
// docs/fork-plan/91-envelope-data-model.md §91.11): "what did I give them last
// year?" against a contact. Covers the states that make the feature useful —
// an idea captured opportunistically (status defaults to idea), the
// offered/given gift with its date, and a received gift (reciprocity /
// say-thanks tracking).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Gift status tokens — must stay in sync with backend/models/gift.go's
// GiftStatus* constants (this codebase's frontend registry copies are
// hand-maintained mirrors, by design).
export const GIFT_STATUSES = ['idea', 'purchased', 'given', 'received'] as const;
export type GiftStatus = (typeof GIFT_STATUSES)[number];

export interface Gift {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  status: GiftStatus;
  occasion?: string;
  description: string;
  // Optional link to the thing itself — a product page for a captured idea
  // (T35). Safe-scheme-validated server-side; still re-checked client-side
  // before it is used as an href (utils/linkResolution.ts).
  url?: string;
  // Optional free-text context beyond what the gift is (T35).
  notes?: string;
  // When the gift was given/received; undefined for a date-less idea.
  date?: string;
  // Optional value in the currency's minor unit, always paired with currency.
  value_cents?: number;
  // Explicit ISO 4217 currency whenever a value is set.
  currency?: string;
  // Optional links to the LifeEvent or Activity this gift relates to.
  life_event_id?: string;
  activity_id?: number;
  // Change-feed tombstone marker (T17), only true via ?since=.
  deleted?: boolean;
}

export interface GiftInput {
  entity_id: string;
  status?: GiftStatus;
  occasion?: string;
  description: string;
  url?: string;
  notes?: string;
  date?: string | null;
  value_cents?: number;
  currency?: string;
  life_event_id?: string;
  activity_id?: number | null;
}

export interface GiftListResponse {
  gifts: Gift[];
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

// GET /gifts
export async function getGifts(params?: {
  entityId?: string;
  cursor?: string;
  limit?: number;
}): Promise<GiftListResponse> {
  const { entityId, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entityId) queryParams.append('entity_id', entityId);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(
    `${API_BASE_URL}/gifts?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /gifts — create is wrapped in {gift: ...}
export async function createGift(input: GiftInput): Promise<Gift> {
  const response = await apiFetch(`${API_BASE_URL}/gifts`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.gift;
}

// PUT /gifts/:id (full-replace semantics for the editable fields)
export async function updateGift(id: string, input: GiftInput): Promise<Gift> {
  const response = await apiFetch(`${API_BASE_URL}/gifts/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /gifts/:id (soft delete)
export async function deleteGift(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/gifts/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
