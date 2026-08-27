// Operational error aggregation API — issue #426, the frontend half of
// GET /admin/error-aggregation. Admin-only, read-only, instance-wide. The
// backend folds the failure rows of the operational-event stream (system_events,
// #424) over a rolling window into one bucket per cause; there is nothing to
// write, and buckets are data (not a fixed vocabulary), so no enum mirror is
// needed here.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// count at which the backend flags a bucket `recurring` — kept here only so the
// UI copy ("Recurring") and the styling threshold agree with
// backend/services/error_aggregation.go's errAggRecurringThreshold. A single
// transient failure is not recurring; a repeating cause is.
export const RECURRING_THRESHOLD = 3;

export interface ErrorBucket {
  // The system_events component the failures share (contact_sync, notification,
  // webhook, …).
  component: string;
  // The normalized error string — the low-cardinality bucket key.
  cause: string;
  // The most recent raw error string in the bucket (already sanitized and
  // length-capped server-side); shown so an operator still sees a real instance.
  sample_error: string;
  // Sorted distinct event_type values in the bucket (usually one).
  event_types: string[];
  count: number;
  recurring: boolean;
  first_seen: string;
  last_seen: string;
  // The exact system_events row ids in the bucket (capped at 500) — pass to the
  // timeline's `ids` filter to open the underlying events.
  event_ids: number[];
  event_ids_truncated: boolean;
}

export interface ErrorAggregationResponse {
  window_hours: number;
  since: string;
  until: string;
  total_events: number;
  buckets: ErrorBucket[];
}

// GET /admin/error-aggregation — buckets over the window, most frequent first
// (then most recently seen).
export async function getErrorAggregation(windowHours = 24): Promise<ErrorAggregationResponse> {
  const q = new URLSearchParams({ window_hours: windowHours.toString() });
  const response = await apiFetch(`${API_BASE_URL}/admin/error-aggregation?${q.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
