// Health / build identity API.
//
// Exists so the running build can be shown to the user. /health (the deep
// check) reports the version, commit and build date injected at link time
// (backend/buildinfo); before that it returned a hardcoded "0.1.0" for every
// build ever made, so a bug report could not be tied to the binary that
// produced it.
//
// The backend health surface is three endpoints (issue #421): /health/live
// (liveness), /health/ready (readiness), and /health (deep). Only the deep
// endpoint carries the build identity, so that is the one this module hits.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// /health is registered at the server root, NOT under /api/v1 (see
// backend/routes/routes.go), so the versioned prefix has to be stripped.
const HEALTH_URL = `${API_BASE_URL.replace(/\/api\/v1$/, '')}/health`;

export interface DatabaseHealth {
  status: string;
  response_time_ms: number;
}

export interface HealthResponse {
  /** healthy | degraded | unhealthy (issue #421). "degraded" is still HTTP 200. */
  status: string;
  timestamp: string;
  database: DatabaseHealth;
  version: string;
  /** Short git SHA; absent on a build with no VCS info. May carry a "-dirty" suffix. */
  commit?: string;
  build_date?: string;
  /** Per-facet deep-health breakdown. Present on the deep /health response;
   * shape mirrors backend services.DeepHealth. Not consumed by the build card. */
  checks?: Record<string, unknown>;
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
