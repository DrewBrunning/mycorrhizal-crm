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

// The storage-growth trend tier (issue #652): usage_percent folded against
// STORAGE_WARN_PERCENT / STORAGE_CRITICAL_PERCENT (with -5% hysteresis). A
// warning/critical tier elevates the overall status to at least degraded.
export type StorageThreshold = 'ok' | 'warning' | 'critical';

export interface SystemStatusStorage {
  database_bytes: number;
  filesystem: SystemStatusFilesystem;
  // Never null on the wire — marshals as [] when no directory is configured.
  directories: DirectoryUsage[];
  // Growth deltas (latest sample minus the oldest within the window) over the
  // persisted storage_samples series. Null until the window holds >= 2 samples.
  growth_7d_bytes?: number | null;
  growth_30d_bytes?: number | null;
  growth_90d_bytes?: number | null;
  // Linear fit of filesystem-used over the last 30 days extrapolated to
  // capacity; null when slope <= 0 or history < 14 days.
  projected_full_at?: string | null;
  // Filesystem used as a percentage of total; null when it can't be stat'ed.
  usage_percent?: number | null;
  // Optional for defensive rendering: the page defaults a missing threshold to
  // 'ok' (no banner), matching its dash-for-missing-fields posture.
  threshold?: StorageThreshold;
}

// Opt-in update-availability block (issue #650). `enabled` mirrors
// UPDATE_CHECK_ENABLED (default off); the rest is only populated when the
// flag is on and a lookup succeeded, so every field except `enabled` is
// optional — an operator with the flag off sees exactly `{enabled: false}`.
export interface SystemStatusUpdate {
  enabled: boolean;
  current?: string;
  latest?: string;
  update_available?: boolean;
  checked_at?: string | null;
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
  update: SystemStatusUpdate;
}

// GET /admin/system-status — the composite operational snapshot.
export async function getSystemStatus(): Promise<SystemStatusResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/system-status`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
