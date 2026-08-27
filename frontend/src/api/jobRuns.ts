// Background-job run history + folded per-job health — issue #391, the
// frontend half of GET /admin/job-runs and GET /admin/job-runs/health.
// Admin-only, read-only, instance-wide. The backend derives the health on read
// by folding the job_runs history; there is nothing to write.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// Result / trigger vocabularies — hand-maintained mirrors of
// backend/models/job_run.go, migration 000041's CHECK constraint,
// backend/openapi.yaml, and the Android core/model mirror (CLAUDE.md frontend
// trap #4). Keep them in sync.
export type JobRunResult = 'success' | 'failure' | 'skipped';
export const JOB_RUN_RESULTS: JobRunResult[] = ['success', 'failure', 'skipped'];

export type JobRunTrigger = 'scheduled' | 'initial' | 'manual';

// healthy = the last executed run succeeded; failing = it failed; unknown = no
// executed run on record (a job that has only ever been skipped is unknown).
export type JobRunStatus = 'healthy' | 'failing' | 'unknown';

export interface JobRun {
  id: number;
  created_at: string;
  job_name: string;
  trigger: JobRunTrigger;
  started_at: string;
  finished_at: string;
  duration_ms: number;
  result: JobRunResult;
  error?: string;
  items_processed?: number | null;
  detail?: string;
  correlation_id: string;
}

export interface JobRunsResponse {
  job_runs: JobRun[];
  total: number;
}

export interface JobRunHealth {
  job_name: string;
  status: JobRunStatus;
  // Most recent run of ANY result (including skipped) — what the job did last.
  last_run_at: string | null;
  last_result: string;
  last_trigger: string;
  last_duration_ms: number | null;
  last_items_processed: number | null;
  last_success_at: string | null;
  last_failure_at: string | null;
  // Sanitized error of the most recent failure; empty unless status is failing.
  last_error: string;
  // First failure of the current unbroken run — non-null exactly when
  // consecutive_failures > 0.
  incident_first_failure_at: string | null;
  consecutive_failures: number;
  // avg/max over the last N executed (non-skipped) runs — the slow-creep trend.
  duration_sample_size: number;
  avg_duration_ms: number | null;
  max_duration_ms: number | null;
}

export interface JobRunHealthResponse {
  jobs: JobRunHealth[];
}

export interface ListJobRunsParams {
  jobName?: string;
  result?: JobRunResult;
  since?: string;
  until?: string;
  limit?: number;
}

// GET /admin/job-runs/health — one entry per known background job, in
// scheduler-registration order.
export async function getJobRunHealth(): Promise<JobRunHealthResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/job-runs/health`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// GET /admin/job-runs — run history, newest-first, for the per-job drill-down.
export async function listJobRuns(params: ListJobRunsParams = {}): Promise<JobRunsResponse> {
  const qs = new URLSearchParams();
  if (params.jobName) qs.set('job_name', params.jobName);
  if (params.result) qs.set('result', params.result);
  if (params.since) qs.set('since', params.since);
  if (params.until) qs.set('until', params.until);
  if (params.limit != null) qs.set('limit', String(params.limit));
  const suffix = qs.toString() ? `?${qs.toString()}` : '';

  const response = await apiFetch(`${API_BASE_URL}/admin/job-runs${suffix}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
