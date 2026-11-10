// Reusable entities (organisations, places, pets) -- issue #2301, ADR 0043.
//
// Hand-written wire types pinned to src/generated/openapi.ts by
// contractConformance.ts. The kind, role and sensitivity lists are hardcoded
// mirrors of the backend vocabularies (frontend trap #4): keep them in sync
// with backend/models/entity.go (EntityKind*) and
// backend/models/contact_entity_link.go (entityLinkRoles).
import {
  API_BASE_URL,
  apiFetch,
  getAuthHeaders,
  getConditionalHeaders,
  parseErrorResponse,
} from './client';
import type { FieldValue, FieldValueInput } from './fieldDefinitions';
import type { PartialDate } from './lifeEvents';

export type EntityKind = 'organisation' | 'place' | 'pet';
export const ENTITY_KINDS: EntityKind[] = ['organisation', 'place', 'pet'];

export type EntitySensitivity = 'normal' | 'private' | 'secret';
export const ENTITY_SENSITIVITIES: EntitySensitivity[] = ['normal', 'private', 'secret'];

// Closed role vocabulary per kind (ADR 0043 decision 2).
export const ENTITY_LINK_ROLES: Record<EntityKind, string[]> = {
  organisation: ['employee', 'owner', 'contractor', 'volunteer', 'member', 'other'],
  place: ['resident', 'owner', 'visitor', 'other'],
  pet: ['owner', 'carer', 'other'],
};

export interface TemporalRange {
  start?: PartialDate;
  end?: PartialDate;
}

export interface Entity {
  id: string;
  kind: EntityKind;
  name: string;
  // Kind-specific JSON (OrganisationCore / PlaceCore / PetCore); see the entity
  // form for the shapes the UI reads and writes.
  core: Record<string, unknown>;
  sensitivity: EntitySensitivity;
  revision: number;
  // Only on GET /entities/{id}.
  link_count?: number;
  projection_count?: number;
  created_at: string;
  updated_at: string;
}

export interface EntityInput {
  kind?: EntityKind;
  name: string;
  core?: Record<string, unknown>;
  sensitivity?: EntitySensitivity;
}

export interface EntityListResponse {
  entities: Entity[];
  limit: number;
  next_cursor: string;
}

export interface EntityDeleteResponse {
  message: string;
  link_count: number;
  projection_count: number;
}

export interface EntityRestoreSkip {
  table: string;
  reason: string;
  count: number;
}

export interface EntityRestoreResponse {
  entity: Entity;
  links_revived: number;
  same_name_live: boolean;
  merged_into?: string;
  skipped: EntityRestoreSkip[];
}

export interface EntityContactLink {
  link_id: string;
  contact_id: number;
  contact_vcard_uid: string;
  firstname: string;
  lastname: string;
  role: string;
  role_label: string;
  is_primary: boolean;
  period?: TemporalRange;
  sensitivity: EntitySensitivity;
}

export interface EntityMergeInput {
  into: string;
  // field name -> which side's value wins for a field the survivor leaves empty.
  resolutions?: Record<string, 'loser' | 'survivor'>;
}

export interface EntityMergeDemotion {
  link_id: string;
  contact_vcard_uid: string;
}

export interface EntityMergeResponse {
  survivor: Entity;
  loser_id: string;
  links_repointed: number;
  links_collapsed: number;
  primaries_demoted: EntityMergeDemotion[];
  field_values_moved: number;
  field_values_dropped: string[];
  resolved_fields: string[];
  restore_scope: 'entity_only';
}

export type EntitySuggestionReason = 'exact_name' | 'normalized_name' | 'address_components';

export interface EntitySuggestionEntity {
  id: string;
  kind: EntityKind;
  name: string;
}

export interface EntitySuggestionPair {
  a: EntitySuggestionEntity;
  b: EntitySuggestionEntity;
  reason: EntitySuggestionReason;
}

export interface EntityValueGroupContact {
  contact_id: number;
  firstname: string;
  lastname: string;
  value_name: string;
  has_primary: boolean;
}

export interface EntityValueGroup {
  key: string;
  kind: 'organisation' | 'place';
  name: string;
  reason: EntitySuggestionReason;
  variants: string[];
  contacts: EntityValueGroupContact[];
}

export interface EntitySuggestionsResponse {
  mode: 'pairs' | 'contact_values';
  pairs: EntitySuggestionPair[];
  groups: EntityValueGroup[];
  limit: number;
  next_cursor: string;
}

export interface EntityFromContactsGroup {
  key: string;
  name?: string;
  role: string;
  set_primary?: boolean;
  contact_ids: number[];
}

export interface EntityFromContactsInput {
  kind: 'organisation' | 'place';
  groups: EntityFromContactsGroup[];
}

export interface EntityFromContactsSkip {
  key: string;
  contact_id?: number;
  reason: 'group_not_found' | 'contact_not_in_group' | 'name_invalid' | 'core_invalid';
}

export interface EntityFromContactsResponse {
  created: { key: string; entity: Entity; links: ContactEntityLink[] }[];
  skipped: EntityFromContactsSkip[];
}

export interface ContactEntityLink {
  id: string;
  contact_vcard_uid: string;
  entity_id: string;
  entity_name: string;
  kind: EntityKind;
  role: string;
  role_label: string;
  is_primary: boolean;
  period?: TemporalRange;
  sensitivity: EntitySensitivity;
  // current: the period includes today. projects_as: which card property the
  // link currently projects into ('' when it does not project).
  current: boolean;
  projects_as: '' | 'ORG' | 'ADR';
  revision: number;
  created_at: string;
  updated_at: string;
}

export interface ContactEntityLinkInput {
  entity_id?: string;
  role: string;
  role_label?: string;
  is_primary?: boolean;
  period?: TemporalRange;
  sensitivity?: EntitySensitivity;
}

export interface ContactEntityLinkFromExistingInput extends ContactEntityLinkInput {
  source: { kind: 'organization' | 'address'; id?: string; index?: number };
}

export interface ContactEntityLinkFromExistingResponse {
  entity: Entity;
  entity_created: boolean;
  link: ContactEntityLink;
}

export interface ContactEntityLinkAdoptResponse {
  adopted: boolean;
  element_id: string;
  element_kind: 'organization' | 'address';
  link: ContactEntityLink;
}

// ADR 0043 decision 3: a projected ORG/ADR entry in a contact record carries
// the reserved element id `ent-<linkId>`. The UI never lets such an entry be
// edited or deleted as the contact's own value, and never writes it back.
export const PROJECTED_ELEMENT_ID_PREFIX = 'ent-';

export function isProjectedElementId(id: string | undefined | null): boolean {
  return !!id && id.startsWith(PROJECTED_ELEMENT_ID_PREFIX);
}

export function entityLinkIdFromElementId(id: string | undefined | null): string | undefined {
  return isProjectedElementId(id)
    ? (id as string).slice(PROJECTED_ELEMENT_ID_PREFIX.length)
    : undefined;
}

const json = { 'Content-Type': 'application/json' };

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await apiFetch(`${API_BASE_URL}${path}`, init);
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json() as Promise<T>;
}

function query(params: Record<string, string | number | undefined>): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') qs.append(k, String(v));
  }
  const s = qs.toString();
  return s ? `?${s}` : '';
}

export async function listEntities(params: {
  kind?: EntityKind;
  q?: string;
  sensitivity?: EntitySensitivity;
  cursor?: string;
  limit?: number;
}): Promise<EntityListResponse> {
  const res = await request<EntityListResponse>(`/entities${query(params)}`, {
    headers: getAuthHeaders(),
  });
  return { ...res, entities: res.entities ?? [] };
}

export function getEntity(id: string): Promise<Entity> {
  return request<Entity>(`/entities/${encodeURIComponent(id)}`, { headers: getAuthHeaders() });
}

export function createEntity(input: EntityInput): Promise<Entity> {
  return request<Entity>('/entities', {
    method: 'POST',
    headers: { ...getAuthHeaders(), ...json },
    body: JSON.stringify(input),
  });
}

export function updateEntity(id: string, input: EntityInput, revision?: number): Promise<Entity> {
  return request<Entity>(`/entities/${encodeURIComponent(id)}`, {
    method: 'PUT',
    headers: getConditionalHeaders(revision),
    body: JSON.stringify(input),
  });
}

export function deleteEntity(id: string, revision?: number): Promise<EntityDeleteResponse> {
  return request<EntityDeleteResponse>(`/entities/${encodeURIComponent(id)}`, {
    method: 'DELETE',
    headers: getConditionalHeaders(revision),
  });
}

export function restoreEntity(id: string): Promise<EntityRestoreResponse> {
  return request<EntityRestoreResponse>(`/entities/${encodeURIComponent(id)}/restore`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
}

export function mergeEntity(
  loserId: string,
  input: EntityMergeInput,
): Promise<EntityMergeResponse> {
  return request<EntityMergeResponse>(`/entities/${encodeURIComponent(loserId)}/merge`, {
    method: 'POST',
    headers: { ...getAuthHeaders(), ...json },
    body: JSON.stringify(input),
  });
}

export async function listEntitySuggestions(params: {
  kind?: EntityKind;
  mode?: 'pairs' | 'contact_values';
  cursor?: string;
  limit?: number;
}): Promise<EntitySuggestionsResponse> {
  const res = await request<EntitySuggestionsResponse>(`/entities/suggestions${query(params)}`, {
    headers: getAuthHeaders(),
  });
  return { ...res, pairs: res.pairs ?? [], groups: res.groups ?? [] };
}

export function dismissEntitySuggestion(entityA: string, entityB: string) {
  return request<{ id_low: string; id_high: string }>('/entities/suggestions/dismiss', {
    method: 'POST',
    headers: { ...getAuthHeaders(), ...json },
    body: JSON.stringify({ entity_a: entityA, entity_b: entityB }),
  });
}

export function createEntitiesFromContacts(
  input: EntityFromContactsInput,
): Promise<EntityFromContactsResponse> {
  return request<EntityFromContactsResponse>('/entities/from-contacts', {
    method: 'POST',
    headers: { ...getAuthHeaders(), ...json },
    body: JSON.stringify(input),
  });
}

export async function listEntityContacts(id: string): Promise<EntityContactLink[]> {
  const res = await request<{ contacts: EntityContactLink[] | null }>(
    `/entities/${encodeURIComponent(id)}/contacts`,
    { headers: getAuthHeaders() },
  );
  return res.contacts ?? [];
}

export async function listEntityFieldValues(id: string): Promise<FieldValue[]> {
  const res = await request<{ field_values: FieldValue[] | null }>(
    `/entities/${encodeURIComponent(id)}/field-values`,
    { headers: getAuthHeaders() },
  );
  return res.field_values ?? [];
}

// Full replace, like the contact route: values for definitions not sent are deleted.
export async function replaceEntityFieldValues(
  id: string,
  fieldValues: FieldValueInput[],
): Promise<FieldValue[]> {
  const res = await request<{ field_values: FieldValue[] | null }>(
    `/entities/${encodeURIComponent(id)}/field-values`,
    {
      method: 'PUT',
      headers: { ...getAuthHeaders(), ...json },
      body: JSON.stringify({ field_values: fieldValues }),
    },
  );
  return res.field_values ?? [];
}

export async function listContactEntityLinks(
  contactId: number | string,
): Promise<ContactEntityLink[]> {
  const res = await request<{ links: ContactEntityLink[] | null }>(
    `/contacts/${contactId}/entity-links`,
    { headers: getAuthHeaders() },
  );
  return res.links ?? [];
}

export function createContactEntityLink(
  contactId: number | string,
  input: ContactEntityLinkInput,
): Promise<ContactEntityLink> {
  return request<ContactEntityLink>(`/contacts/${contactId}/entity-links`, {
    method: 'POST',
    headers: { ...getAuthHeaders(), ...json },
    body: JSON.stringify(input),
  });
}

export function updateContactEntityLink(
  contactId: number | string,
  linkId: string,
  input: ContactEntityLinkInput,
  revision?: number,
): Promise<ContactEntityLink> {
  return request<ContactEntityLink>(`/contacts/${contactId}/entity-links/${linkId}`, {
    method: 'PUT',
    headers: getConditionalHeaders(revision),
    body: JSON.stringify(input),
  });
}

export async function deleteContactEntityLink(
  contactId: number | string,
  linkId: string,
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/entity-links/${linkId}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export function adoptContactEntityLink(
  contactId: number | string,
  linkId: string,
): Promise<ContactEntityLinkAdoptResponse> {
  return request<ContactEntityLinkAdoptResponse>(
    `/contacts/${contactId}/entity-links/${linkId}/adopt`,
    { method: 'POST', headers: getAuthHeaders() },
  );
}

export function linkContactEntityFromExisting(
  contactId: number | string,
  input: ContactEntityLinkFromExistingInput,
): Promise<ContactEntityLinkFromExistingResponse> {
  return request<ContactEntityLinkFromExistingResponse>(
    `/contacts/${contactId}/entity-links/from-existing`,
    {
      method: 'POST',
      headers: { ...getAuthHeaders(), ...json },
      body: JSON.stringify(input),
    },
  );
}
