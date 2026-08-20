// ReachOutSuggestion API calls -- issue #177, the event-driven counterpart
// to cadence.overdue: a detected organization/title/address change on a
// contact, surfaced as a dismissible dashboard suggestion.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Kind tokens -- MUST be kept in sync by hand with
// backend/models/reach_out_suggestion.go's ReachOutKind* constants (this
// repo has no dynamic type-list endpoint; every enum is a hardcoded frontend
// mirror).
export type ReachOutSuggestionKind = 'organization' | 'title' | 'address';

export interface ReachOutSuggestion {
  id: string;
  created_at: string;
  updated_at: string;
  contact_vcard_uid: string;
  kind: ReachOutSuggestionKind;
  old_value: string;
  new_value: string;
  audit_event_id: number;
  reminder_id?: number;
  status: 'pending' | 'dismissed';
  contact_id: number;
  contact_name: string;
  photo_thumbnail?: string;
}

export interface ReachOutSuggestionsResponse {
  suggestions: ReachOutSuggestion[];
}

export async function getReachOutSuggestions(): Promise<ReachOutSuggestionsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/reach-out-suggestions`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function dismissReachOutSuggestion(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/reach-out-suggestions/${id}/dismiss`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
