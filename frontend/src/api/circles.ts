import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { TimelineItem } from './timeline';

// Fixed palette tokens (issue #1995); mirror of backend models.CircleColors.
// '' means "no colour". Hardcoded mirror -- keep in sync by hand (frontend trap #4).
export const CIRCLE_COLORS = [
  'red',
  'orange',
  'yellow',
  'green',
  'teal',
  'blue',
  'purple',
  'pink',
  'grey',
] as const;
export type CircleColor = '' | (typeof CIRCLE_COLORS)[number];
export const CIRCLE_DESCRIPTION_MAX = 500;

export interface Circle {
  id: string;
  created_at: string;
  updated_at: string;
  name: string;
  description?: string;
  color?: CircleColor;
}

// Optional metadata on create/update. An omitted key keeps the stored value
// on update; '' clears it.
export interface CircleMetadata {
  description?: string;
  color?: CircleColor;
}

export interface CircleMember {
  id: number;
  circle_id: string;
  member_vcard_uid: string;
}

export interface CircleCreateResponse {
  message: string;
  circle: Circle;
}

export interface CircleListResponse {
  circles: Circle[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
  members?: CircleMember[];
}

// GET /circles
export async function listCircles(params?: {
  cursor?: string;
  limit?: number;
  include_members?: boolean;
}): Promise<CircleListResponse> {
  const { cursor, limit = 100, include_members = false } = params || {};
  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });
  if (cursor) queryParams.append('cursor', cursor);
  if (include_members) queryParams.append('include_members', 'true');
  const response = await apiFetch(`${API_BASE_URL}/circles?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /circles
export async function createCircle(
  name: string,
  metadata?: CircleMetadata,
): Promise<CircleCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/circles`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name, ...metadata }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// PUT /circles/:id
export async function updateCircle(
  id: string,
  name: string,
  metadata?: CircleMetadata,
): Promise<Circle> {
  const response = await apiFetch(`${API_BASE_URL}/circles/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name, ...metadata }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /circles/:id
export async function deleteCircle(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/circles/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// POST /circles/:id/members
export async function addCircleMember(
  circleId: string,
  memberVCardUid: string,
): Promise<CircleMember> {
  const response = await apiFetch(`${API_BASE_URL}/circles/${circleId}/members`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ member_vcard_uid: memberVCardUid }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /circles/:id/members/:vcard_uid
export async function removeCircleMember(circleId: string, memberVCardUid: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/circles/${circleId}/members/${memberVCardUid}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export interface CircleTimelineResponse {
  // Serialized `[]`, never null/absent, when empty.
  items: TimelineItem[];
  next_cursor: string;
  limit: number;
  // Members in the circle (archived members count but contribute no events).
  member_count: number;
}

// GET /circles/:id/timeline -- the merged timeline of the circle's live members.
export async function getCircleTimeline(
  circleId: string,
  params?: { cursor?: string; limit?: number },
): Promise<CircleTimelineResponse> {
  const { cursor, limit = 25 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(
    `${API_BASE_URL}/circles/${circleId}/timeline?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
