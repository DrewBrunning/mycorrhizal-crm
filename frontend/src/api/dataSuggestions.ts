// Contact-address suggestion API calls -- the inverse of T40's "suggest
// households from shared address": here the relationship/household already
// exists, and we propose the *address* the other party carries. Read-only
// generate + explicit apply; nothing is written at generate time.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// A neutral AddressComponent pair, mirroring contactmodel.AddressComponent's
// wire shape on the suggestion's address.
export interface AddressComponent {
  kind: string;
  value: string;
}

export interface ContactAddressSuggestion {
  contact_vcard_uid: string;
  contact_name: string;
  source_kind: 'relationship' | 'household';
  source_id: string;
  source_name: string;
  relation_type?: string;
  address: {
    components?: AddressComponent[];
    full?: string;
  };
  // Normalized address identity (street + city/region + postal + country) —
  // the value the apply endpoint re-derives the address from server-side.
  address_key: string;
}

export interface ContactAddressSuggestionsResponse {
  suggestions: ContactAddressSuggestion[];
  total: number;
}

// Renders a suggestion's address as a single display line (street, locality,
// region, postcode, country), falling back to the full text when present.
// Same helper shape as households.ts's formatSuggestionAddress. Components is
// optional because the backend serializes it `omitempty` -- a source address
// with no non-empty parts arrives as `{}` and must render as an empty line,
// not throw.
export function formatSuggestionAddress(address: ContactAddressSuggestion['address']): string {
  if (!address) return '';
  if (address.full) return address.full;
  const byKind: Record<string, string> = {};
  for (const comp of address.components ?? []) {
    if (!(comp.kind in byKind)) byKind[comp.kind] = comp.value;
  }
  const parts = ['name', 'locality', 'region', 'postcode', 'country']
    .map((kind) => byKind[kind])
    .filter((v) => v?.trim());
  return parts.join(', ');
}

// POST /contacts/address-suggestions -- read-only and idempotent: proposes
// addresses for the caller's contacts from confirmed relationships and
// households, skipping any the contact already has.
export async function suggestContactAddresses(): Promise<ContactAddressSuggestionsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/address-suggestions`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /contacts/address-suggestions/apply -- apply one suggestion. The
// server re-derives the address from the current graph, so the client only
// names the suggestion by identity.
export async function applyContactAddressSuggestion(input: {
  contact_vcard_uid: string;
  source_kind: 'relationship' | 'household';
  source_id: string;
  address_key: string;
}): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/address-suggestions/apply`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
