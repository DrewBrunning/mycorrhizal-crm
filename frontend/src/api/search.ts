// Full-text search API (T11): search across contacts, notes, and
// interactions. Backed by backend FTS5; user-scoped server-side.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface SearchContactHit {
  id: number;
  uid: string;
  firstname: string;
  lastname: string;
  nickname?: string;
  fn?: string;
  primary_email?: string;
  primary_phone?: string;
  birthday?: string;
  org?: string;
  photo?: string;
  photo_thumbnail?: string;
  archived?: boolean;
  snippet?: string;
}

export interface SearchNoteHit {
  id: number;
  content: string;
  date: string;
  contact_id?: number | null;
  contact_name?: string;
  snippet?: string;
}

export interface SearchActivityHit {
  id: number;
  title: string;
  description?: string;
  location?: string;
  date: string;
  type?: string;
  uuid?: string;
  snippet?: string;
}

export interface SearchResult {
  query: string;
  // Canonical relation token when the whole query is a registry synonym
  // ("brother" -> sibling_of).
  resolved_relation?: string;
  contacts: SearchContactHit[];
  notes: SearchNoteHit[];
  activities: SearchActivityHit[];
}

export async function searchAll(query: string, limit?: number): Promise<SearchResult> {
  const queryParams = new URLSearchParams({ q: query });
  if (limit != null) queryParams.append('limit', String(limit));
  const response = await apiFetch(`${API_BASE_URL}/search?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function rebuildSearchIndex(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/admin/search/rebuild`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
