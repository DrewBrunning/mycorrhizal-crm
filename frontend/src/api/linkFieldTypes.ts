// LinkFieldType API calls (T34) — the user-configurable messaging/social
// link-type registry (T34).
// Deliberately NOT a hardcoded oneof mirror like this codebase's other
// enums (contactFields.ts's own frontend-trap-4 convention) — see the
// ticket's "Decisions made" section for why this one is a real CRUD
// registry instead.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export type LinkFieldTypeCategory = 'messaging' | 'social' | 'other';

export interface LinkFieldType {
  id: string;
  name: string;
  // URI template with a literal "{value}" placeholder, e.g.
  // "https://wa.me/{value}". May be empty for a seeded default with no
  // stable public profile-by-handle URL (e.g. Discord) — not resolvable to
  // a link until the user fills it in.
  protocol: string;
  category: LinkFieldTypeCategory;
  icon?: string;
  // True for a seeded row; false for a user addition. Provenance only —
  // editing a default's protocol keeps it flagged as a default.
  is_default: boolean;
  position: number;
  created_at: string;
  updated_at: string;
}

export interface LinkFieldTypeInput {
  name: string;
  protocol: string;
  category: LinkFieldTypeCategory;
  icon?: string;
  // Optional on create (server defaults to end-of-list); always sent on
  // update as a full replace, matching the backend DTO.
  position?: number;
}

export async function getLinkFieldTypes(): Promise<LinkFieldType[]> {
  const response = await apiFetch(`${API_BASE_URL}/link-field-types`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.link_field_types || [];
}

export async function createLinkFieldType(input: LinkFieldTypeInput): Promise<LinkFieldType> {
  const response = await apiFetch(`${API_BASE_URL}/link-field-types`, {
    method: 'POST', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.link_field_type;
}

export async function updateLinkFieldType(id: string, input: LinkFieldTypeInput): Promise<LinkFieldType> {
  const response = await apiFetch(`${API_BASE_URL}/link-field-types/${id}`, {
    method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json(); // raw type, NOT wrapped -- unlike create
}

export async function deleteLinkFieldType(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/link-field-types/${id}`, {
    method: 'DELETE', headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// order is the caller's full set of LinkFieldType IDs in the desired
// display order; the backend rejects anything but a complete, owned,
// duplicate-free set.
export async function reorderLinkFieldTypes(order: string[]): Promise<LinkFieldType[]> {
  const response = await apiFetch(`${API_BASE_URL}/link-field-types/reorder`, {
    method: 'PUT', headers: getAuthHeaders(), body: JSON.stringify({ order }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.link_field_types || [];
}
