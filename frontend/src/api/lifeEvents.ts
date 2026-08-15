import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// LifeEvent category tokens — T36. Hardcoded mirror of backend/models/life_event.go's
// LifeEventCategory* constants (frontend-trap-4 in CLAUDE.md: there is no
// dynamic type-list endpoint, by design — keep this in sync by hand).
export const LIFE_EVENT_CATEGORIES = [
  'home_living',
  'health_wellness',
  'work_education',
  'travel_experiences',
  'family_relationships',
] as const;
export type LifeEventCategory = (typeof LIFE_EVENT_CATEGORIES)[number];

// LifeEvent type tokens, grouped by category and ordered exactly as the
// ticket's authoritative list (itself the order Monica settled on through
// real use) — must stay in sync with backend/models/life_event_type_
// registry.go's LifeEventTypeCategories / LifeEventTypesForCategory.
export const LIFE_EVENT_TYPES_BY_CATEGORY: Record<LifeEventCategory, readonly string[]> = {
  home_living: [
    'moved',
    'bought_a_home',
    'made_a_home_improvement',
    'went_on_holidays',
    'got_a_new_vehicle',
    'got_a_roommate',
  ],
  health_wellness: [
    'overcame_an_illness',
    'quit_a_habit',
    'started_new_eating_habits',
    'lost_weight',
    'started_wearing_glasses_or_contacts',
    'broke_a_bone',
    'removed_braces',
    'had_surgery',
    'went_to_the_dentist',
  ],
  work_education: [
    'job_change',
    'retired',
    'started_school',
    'studied_abroad',
    'started_volunteering',
    'published_a_paper',
    'started_military_service',
    'graduated',
  ],
  travel_experiences: [
    'started_a_sport',
    'started_a_hobby',
    'learned_a_new_instrument',
    'learned_a_new_language',
    'got_a_tattoo_or_piercing',
    'got_a_license',
    'traveled',
    'got_an_achievement_or_award',
    'changed_beliefs',
    'spoke_for_the_first_time',
    'kissed_for_the_first_time',
  ],
  family_relationships: [
    'started_a_relationship',
    'got_engaged',
    'married',
    'anniversary',
    'expects_a_baby',
    'had_child',
    'added_a_family_member',
    'adopted_pet',
    'ended_a_relationship',
    'lost_a_loved_one',
  ],
};

/**
 * Reports whether token is one of the five known category tokens — guards
 * an indexed lookup into LIFE_EVENT_TYPES_BY_CATEGORY against an unknown
 * value (stale/corrupted data, or a category this frontend copy predates —
 * the frontend-trap-4 mirror-drift scenario) so callers narrow to
 * LifeEventCategory instead of blindly casting.
 */
export function isKnownLifeEventCategory(token: string): token is LifeEventCategory {
  return (LIFE_EVENT_CATEGORIES as readonly string[]).includes(token);
}

export interface PartialDate {
  year?: number;
  month?: number;
  day?: number;
}

export interface LifeEvent {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  type: string;
  category?: string;
  date?: PartialDate;
  description?: string;
  source?: string;
  related_entity_ids?: string[];
  remind?: boolean;
}

export interface LifeEventCreateResponse {
  message: string;
  life_event: LifeEvent;
}

export interface LifeEventListResponse {
  life_events: LifeEvent[];
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

export interface GetLifeEventsParams {
  entity_id?: string;
  cursor?: string;
  limit?: number;
}

export async function getLifeEvents(
  params: GetLifeEventsParams = {}
): Promise<LifeEventListResponse> {
  const { entity_id, cursor, limit = 25 } = params;
  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });
  if (cursor) queryParams.append('cursor', cursor);
  if (entity_id) queryParams.append('entity_id', entity_id);

  const response = await apiFetch(
    `${API_BASE_URL}/life-events?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export interface LifeEventInputData {
  entity_id: string;
  type: string;
  category?: string;
  date?: PartialDate;
  description?: string;
  source?: string;
  related_entity_ids?: string[];
  remind?: boolean;
}

export async function createLifeEvent(
  data: LifeEventInputData
): Promise<LifeEventCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/life-events`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function updateLifeEvent(
  id: string,
  data: LifeEventInputData
): Promise<LifeEvent> {
  const response = await apiFetch(`${API_BASE_URL}/life-events/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deleteLifeEvent(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/life-events/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export function partialDateDisplay(date?: PartialDate): string {
  if (!date) return '';
  const y = date.year != null ? String(date.year) : '';
  const m = date.month != null ? String(date.month).padStart(2, '0') : '';
  const d = date.day != null ? String(date.day).padStart(2, '0') : '';
  if (y && m && d) return `${y}-${m}-${d}`;
  if (y) return y;
  if (m && d) return `${m}/${d}`;
  if (m) return `${m}/??`;
  return '';
}

export function partialDateHasMonthDay(date?: PartialDate): boolean {
  return date != null && date.month != null && date.day != null;
}

export function partialDateIsYearOnly(date?: PartialDate): boolean {
  return date != null && date.year != null && date.month == null && date.day == null;
}
