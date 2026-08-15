// Household API calls -- T1.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { RelationshipEdge } from './relationshipEdges';

// Mirrors backend/models/household.go's HouseholdType* constants and the
// `oneof=family_unit roommates other` validator on Household.Type. No dynamic
// type-list endpoint exists anywhere in this codebase -- this is a hardcoded
// mirror kept in sync by hand (same convention as every other enum here).
export type HouseholdType = 'family_unit' | 'roommates' | 'other';

export const HOUSEHOLD_TYPES: HouseholdType[] = ['family_unit', 'roommates', 'other'];

// Conventional (not enforced) role tokens, mirroring backend/models/
// household.go's HouseholdRole* constants.
export const HOUSEHOLD_ROLES = ['adult', 'child', 'pet', 'roommate'] as const;
export type HouseholdRole = (typeof HOUSEHOLD_ROLES)[number];

export interface Household {
  id: string;
  created_at: string;
  updated_at: string;
  name: string;
  type: HouseholdType;
}

export interface HouseholdMember {
  id: number;
  household_id: string;
  member_vcard_uid: string;
  role?: string;
  since?: string;
  until?: string;
}

export interface HouseholdWithMembers {
  household: Household;
  members: HouseholdMember[];
}

export interface HouseholdListResponse {
  households: Household[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
  members?: HouseholdMember[];
}

export interface HouseholdInput {
  name: string;
  type: HouseholdType;
}

export interface HouseholdMemberInput {
  member_vcard_uid: string;
  role?: string;
  since?: string;
  until?: string;
}

export interface SuggestRelationshipsResponse {
  message: string;
  household_id: string;
  suggested_edges: RelationshipEdge[];
  total: number;
}

// GET /households
export async function listHouseholds(params?: {
  cursor?: string;
  limit?: number;
  include_members?: boolean;
}): Promise<HouseholdListResponse> {
  const { cursor, limit = 100, include_members = false } = params || {};
  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });
  if (cursor) queryParams.append('cursor', cursor);
  if (include_members) queryParams.append('include_members', 'true');
  const response = await apiFetch(
    `${API_BASE_URL}/households?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /households
export async function createHousehold(input: HouseholdInput): Promise<Household> {
  const response = await apiFetch(`${API_BASE_URL}/households`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.household;
}

// PUT /households/:id
export async function updateHousehold(id: string, input: HouseholdInput): Promise<Household> {
  const response = await apiFetch(`${API_BASE_URL}/households/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /households/:id
export async function deleteHousehold(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/households/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// POST /households/:id/members
export async function addHouseholdMember(
  householdId: string,
  input: HouseholdMemberInput
): Promise<HouseholdMember> {
  const response = await apiFetch(`${API_BASE_URL}/households/${householdId}/members`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.member;
}

// DELETE /households/:id/members/:vcard_uid
export async function removeHouseholdMember(
  householdId: string,
  memberVCardUid: string
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/households/${householdId}/members/${memberVCardUid}`,
    { method: 'DELETE', headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
}

// PATCH /households/:id/members/:vcard_uid — update a member's role in-place (T1 review B3+B4).
export async function updateHouseholdMember(
  householdId: string,
  memberVCardUid: string,
  role: string
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/households/${householdId}/members/${memberVCardUid}`,
    { method: 'PATCH', headers: getAuthHeaders(), body: JSON.stringify({ role }) }
  );
  if (!response.ok) throw await parseErrorResponse(response);
}

// POST /households/:id/suggest-relationships -- the trigger that produces
// suggested RelationshipEdges for every applicable member pair. Idempotent:
// re-running never duplicates edges (services.GenerateHouseholdSuggestions).
export async function suggestHouseholdRelationships(
  householdId: string
): Promise<SuggestRelationshipsResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/households/${householdId}/suggest-relationships`,
    { method: 'POST', headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// ---------------------------------------------------------------------------
// T40 address-based household suggestions (T40).
// ---------------------------------------------------------------------------

// A neutral AddressComponent pair, mirroring contactmodel.AddressComponent's
// wire shape on the suggestion's address.
export interface AddressComponent {
  kind: string;
  value: string;
}

export interface AddressHouseholdSuggestion {
  address_hash: string;
  member_hash: string;
  member_vcard_uids: string[];
  address: {
    components: AddressComponent[];
    full?: string;
  };
}

export interface SuggestAddressHouseholdsResponse {
  suggestions: AddressHouseholdSuggestion[];
  total: number;
}

// Renders a suggestion's address as a single display line (street, locality,
// region, postcode, country), falling back to the full text when present.
// The T79 sub-street parts (PO box / apartment / floor) are deliberately NOT
// rendered here: a suggestion's address is a *building*-level shared address
// (matching backend AddressNormalizedKey's scope, which also excludes them),
// so showing one member's apartment would mislead. Backend FormatAddress
// includes them on individual contact display; this is the household surface.
export function formatSuggestionAddress(address: AddressHouseholdSuggestion['address']): string {
  if (address.full) return address.full;
  const byKind: Record<string, string> = {};
  for (const comp of address.components) {
    if (!(comp.kind in byKind)) byKind[comp.kind] = comp.value;
  }
  const parts = ['name', 'locality', 'region', 'postcode', 'country']
    .map((kind) => byKind[kind])
    .filter((v) => v && v.trim());
  return parts.join(', ');
}

// POST /households/suggest-addresses -- T40 detection trigger. Read-only and
// idempotent.
export async function suggestAddressHouseholds(): Promise<SuggestAddressHouseholdsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/households/suggest-addresses`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /households/suggestions/accept -- create the Household + member rows
// for a suggested group. The server re-validates the group from the member
// VCardUIDs.
export async function acceptAddressHouseholdSuggestion(
  memberVCardUids: string[],
  input?: { name?: string; type?: HouseholdType }
): Promise<Household> {
  const response = await apiFetch(`${API_BASE_URL}/households/suggestions/accept`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ member_vcard_uids: memberVCardUids, ...input }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.household;
}

// POST /households/suggestions/dismiss -- permanently dismiss a suggested
// group so the scan stops offering it.
export async function dismissAddressHouseholdSuggestion(
  memberVCardUids: string[]
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/households/suggestions/dismiss`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ member_vcard_uids: memberVCardUids }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
