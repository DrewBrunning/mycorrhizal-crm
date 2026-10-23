// API client for network graph data

import type { GraphResponse } from '../types/graph';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface GetGraphParams {
  // true asks for every non-archived contact (the server's default, kept for
  // pre-v1.5 clients); false asks for the depth-2 ego-network of the caller's
  // self contact (issue #1791), which the web UI uses by default.
  full?: boolean;
}

export async function getGraph(params?: GetGraphParams): Promise<GraphResponse> {
  // Always state the view: an absent `full` is the server's backward-
  // compatible full graph, not the ego view the web wants.
  const query = params?.full ? '?full=true' : '?full=false';
  const response = await apiFetch(`${API_BASE_URL}/graph${query}`, { headers: getAuthHeaders() });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// --- Graph traversal / multi-hop chains (T10) ---

export interface GraphChainStep {
  contact_id: number;
  contact_vcard_uid: string;
  contact_name: string;
  // Display relation token: what this step's contact IS to the previous
  // contact ("sister_of", "spouse_of"). The inverse is already applied when
  // the hop walked against the edge's stored direction.
  relation: string;
}

export interface GraphChain {
  target_id: number;
  target_vcard_uid: string;
  target_name: string;
  depth: number;
  steps: GraphChainStep[];
  // The target contact's relationship health score (issue #383, ADR-0023).
  // This is the only health-score-bearing field the Android ego-network
  // screen ever sees (it never calls GET /graph, only GET
  // /graph/connections), so this mirrors GraphNode's fields exactly -- see
  // backend/models/dtos.go's GraphChain doc comment.
  health_score?: number | null;
  health_band?: string;
  // Issue #1193: true when the target contact records a death anniversary
  // (Card.Anniversaries[kind=death]). Mirrors GraphNode's field for the same
  // "Android's only surface for this" reason as health_score/health_band
  // above.
  deceased?: boolean;
}

export interface GraphConnectionsResponse {
  from_vcard_uid: string;
  from_name: string;
  depth: number;
  chains: GraphChain[];
}

export interface GetConnectionsParams {
  from: string; // Contact.VCardUID
  depth?: number;
  // Canonical relation token or registry synonym ("brother" -> sibling_of).
  relation?: string;
}

// GET /graph/connections — from a starting contact, every reachable contact
// within depth hops, each with its chain of relation steps.
export async function getConnections(
  params: GetConnectionsParams,
): Promise<GraphConnectionsResponse> {
  const { from, depth, relation } = params;
  const queryParams = new URLSearchParams({ from });
  if (depth != null) queryParams.append('depth', String(depth));
  if (relation) queryParams.append('relation', relation);
  const response = await apiFetch(`${API_BASE_URL}/graph/connections?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
