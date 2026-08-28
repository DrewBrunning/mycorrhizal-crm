// Admin system-status API — the frontend half of GET /admin/system-status
// (issue #388), rendered at /system-status (issue #649). Admin-only,
// read-only, instance-wide. The backend derives the snapshot on read (deep
// health + config read-back + storage walk); there is nothing to write.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// The rolled-up facet status. The overall picture uses healthy | degraded |
// unhealthy; individual deep-health facets use ok | degraded | unhealthy |
// not_configured (mirroring services.DeepStatus* in backend/services).
export type OverallStatus = 'healthy' | 'degraded' | 'unhealthy';

export interface HealthCheckDetail {
  status: string;
  reason?: string;
}

export interface BackgroundJobStatus {
  name: string;
  last_run_at?: string | null;
  locked: boolean;
  stuck: boolean;
}

export interface BackgroundJobsCheck {
  status: string;
  reason?: string;
  jobs?: BackgroundJobStatus[];
}

// Mirrors backend services.DeepHealth — the same shape /health reports, plus
// the rolled-up status. reason strings are already sanitized server-side.
export interface DeepHealth {
  status: OverallStatus;
  database: HealthCheckDetail;
  migrations: HealthCheckDetail;
  integrity_check: HealthCheckDetail;
  restore_drill: HealthCheckDetail;
  background_jobs: BackgroundJobsCheck;
  integrations: Record<string, HealthCheckDetail>;
}

// Mirrors backend buildinfo.Info — identity of the running binary.
export interface BuildInfo {
  version: string;
  commit?: string;
  build_date?: string;
}

export interface SystemStatusUptime {
  started_at: string; // RFC3339
  uptime_seconds: number;
}

export interface SystemStatusMigration {
  applied: number;
  latest: number;
  pending: number;
  dirty: boolean;
}

export interface SystemStatusConfigValidation {
  field: string;
  message: string;
}

// The feature-flag keys, mirroring backend SystemStatusFeatures EXACTLY.
// No dynamic type-list endpoint exists — this is a hand-maintained mirror of
// the backend (CLAUDE.md frontend trap #4). Keep the two in sync.
export const SYSTEM_STATUS_FEATURE_KEYS = [
  'carddav',
  'caldav',
  'oidc',
  'email',
  'metrics',
  'db_integrity_check',
  'db_restore_drill',
] as const;

export type SystemStatusFeatureKey = (typeof SYSTEM_STATUS_FEATURE_KEYS)[number];

export type SystemStatusFeatures = Record<SystemStatusFeatureKey, boolean>;

export interface SystemStatusConfig {
  // Never null on the wire — marshals as [] on a clean boot (frontend trap #8).
  validation: SystemStatusConfigValidation[];
  features: SystemStatusFeatures;
}

export interface SystemStatusDatabase {
  sqlite_version: string;
  journal_mode: string;
  wal_bytes: number;
}

// Mirrors backend services.DirectoryUsage.
export interface DirectoryUsage {
  path: string;
  bytes: number;
  file_count: number;
  truncated: boolean;
}

export interface SystemStatusFilesystem {
  free_bytes: number;
  total_bytes: number;
}

export interface SystemStatusStorage {
  database_bytes: number;
  filesystem: SystemStatusFilesystem;
  // Never null on the wire — marshals as [] when no directory is configured.
  directories: DirectoryUsage[];
}

// Field-for-field mirror of backend SystemStatusResponse
// (backend/controllers/system_status_controller.go).
export interface SystemStatusResponse {
  overall: OverallStatus;
  health: DeepHealth;
  version: BuildInfo;
  uptime: SystemStatusUptime;
  migration: SystemStatusMigration;
  config: SystemStatusConfig;
  database: SystemStatusDatabase;
  storage: SystemStatusStorage;
}

// GET /admin/system-status — the composite operational snapshot.
export async function getSystemStatus(): Promise<SystemStatusResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/system-status`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
