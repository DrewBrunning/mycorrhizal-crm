// OccasionEvent API calls (docs/adrs/0026-occasions-events.md, issue #1228): a
// one-off event the user is hosting, plus its attendee/RSVP list. RSVP is a
// manually-recorded status ("what they told you"), not a delivered invitation —
// this feature sends nothing.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { OccasionSensitivity } from './occasionObligations';

export type OccasionEventRSVP = 'pending' | 'accepted' | 'declined' | 'maybe';

export const OCCASION_EVENT_RSVPS: OccasionEventRSVP[] = [
  'pending',
  'accepted',
  'declined',
  'maybe',
];

export interface OccasionEvent {
  id: string;
  created_at: string;
  updated_at: string;
  title: string;
  starts_at: string;
  ends_at?: string;
  location?: string;
  sensitivity: OccasionSensitivity;
  notes?: string;
  // Change-feed tombstone marker (T17), only true via ?since=.
  deleted?: boolean;
}

export interface OccasionEventInput {
  title: string;
  starts_at: string;
  ends_at?: string | null;
  location?: string;
  sensitivity?: OccasionSensitivity;
  notes?: string;
}

export interface OccasionEventAttendee {
  id: string;
  created_at: string;
  updated_at: string;
  event_id: string;
  entity_id: string;
  rsvp: OccasionEventRSVP;
}

export interface OccasionEventAttendeeView {
  id: string;
  event_id: string;
  entity_id: string;
  contact_id: number;
  contact_name: string;
  rsvp: OccasionEventRSVP;
}

export interface InviteeSuggestion {
  contact_id: number;
  contact_name: string;
  entity_id: string;
}

export interface OccasionEventsResponse {
  occasion_events: OccasionEvent[];
  total: number;
  next_cursor: string;
  limit: number;
}

export interface OccasionEventDetail {
  occasion_event: OccasionEvent;
  attendees: OccasionEventAttendeeView[];
}

// GET /occasion-events
export async function getOccasionEvents(params?: {
  from?: string;
  to?: string;
  cursor?: string;
  limit?: number;
}): Promise<OccasionEventsResponse> {
  const { from, to, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (from) queryParams.append('from', from);
  if (to) queryParams.append('to', to);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(`${API_BASE_URL}/occasion-events?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// GET /occasion-events/:id
export async function getOccasionEvent(id: string): Promise<OccasionEventDetail> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-events/${id}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /occasion-events — create is wrapped in {occasion_event: ...}
export async function createOccasionEvent(input: OccasionEventInput): Promise<OccasionEvent> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-events`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.occasion_event;
}

// PUT /occasion-events/:id (full-replace)
export async function updateOccasionEvent(
  id: string,
  input: OccasionEventInput,
): Promise<OccasionEvent> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-events/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /occasion-events/:id (soft delete)
export async function deleteOccasionEvent(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-events/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// POST /occasion-events/:id/attendees
export async function addOccasionEventAttendee(
  eventId: string,
  input: { entity_id: string; rsvp?: OccasionEventRSVP },
): Promise<OccasionEventAttendee> {
  const response = await apiFetch(`${API_BASE_URL}/occasion-events/${eventId}/attendees`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.attendee;
}

// PUT /occasion-events/:id/attendees/:vcard_uid — records the RSVP status.
export async function updateOccasionEventAttendee(
  eventId: string,
  vcardUid: string,
  rsvp: OccasionEventRSVP,
): Promise<OccasionEventAttendee> {
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-events/${eventId}/attendees/${vcardUid}`,
    {
      method: 'PUT',
      headers: getAuthHeaders(),
      body: JSON.stringify({ rsvp }),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /occasion-events/:id/attendees/:vcard_uid
export async function removeOccasionEventAttendee(
  eventId: string,
  vcardUid: string,
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-events/${eventId}/attendees/${vcardUid}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    },
  );
  if (!response.ok) throw await parseErrorResponse(response);
}

// GET /occasion-events/invitee-suggestions — expands circles into candidates,
// excluding anyone already invited when eventId is given.
export async function getInviteeSuggestions(params: {
  circleIds: string[];
  eventId?: string;
}): Promise<InviteeSuggestion[]> {
  const queryParams = new URLSearchParams({ circle_ids: params.circleIds.join(',') });
  if (params.eventId) queryParams.append('event_id', params.eventId);
  const response = await apiFetch(
    `${API_BASE_URL}/occasion-events/invitee-suggestions?${queryParams.toString()}`,
    { headers: getAuthHeaders() },
  );
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.suggestions || [];
}
