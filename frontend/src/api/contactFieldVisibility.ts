// Per-contact field visibility (issue #1997): the caller's cosmetic overrides
// of which extended contact fields the UI shows for ONE contact, plus the
// hide-empty mode. Stored outside the neutral Card, so it is never part of a
// contact record, export or sync; writing it does not change the contact's
// revision, so none of these calls send If-Match.
import type { ContactFieldKey } from '../contactFields';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

/** A stored per-contact override. "inherit" is the absence of an override. */
export type FieldOverrideState = 'show' | 'hide';
/** What the editor sends: inherit clears the override. */
export type FieldStateInput = 'inherit' | FieldOverrideState;

export interface ResolvedFieldVisibility {
  /** Every known field key -> whether it is shown. */
  fields: Record<string, boolean>;
  /** Whether empty fields are suppressed in the detail view. */
  hide_empty: boolean;
}

export interface ContactFieldVisibility {
  /** Only the explicit per-contact overrides; inherited fields are absent. */
  overrides: Record<string, FieldOverrideState>;
  /** The raw per-contact hide-empty override ('hide' = hide empty fields). */
  hide_empty: FieldStateInput;
  /** The account-level decision the contact inherits. */
  defaults: ResolvedFieldVisibility;
  /** The decision to render from (overrides applied on top of defaults). */
  resolved: ResolvedFieldVisibility;
}

/** The PUT body: the COMPLETE override set (full replace). */
export interface ContactFieldVisibilityInput {
  overrides?: Partial<Record<ContactFieldKey, FieldStateInput>>;
  hide_empty?: FieldStateInput;
}

const path = (contactId: string | number) =>
  `${API_BASE_URL}/contacts/${contactId}/field-visibility`;

export async function getContactFieldVisibility(
  contactId: string | number,
): Promise<ContactFieldVisibility> {
  const response = await apiFetch(path(contactId), { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return (await response.json()) as ContactFieldVisibility;
}

export async function putContactFieldVisibility(
  contactId: string | number,
  input: ContactFieldVisibilityInput,
): Promise<ContactFieldVisibility> {
  const response = await apiFetch(path(contactId), {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return (await response.json()) as ContactFieldVisibility;
}

/** Removes every override: the contact inherits the account defaults again. */
export async function resetContactFieldVisibility(
  contactId: string | number,
): Promise<ContactFieldVisibility> {
  const response = await apiFetch(path(contactId), {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return (await response.json()) as ContactFieldVisibility;
}
