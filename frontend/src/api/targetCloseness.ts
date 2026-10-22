// Per-contact target closeness API calls (issue #1796/#1797, ADR-0023
// amendment). Mirrors backend/models/contact_target_closeness.go's
// ContactTargetClosenessResponse exactly -- see contactScore.ts for the
// sibling api-module pattern this follows.
import type { CadencePolicyInput } from './cadencePolicies';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// MUST be kept in sync by hand with backend models.TargetClosenessTiers()
// (CLAUDE.md frontend trap #4).
export const TARGET_CLOSENESS_TIERS = ['immediate', 'close', 'casual', 'distant'] as const;
export type TargetClosenessTier = (typeof TARGET_CLOSENESS_TIERS)[number];

export interface ContactTargetClosenessResponse {
  contact_id: number;
  target_closeness: TargetClosenessTier | null;
  default_interval_days: number | null;
  cadence_policy_offer: CadencePolicyInput | null;
}

// PUT /contacts/:id/target-closeness -- set the target tier. The response
// offers a cadence policy when the contact has none yet.
export async function setTargetCloseness(
  contactId: number | string,
  tier: TargetClosenessTier,
): Promise<ContactTargetClosenessResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/target-closeness`, {
    method: 'PUT',
    headers: { ...getAuthHeaders(), 'Content-Type': 'application/json' },
    body: JSON.stringify({ tier }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /contacts/:id/target-closeness -- clear the target.
export async function clearTargetCloseness(
  contactId: number | string,
): Promise<ContactTargetClosenessResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/target-closeness`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
