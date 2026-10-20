// Contact relationship health score API calls (issue #383, ADR-0023).
// Mirrors backend/models/contact_score.go's ContactScoreResponse exactly --
// see relationshipEdges.ts for the sibling api-module pattern this follows.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// Issue #1733: why a contact counts as in regular contact without logged
// activity. MUST stay in sync with backend/internal/scoring's
// AmbientReason* constants.
export type AmbientReason = 'household' | 'flag';

// Mirrors backend/models/contact_score.go's ContactScoreFacet.
export interface ContactScoreFacet {
  value: number;
  weight: number;
  reason: string;
}

// Mirrors backend/models/contact_score.go's ContactScoreResponse. `band` is
// loosely typed (string, not a union) defensively, matching this codebase's
// convention elsewhere (e.g. RelationshipEdge.type) for a value this frontend
// doesn't itself validate -- see healthBand.ts for the moss/chanterelle/
// russula mapping consumers actually switch on.
export interface ContactScoreResponse {
  contact_id: number;
  score: number;
  band: string;
  recency: ContactScoreFacet;
  frequency: ContactScoreFacet;
  closeness: ContactScoreFacet;
  reach_out: ContactScoreFacet;
  last_updated: ContactScoreFacet;
  // Issue #1733: true when Recency and Frequency were forced to 100 because the
  // contact lives with the user (reason 'household') or is flagged ('flag').
  // The server always sends ambient; it is optional here only because the
  // spec's ContactScoreResponse declares no required fields (treat absent as
  // false). ambient_reason is absent when ambient is false.
  ambient?: boolean;
  ambient_reason?: AmbientReason;
}

// GET /contacts/:id/score -- the explainable relationship health score for a
// single contact.
export async function getContactScore(contactId: number | string): Promise<ContactScoreResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/score`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
