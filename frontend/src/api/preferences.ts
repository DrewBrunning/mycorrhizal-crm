// Preference API calls -- T20a (T20a,
// docs/adrs/0001-neutral-hub-and-spoke-contact-model.md).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Mirrors backend/models/preference.go's conventional (open) category set.
// The category set was trimmed to the two display groups the Preferences tab
// now shows: food/drink together ("Food & Drink Preferences") and media
// ("Media Preferences"). hobby was dropped because it lives in
// Card.PersonalInfo[kind=hobby] (the vCard 4.0 home); gift because gifts are
// their own entity; dislike because it is now a `key` under food/drink, not a
// category. The backend keeps the classifier open — an unrecognized category
// still displays via its raw token (see PREFERENCE_DEFAULT_KEYS' note).
export const PREFERENCE_CATEGORIES = ['food', 'drink', 'media'] as const;
export type PreferenceCategory = (typeof PREFERENCE_CATEGORIES)[number];

// Default `key` suggestions per category, offered by the free-solo key input
// in the preference dialog. These are conveniences, not a closed enum — the
// key stays free text, exactly like gender in contact creation.
export const PREFERENCE_DEFAULT_KEYS: Record<PreferenceCategory, string[]> = {
  food: ['favorite', 'dislike', 'allergy'],
  drink: ['favorite', 'dislike', 'allergy'],
  media: ['show', 'movie', 'music'],
};

// clothing_size is managed from the Gifts tab (where you check sizes before
// buying), so it is not offered in the general preference dialog but is still
// a valid category on the wire.
export const PREFERENCE_CLOTHING_SIZE = 'clothing_size';

// Mirrors backend/models/preference.go's closed Source set (§91.9).
export type PreferenceSource = 'conversation_note' | 'user' | 'ai-suggested' | 'external';
// Mirrors the shared normal/private/secret set (RelationshipEdgeSensitivity).
export type PreferenceSensitivity = 'normal' | 'private' | 'secret';

export interface Preference {
  id: string;
  created_at: string;
  updated_at: string;
  entity_id: string;
  category: string;
  key?: string;
  value: string;
  source?: PreferenceSource;
  confidence?: number;
  last_confirmed?: string;
  sensitivity: PreferenceSensitivity;
}

export interface PreferenceInput {
  entity_id: string;
  category: string;
  key?: string;
  value: string;
  source?: PreferenceSource;
  confidence?: number;
  last_confirmed?: string;
  sensitivity?: PreferenceSensitivity;
}

export interface PreferencesResponse {
  preferences: Preference[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
}

// GET /preferences
export async function getPreferences(params?: {
  entityId?: string;
  cursor?: string;
  limit?: number;
}): Promise<PreferencesResponse> {
  const { entityId, cursor, limit = 100 } = params || {};
  const queryParams = new URLSearchParams({ limit: limit.toString() });
  if (entityId) queryParams.append('entity_id', entityId);
  if (cursor) queryParams.append('cursor', cursor);
  const response = await apiFetch(
    `${API_BASE_URL}/preferences?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /preferences
export async function createPreference(input: PreferenceInput): Promise<Preference> {
  const response = await apiFetch(`${API_BASE_URL}/preferences`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.preference;
}

// PUT /preferences/:id (full-replace)
export async function updatePreference(id: string, input: PreferenceInput): Promise<Preference> {
  const response = await apiFetch(`${API_BASE_URL}/preferences/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /preferences/:id
export async function deletePreference(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/preferences/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
