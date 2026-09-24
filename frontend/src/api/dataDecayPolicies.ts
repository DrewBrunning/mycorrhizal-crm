// DataDecayPolicy API calls -- issue #352, docs/adrs/0026-data-decay.md,
// opt-in per-contact rules to periodically re-verify stored info is still
// accurate, with DERIVED health (never stored) except last_verified_at
// itself -- see DataDecayPolicy.LastVerifiedAt's backend doc comment for why
// that one field is stored rather than derived.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface DataDecayHealth {
  next_due: string;
  // Whole calendar days past due. 0 when due today, in the future, or
  // undefined. "Due today" is not overdue.
  overdue_by: number;
}

export interface DataDecayPolicy {
  id: string;
  entity_id: string; // Contact.VCardUID
  interval_days: number;
  last_verified_at?: string | null;
  active: boolean;
  created_at: string;
  updated_at: string;
  health?: DataDecayHealth;
}

export interface DataDecayPolicyInput {
  entity_id: string;
  interval_days: number;
  active?: boolean;
}

export interface OverdueDataDecayPolicy {
  policy: DataDecayPolicy;
  health: DataDecayHealth;
  contact_id: number;
  contact_name: string;
  photo_thumbnail?: string;
}

export interface DataDecayPoliciesResponse {
  data_decay_policies: DataDecayPolicy[];
  total: number;
  next_cursor: string;
  limit: number;
}

export interface OverdueDataDecayPoliciesResponse {
  overdue: OverdueDataDecayPolicy[];
}

export async function getDataDecayPolicies(entityId: string): Promise<DataDecayPoliciesResponse> {
  const queryParams = new URLSearchParams({ entity_id: entityId });
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getOverdueDataDecayPolicies(): Promise<OverdueDataDecayPoliciesResponse> {
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies/overdue`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// NOTE the response shape asymmetry: create is wrapped in
// {message, data_decay_policy: ...}, update/verify return the policy raw --
// same convention as cadence policies.
export async function createDataDecayPolicy(input: DataDecayPolicyInput): Promise<DataDecayPolicy> {
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.data_decay_policy;
}

export async function updateDataDecayPolicy(
  id: string,
  input: DataDecayPolicyInput,
): Promise<DataDecayPolicy> {
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json(); // raw policy, NOT wrapped -- unlike create
}

export async function deleteDataDecayPolicy(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// The "confirm still current" action -- stamps last_verified_at = now,
// resetting the derived health's baseline.
export async function verifyDataDecayPolicy(id: string): Promise<DataDecayPolicy> {
  const response = await apiFetch(`${API_BASE_URL}/data-decay-policies/${id}/verify`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json(); // raw policy, NOT wrapped
}
