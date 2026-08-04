// Notes-related API calls
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface Note {
  ID: number;
  content: string;
  date: string;
  contact_id?: number;
  CreatedAt: string;
  UpdatedAt: string;
}

export interface NotesResponse {
  notes: Note[];
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor?: string;
  limit?: number;
  // Queue depth for the N4 inbox: how many notes match the current filters in
  // total, not how many are on the loaded page. Only the unassigned-notes
  // endpoint returns it — a contact's note history is unbounded, which is why
  // T17 dropped counts there, whereas the unfiled set is a queue being drained.
  // Optional because the contact-scoped list shares this type and omits it.
  total?: number;
}

export interface GetNotesParams {
  cursor?: string;
  limit?: number;
  search?: string;
  fromDate?: string;
  toDate?: string;
}

// Get notes for a contact
export async function getContactNotes(
  contactId: string | number
): Promise<NotesResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${contactId}/notes`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get all unassigned notes
export async function getUnassignedNotes(
  params: GetNotesParams = {}
): Promise<NotesResponse> {
  const { cursor, limit = 25 } = params;
  const search = params.search?.trim();

  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });

  if (cursor) queryParams.append('cursor', cursor);
  if (search) {
    queryParams.append('search', search);
  }

  if (params.fromDate) {
    queryParams.append('fromDate', params.fromDate);
  }

  if (params.toDate) {
    queryParams.append('toDate', params.toDate);
  }

  const response = await apiFetch(
    `${API_BASE_URL}/notes?${queryParams.toString()}`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get single note
export async function getNote(
  id: string | number
): Promise<Note> {
  const response = await apiFetch(
    `${API_BASE_URL}/notes/${id}`,
    { headers: getAuthHeaders() }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Create note for contact
export async function createNote(
  contactId: string | number,
  data: { content: string; date: string; contact_id?: number }
): Promise<Note> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${contactId}/notes`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(data),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Create unassigned note
export async function createUnassignedNote(
  data: { content: string; date: string; contact_id?: number }
): Promise<Note> {
  const response = await apiFetch(
    `${API_BASE_URL}/notes`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
      body: JSON.stringify(data),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Update note
export async function updateNote(
  id: string | number,
  data: { content: string; date: string; contact_id?: number | null }
): Promise<Note> {
  const response = await apiFetch(
    `${API_BASE_URL}/notes/${id}`,
    {
      method: 'PUT',
      headers: getAuthHeaders(),
      body: JSON.stringify(data),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Delete note
export async function deleteNote(
  id: string | number
): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/notes/${id}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
    }
  );

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}
