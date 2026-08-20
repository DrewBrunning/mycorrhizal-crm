// Preference API calls -- T20a (T20a,
// docs/adrs/0001-neutral-hub-and-spoke-contact-model.md).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

// Mirrors backend/models/preference.go's conventional (open) category set.
//
// The redesign's rule: a domain that needs both a "what kind of thing" axis
// and a "how do they feel about it" axis (media, jewelry) pushes the kind
// into the category (e.g. media_movie, jewelry_metal — cheap to extend,
// exactly like clothing_size already does) and reserves `key` uniformly for
// disposition (favorite/like/dislike/allergy, a suggestion list per category,
// still free text). `dislike` is the one exception: a general,
// non-domain-specific gift-avoidance note (e.g. "no candles"), so it has no
// disposition of its own — `keyMode: 'freeSolo'` with no suggestions.
//
// The backend keeps the classifier open — an unrecognized category still
// displays via its raw token (Chip label falls back to the token itself).
//
// Sections also decide which tab a category surfaces in. foodDrink/media/
// hobby are "get to know them" facts and live in the Overview tab's
// Preferences panel; jewelry/giftPreferences/giftAvoid are "check this right
// before buying" facts and live in the Gifts tab, alongside clothing sizes —
// see GIFTS_TAB_SECTIONS below. hobby gets its own section (rather than
// folding into giftPreferences) specifically so it stays in Preferences: an
// activity is as much a conversational fact as a gift idea.
export type PreferenceSection = 'foodDrink' | 'media' | 'hobby' | 'jewelry' | 'giftPreferences' | 'giftAvoid';
export type PreferenceKeyMode = 'disposition' | 'freeSolo';

// Sections whose categories surface in the Gifts tab (alongside clothing
// sizes) rather than the Overview tab's Preferences panel.
export const GIFTS_TAB_SECTIONS: PreferenceSection[] = ['jewelry', 'giftPreferences', 'giftAvoid'];
// The complement: sections shown in the Overview tab's Preferences panel and
// dialog. Every PreferenceSection must appear in exactly one of these two
// lists — enforced by locales.test.ts-style coverage isn't set up for this,
// so keep them in sync by hand when adding a new section.
export const OVERVIEW_TAB_SECTIONS: PreferenceSection[] = ['foodDrink', 'media', 'hobby'];

export interface PreferenceCategoryConfig {
  category: string;
  section: PreferenceSection;
  keyMode: PreferenceKeyMode;
  keySuggestions: string[];
}

const DISPOSITION = ['favorite', 'like', 'dislike'];
const DISPOSITION_WITH_ALLERGY = ['favorite', 'like', 'dislike', 'allergy'];

export const PREFERENCE_CATEGORY_CONFIG: PreferenceCategoryConfig[] = [
  // Food & Drink — presented as one merged section in the UI, but kept as
  // two DB categories so food-vs-drink stays separately filterable/exportable
  // (the CSV export's "Food Preference" column reads category=food only).
  { category: 'food', section: 'foodDrink', keyMode: 'disposition', keySuggestions: ['favorite', 'dislike', 'allergy'] },
  { category: 'drink', section: 'foodDrink', keyMode: 'disposition', keySuggestions: ['favorite', 'dislike', 'allergy'] },

  // Media — medium (and, for music/books, facet) lives in the category.
  { category: 'media_movie', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_tv', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_game', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_podcast', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_music_artist', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_music_album', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_music_genre', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_music_song', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_book_author', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_book_series', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'media_book_title', section: 'media', keyMode: 'disposition', keySuggestions: DISPOSITION },

  // Jewelry & Style — aspect (metal/stone/style/type) lives in the category.
  { category: 'jewelry_metal', section: 'jewelry', keyMode: 'disposition', keySuggestions: DISPOSITION_WITH_ALLERGY },
  { category: 'jewelry_stone', section: 'jewelry', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'jewelry_style', section: 'jewelry', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'jewelry_type', section: 'jewelry', keyMode: 'disposition', keySuggestions: DISPOSITION },

  // Activities & Hobbies — a "get to know them" fact, stays in Preferences.
  { category: 'hobby', section: 'hobby', keyMode: 'disposition', keySuggestions: DISPOSITION },

  // Gift Preferences — single-facet "tastes", each its own category chip.
  { category: 'flowers', section: 'giftPreferences', keyMode: 'disposition', keySuggestions: DISPOSITION_WITH_ALLERGY },
  { category: 'color', section: 'giftPreferences', keyMode: 'disposition', keySuggestions: DISPOSITION },
  { category: 'fragrance', section: 'giftPreferences', keyMode: 'disposition', keySuggestions: DISPOSITION_WITH_ALLERGY },
  { category: 'cause', section: 'giftPreferences', keyMode: 'disposition', keySuggestions: ['favorite', 'like'] },

  // Gift Avoid — general, non-domain-specific avoidance notes.
  { category: 'dislike', section: 'giftAvoid', keyMode: 'freeSolo', keySuggestions: [] },
];

export const PREFERENCE_CATEGORIES = PREFERENCE_CATEGORY_CONFIG.map((c) => c.category);
export type PreferenceCategory = string;

// Default `key` suggestions per category, offered by the free-solo key input
// in the preference dialog. These are conveniences, not a closed enum — the
// key stays free text, exactly like gender in contact creation.
export const PREFERENCE_DEFAULT_KEYS: Record<string, string[]> = Object.fromEntries(
  PREFERENCE_CATEGORY_CONFIG.map((c) => [c.category, c.keySuggestions]),
);

const CATEGORY_TO_SECTION: Record<string, PreferenceSection> = Object.fromEntries(
  PREFERENCE_CATEGORY_CONFIG.map((c) => [c.category, c.section]),
);

// True for jewelry_*/flowers/color/fragrance/cause/dislike — the categories
// that surface in the Gifts tab. An unrecognized category (not in
// PREFERENCE_CATEGORY_CONFIG at all — legacy data, or a future addition)
// returns false, so it falls to the Overview tab's "Other" catch-all rather
// than silently hiding from both.
export function isGiftsTabCategory(category: string): boolean {
  const section = CATEGORY_TO_SECTION[category];
  return section != null && GIFTS_TAB_SECTIONS.includes(section);
}

// clothing_size is managed from the Gifts tab (where you check sizes before
// buying), so it is not offered in the general preference dialog but is still
// a valid category on the wire. Key holds a free-solo clothing *type* here
// (shirt, ring, ...) rather than a disposition — sizing is a fact, not a
// taste, so favorite/dislike don't apply.
export const PREFERENCE_CLOTHING_SIZE = 'clothing_size';
export const CLOTHING_TYPE_SUGGESTIONS = [
  'shirt', 'pants', 'dress', 'skirt', 'undergarments', 'outerwear',
  'shoe', 'hat', 'glove', 'belt', 'ring', 'socks',
];

// Mirrors backend/models/preference.go's closed Source set .
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
  notes?: string;
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
  notes?: string;
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
