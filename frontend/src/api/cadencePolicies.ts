// CadencePolicy API calls -- T19,
// relationship-maintenance rules ("stay in touch every N days") with
// DERIVED health (never stored).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Interaction types that can be opted into a cadence -- the pick-list for a
// policy's qualifying_types. MUST be kept in sync by hand with
// backend/models/activity.go's InteractionType* constants (this repo has no
// dynamic type-list endpoint; every enum is a hardcoded frontend mirror).
//
// Deliberately EXCLUDES 'photo': it is the one globally non-qualifying type
// (backend Activity.Qualifying()), so offering it as a checkbox would be dead
// weight -- it can never reset a cadence, no matter how it is listed. If the
// backend ever marks another type non-qualifying, drop it here too.
export const QUALIFYING_INTERACTION_TYPE_TOKENS = [
  'call',
  'video_call',
  'visit',
  'meal',
  'gift',
  'message',
  'shared_activity',
] as const;
export type InteractionType = (typeof QUALIFYING_INTERACTION_TYPE_TOKENS)[number];

// Derived relationship health  -- computed from the timeline, never
// persisted. `has_qualifying_interaction` stays false until the contact has
// had at least one qualifying interaction (cadence does NOT count from the
// contact's creation date).
export interface CadenceHealth {
  has_qualifying_interaction: boolean;
  last_interaction?: string | null;
  next_due?: string | null;
  // Whole calendar days past due. 0 when due today, in the future, or
  // undefined. "Due today" is not overdue.
  overdue_by: number;
}

export interface CadencePolicy {
  id: string;
  entity_id: string; // Contact.VCardUID
  target_interval_days: number;
  qualifying_types: string[]; // empty = all default-qualifying types count
  created_at: string;
  updated_at: string;
  health?: CadenceHealth;
}

export interface CadencePolicyInput {
  entity_id: string;
  target_interval_days: number;
  qualifying_types?: string[];
}

export interface OverdueCadence {
  policy: CadencePolicy;
  health: CadenceHealth;
  contact_id: number;
  contact_name: string;
  photo_thumbnail?: string;
}

export interface CadencePoliciesResponse {
  cadence_policies: CadencePolicy[];
  total: number;
  next_cursor: string;
  limit: number;
}

export interface OverdueCadencesResponse {
  overdue: OverdueCadence[];
}

export async function getCadencePolicies(entityId: string): Promise<CadencePoliciesResponse> {
  const queryParams = new URLSearchParams({ entity_id: entityId });
  const response = await apiFetch(`${API_BASE_URL}/cadence-policies?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getOverdueCadences(): Promise<OverdueCadencesResponse> {
  const response = await apiFetch(`${API_BASE_URL}/cadence-policies/overdue`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// NOTE the response shape asymmetry: create is wrapped in
// {message, cadence_policy: ...}, update returns the policy raw.
export async function createCadencePolicy(input: CadencePolicyInput): Promise<CadencePolicy> {
  const response = await apiFetch(`${API_BASE_URL}/cadence-policies`, {
    method: 'POST', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.cadence_policy;
}

export async function updateCadencePolicy(id: string, input: CadencePolicyInput): Promise<CadencePolicy> {
  const response = await apiFetch(`${API_BASE_URL}/cadence-policies/${id}`, {
    method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json(); // raw policy, NOT wrapped -- unlike create
}

export async function deleteCadencePolicy(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/cadence-policies/${id}`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
