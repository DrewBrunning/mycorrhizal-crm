// Contact relationship health score API calls (issue #383, ADR-0023).
// Mirrors backend/models/contact_score.go's ContactScoreResponse exactly --
// see relationshipEdges.ts for the sibling api-module pattern this follows.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

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
