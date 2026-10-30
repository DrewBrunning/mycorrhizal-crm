// Activities-related API calls
import {
  API_BASE_URL,
  apiFetch,
  getAuthHeaders,
  getConditionalHeaders,
  parseErrorResponse,
} from './client';

export interface ActivityContact {
  ID: number;
  firstname: string;
  lastname: string;
  nickname?: string;
}

export interface Activity {
  ID: number;
  title: string;
  description?: string;
  location?: string;
  date: string;
  // Issue #2006: inclusive last day of a multi-day activity; same calendar-day
  // RFC 3339 shape as `date`. Absent for a single-day activity.
  end_date?: string;
  CreatedAt: string;
  UpdatedAt: string;
  contacts?: ActivityContact[];
  // Issue #591: monotonic per-row write counter, read-only (ADR 0006).
  revision?: number;
}

export interface ActivitiesResponse {
  activities: Activity[];
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

export interface GetActivitiesParams {
  cursor?: string;
  limit?: number;
  includeContacts?: boolean;
  search?: string;
  fromDate?: string;
  toDate?: string;
}

// Get a page of activities, resumable via next_cursor (T17)
export async function getActivities(params: GetActivitiesParams): Promise<ActivitiesResponse> {
  const { cursor, limit = 25, includeContacts = false } = params;
  const search = params.search?.trim();

  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });

  if (cursor) queryParams.append('cursor', cursor);
  if (includeContacts) {
    queryParams.append('include', 'contacts');
  }

  if (search) {
    queryParams.append('search', search);
  }

  if (params.fromDate) {
    queryParams.append('fromDate', params.fromDate);
  }

  if (params.toDate) {
    queryParams.append('toDate', params.toDate);
  }

  const response = await apiFetch(`${API_BASE_URL}/activities?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get activities for a contact
export async function getContactActivities(
  contactId: string | number,
): Promise<{ activities: Activity[] }> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/activities`, {
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

export type AddToActivityStatus = 'added' | 'already_member' | 'not_found';

export interface AddContactToActivitiesResponse {
  contact_id: number;
  results: { activity_id: number; status: AddToActivityStatus }[];
}

// Add a contact to existing activities. Writes only the membership: no other
// activity field or member changes, and a repeat is reported already_member.
export async function addContactToActivities(
  contactId: string | number,
  activityIds: number[],
): Promise<AddContactToActivitiesResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/activities`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ activity_ids: activityIds }),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get single activity
export async function getActivity(id: string | number): Promise<Activity> {
  const response = await apiFetch(`${API_BASE_URL}/activities/${id}`, {
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Create activity
export async function createActivity(data: {
  title: string;
  description: string;
  location: string;
  date: string;
  end_date?: string;
  contact_ids: number[];
  // Opaque reference to the subsystem the activity came from. A repeat create of
  // a `geopulse:stay:<id>` ref returns the existing activity instead of a second.
  external_ref?: string;
}): Promise<Activity> {
  const response = await apiFetch(`${API_BASE_URL}/activities`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Update activity
export async function updateActivity(
  id: string | number,
  data: {
    title?: string;
    description?: string;
    location?: string;
    date?: string;
    // Omitted keeps the stored end date; null clears it (issue #2006).
    end_date?: string | null;
    contact_ids?: number[];
  },
  revision?: number,
): Promise<Activity> {
  const response = await apiFetch(`${API_BASE_URL}/activities/${id}`, {
    method: 'PUT',
    headers: getConditionalHeaders(revision),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Delete activity
export async function deleteActivity(id: string | number, revision?: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/activities/${id}`, {
    method: 'DELETE',
    headers: getConditionalHeaders(revision),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}
