// GeoPulse API calls — issue #160 (ADR 0033): correlate GeoPulse location
// history into human-confirmed Activity suggestions. The connection is
// per-user-global and the API token stays server-side (encrypted at rest); the
// browser only ever talks to this backend. The suggestion list is ephemeral —
// confirming one is an ordinary POST /activities (see api/activities.ts) with
// `external_ref` set to the suggestion's.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface GeoPulseConfigResponse {
  base_url: string;
  has_api_key: boolean;
}

export interface GeoPulseConfigInput {
  base_url: string;
  // Write-only: empty on update keeps the stored token; required on first connect.
  api_key?: string;
}

export interface GeoPulseConnectionTestResult {
  ok: boolean;
  stage: 'reachability' | 'auth' | 'ok';
  message: string;
}

// Display-only: never persisted on the Activity.
export interface GeoPulsePhotoSuggestion {
  id: string;
  file_name: string;
  taken_at: string;
}

export interface GeoPulseStaySuggestion {
  stay_id: number;
  // `geopulse:stay:<id>` — pass it through as the Activity's external_ref.
  external_ref: string;
  location: string;
  city: string;
  country: string;
  latitude: number;
  longitude: number;
  timestamp: string;
  duration_seconds: number;
  photos: GeoPulsePhotoSuggestion[];
  // True when the photo lookup failed or was skipped: an empty `photos` then
  // means "could not check", not "none".
  photos_unavailable: boolean;
  // Present when an Activity already carries this stay's external_ref.
  existing_activity_id?: number;
}

export interface GeoPulseSuggestionsResponse {
  date: string;
  suggestions: GeoPulseStaySuggestion[];
}

export async function getGeoPulseConfig(): Promise<GeoPulseConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/geopulse/config`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function saveGeoPulseConfig(
  input: GeoPulseConfigInput,
): Promise<GeoPulseConfigResponse> {
  const response = await apiFetch(`${API_BASE_URL}/geopulse/config`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteGeoPulseConfig(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/geopulse/config`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// Diagnoses the saved connection — reachability then API token validity. A
// diagnosed failure (ok: false) is still a 200; only a missing/unusable saved
// connection throws.
export async function testGeoPulseConnection(): Promise<GeoPulseConnectionTestResult> {
  const response = await apiFetch(`${API_BASE_URL}/geopulse/test-connection`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// One synchronous, user-initiated lookup for a single calendar day. `date` is
// YYYY-MM-DD and `timezone` the IANA zone that defines the day's boundaries
// (the server defaults to UTC).
export async function getGeoPulseSuggestions(
  date: string,
  timezone?: string,
): Promise<GeoPulseSuggestionsResponse> {
  const params = new URLSearchParams({ date });
  if (timezone) params.set('timezone', timezone);
  const response = await apiFetch(`${API_BASE_URL}/geopulse/suggestions?${params.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
