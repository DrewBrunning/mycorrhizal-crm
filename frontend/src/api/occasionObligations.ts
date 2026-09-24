// OccasionObligation API calls (ADR 0024, issue #387): "this contact is on
// my holiday card list every year" — a standing recurring obligation, the
// layer above LifeEvent/Gift/Preference.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import { downloadFileFromResponse } from './export';

// OccasionObligation.Kind is an open classifier (backend/models/occasion_obligation.go)
// — card/gift/invite are the three named ones; the value is free text
// server-side, this frontend mirror just offers the three as suggestions.
export const OCCASION_OBLIGATION_KINDS = ['card', 'gift', 'invite'] as const;
export type OccasionObligationKind = (typeof OCCASION_OBLIGATION_KINDS)[number];

// Mirrors the shared normal/private/secret set (RelationshipEdgeSensitivity).
export type OccasionSensitivity = 'normal' | 'private' | 'secret';

export interface OccasionObligation {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  kind: string;
  label: string;
  anchor_month?: number;
  anchor_day?: number;
  linked_life_event_id?: string;
  lead_time_days: number;
  active: boolean;
  sensitivity: OccasionSensitivity;
  notes?: string;
  // Change-feed tombstone marker (T17), only true via ?since=.
  deleted?: boolean;
}

export interface OccasionObligationInput {
  entity_id: string;
  kind: string;
  label: string;
  anchor_month?: number | null;
  anchor_day?: number | null;
  linked_life_event_id?: string;
  lead_time_days?: number;
  active?: boolean;
  sensitivity?: OccasionSensitivity;
  notes?: string;
}

export interface OccasionObligationsResponse {
  occasion_obligations: OccasionObligation[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

// GET /occasion-obligations
export async function getOccasionObligations(params?: {
  entityId?: string;
  cursor?: string;
  limit?: number;
}): Promise<OccasionObligationsResponse> {
  const { entityId, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entityId) queryParams.append('entity_id', entityId);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-obligations?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /occasion-obligations — create is wrapped in {occasion_obligation: ...}
export async function createOccasionObligation(
  input: OccasionObligationInput,
): Promise<OccasionObligation> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-obligations`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.occasion_obligation;
}

// PUT /occasion-obligations/:id (full-replace)
export async function updateOccasionObligation(
  id: string,
  input: OccasionObligationInput,
): Promise<OccasionObligation> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-obligations/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /occasion-obligations/:id (soft delete)
export async function deleteOccasionObligation(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-obligations/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// ---------------------------------------------------------------------------
// Read-only aggregates (ADR 0024 parts 3-5): the dashboard widget, the
// card-list CSV, and the gift shopping list.
// ---------------------------------------------------------------------------

export type UpcomingOccasionSource = 'birthday' | 'anniversary' | 'life_event' | 'obligation';

export interface UpcomingOccasion {
  contact_id: number;
  contact_name: string;
  source: UpcomingOccasionSource;
  label: string;
  date: string;
  days_until: number;
  kind?: string;
}

export interface UpcomingOccasionsResponse {
  occasions: UpcomingOccasion[];
  days: number;
}

// GET /occasions/upcoming
export async function getUpcomingOccasions(params?: {
  days?: 30 | 90;
  includeSensitive?: boolean;
}): Promise<UpcomingOccasionsResponse> {
  const { days = 30, includeSensitive = false } = params || {};
  const queryParams = new URLSearchParams({ days: String(days) });
  if (includeSensitive) queryParams.append('include_sensitive', 'true');
  const response = await apiFetch(`${API_BASE_URL}/occasions/upcoming?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export type GiftShoppingStatus = 'needed' | 'idea' | 'purchased' | 'given' | 'received';

export interface GiftShoppingItem {
  contact_id: number;
  contact_name: string;
  obligation_id: string;
  label: string;
  date: string;
  days_until: number;
  status: GiftShoppingStatus;
  linked_gift_id?: string;
}

export interface GiftShoppingListResponse {
  gift_shopping_list: GiftShoppingItem[];
  days: number;
}

// GET /occasion-obligations/gift-shopping-list
export async function getGiftShoppingList(params?: {
  days?: 30 | 90;
  includeSensitive?: boolean;
}): Promise<GiftShoppingListResponse> {
  const { days = 30, includeSensitive = false } = params || {};
  const queryParams = new URLSearchParams({ days: String(days) });
  if (includeSensitive) queryParams.append('include_sensitive', 'true');
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-obligations/gift-shopping-list?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// GET /occasion-obligations/card-list — triggers a browser download, reusing
// api/export.ts's downloadFileFromResponse rather than duplicating the
// blob/anchor-click mechanics (the same reuse api/audit.ts's exportAuditLog
// already does).
export async function downloadOccasionCardListCSV(params?: {
  kind?: string;
  includeSensitive?: boolean;
}): Promise<void> {
  const { kind = 'card', includeSensitive = false } = params || {};
  const queryParams = new URLSearchParams({ kind });
  if (includeSensitive) queryParams.append('include_sensitive', 'true');
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-obligations/card-list?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  await downloadFileFromResponse(response, `mycorrhizal-${kind}-list.csv`);
}
