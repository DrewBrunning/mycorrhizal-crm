// Health / build identity API.
//
// Exists so the running build can be shown to the user. /health reports the
// version, commit and build date injected at link time (backend/buildinfo);
// before that it returned a hardcoded "0.1.0" for every build ever made, so a
// bug report could not be tied to the binary that produced it.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// /health is registered at the server root, NOT under /api/v1 (see
// backend/routes/routes.go), so the versioned prefix has to be stripped.
const HEALTH_URL = `${API_BASE_URL.replace(/\/api\/v1$/, '')}/health`;

export interface DatabaseHealth {
  status: string;
  response_time_ms: number;
}

export interface HealthResponse {
  status: string;
  timestamp: string;
  database: DatabaseHealth;
  version: string;
  /** Short git SHA; absent on a build with no VCS info. May carry a "-dirty" suffix. */
  commit?: string;
  build_date?: string;
}

// GET /health — unauthenticated, but auth headers are sent when present so the
// call behaves like every other one in this module.
export async function getHealth(): Promise<HealthResponse> {
  const response = await apiFetch(HEALTH_URL, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

/**
 * Formats the build identity for display, e.g. "v0.2.0 (abc1234)".
 * Falls back to the bare version when no commit was stamped.
 */
export function formatBuildVersion(health: Pick<HealthResponse, 'version' | 'commit'>): string {
  if (!health.commit) return health.version;
  return `${health.version} (${health.commit})`;
}
