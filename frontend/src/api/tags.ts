import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface Tag {
  id: string;
  created_at: string;
  updated_at: string;
  name: string;
}

export interface ContactTag {
  id: number;
  tag_id: string;
  contact_vcard_uid: string;
}

export interface TagWithContacts {
  tag: Tag;
  contacts: ContactTag[];
}

export interface TagCreateResponse {
  message: string;
  tag: Tag;
}

export interface TagListResponse {
  tags: Tag[];
  total: number;
  // T17 cursor pagination: opaque resume token; empty when there are no more rows.
  next_cursor: string;
  limit: number;
  contacts?: ContactTag[];
}

// GET /tags
export async function listTags(params?: {
  cursor?: string;
  limit?: number;
  include_contacts?: boolean;
}): Promise<TagListResponse> {
  const { cursor, limit = 100, include_contacts = false } = params || {};
  const queryParams = new URLSearchParams({
    limit: limit.toString(),
  });
  if (cursor) queryParams.append('cursor', cursor);
  if (include_contacts) queryParams.append('include_contacts', 'true');
  const response = await apiFetch(`${API_BASE_URL}/tags?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /tags
export async function createTag(name: string): Promise<TagCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/tags`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// PUT /tags/:id
export async function updateTag(id: string, name: string): Promise<Tag> {
  const response = await apiFetch(`${API_BASE_URL}/tags/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /tags/:id
export async function deleteTag(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/tags/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// POST /tags/:id/contacts
export async function addContactTag(tagId: string, contactVCardUid: string): Promise<ContactTag> {
  const response = await apiFetch(`${API_BASE_URL}/tags/${tagId}/contacts`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ contact_vcard_uid: contactVCardUid }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// DELETE /tags/:id/contacts/:vcard_uid
export async function removeContactTag(tagId: string, contactVCardUid: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/tags/${tagId}/contacts/${contactVCardUid}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
