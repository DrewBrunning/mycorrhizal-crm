// Contact timeline API calls -- backend endpoint is T66
// (docs/fork-plan/tickets/110-T66-contact-timeline-bounded-view-and-
// explorer.md), consumed here by the T78 web explorer. GET
// /contacts/:id/timeline returns a cursor-paginated page of a contact's
// merged timeline across all six event types, filterable by ?type=
// (comma-separated subset) and ?bucket= (recency).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { Note } from './notes';
import { Activity } from './activities';
import { ReminderCompletion } from './reminders';
import { LifeEvent } from './lifeEvents';
import { ExternalActivity } from './externalLinks';
import { Gift } from './gifts';

// The six raw timeline event types, in the canonical order backend
// models/timeline.go uses for its sort tiebreak. Hardcoded mirror of that
// list -- this codebase has no dynamic type-list endpoint (CLAUDE.md
// frontend trap #4); keep in sync with backend/models/timeline.go by hand.
export const TIMELINE_TYPES = [
  'note',
  'activity',
  'completion',
  'life_event',
  'external_activity',
  'gift',
] as const;
export type TimelineType = (typeof TIMELINE_TYPES)[number];

// Recency buckets -- mirror of backend/models/timeline.go's TimelineBucket*
// constants; the backend validates against exactly this set.
export const TIMELINE_BUCKETS = [
  'last_7_days',
  'last_30_days',
  'last_90_days',
  'this_year',
  'all',
] as const;
export type TimelineBucket = (typeof TIMELINE_BUCKETS)[number];

// One entry in the merged timeline: the raw entity under `data` (switch on
// `type` before reading it), plus the normalized date the endpoint sorts on.
// `id` is the entity's PK as a string -- the six tables mix uint and
// UUID-string PKs -- and the row key the cursor resumes from.
export interface TimelineItem {
  type: TimelineType;
  id: string;
  date: string;
  data: Note | Activity | ReminderCompletion | LifeEvent | ExternalActivity | Gift;
}

export interface TimelineResponse {
  // Serialized `[]`, never null/absent, when the page is empty.
  items: TimelineItem[];
  // Opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

export interface GetTimelineParams {
  contactId: string | number;
  // Empty/undefined means all six types.
  types?: TimelineType[];
  bucket?: TimelineBucket;
  cursor?: string;
  limit?: number;
}

export async function getTimeline(params: GetTimelineParams): Promise<TimelineResponse> {
  const { contactId, types, bucket, cursor, limit = 25 } = params;
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  // Empty means all types; the full set is the same query as "no filter", so
  // omit it rather than sending all six tokens.
  if (types && types.length > 0 && types.length < TIMELINE_TYPES.length) {
    queryParams.append('type', types.join(','));
  }
  if (bucket && bucket !== 'all') {
    queryParams.append('bucket', bucket);
  }
  if (cursor) {
    queryParams.append('cursor', cursor);
  }

  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${contactId}/timeline?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
