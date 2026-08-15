// M4 contact-detail composite (M4): everything ContactDetailPage.tsx
// renders for one contact, in one call, replacing the ~21-endpoint fan-out.
//
// This module is a standalone api surface for the composite's OpenAPI
// contract -- the Android client's target (M1 §4.2 ContactDetailScreen).
// ContactDetailPage.tsx itself is NOT rewired onto this endpoint (its
// incremental per-hook loading is deliberate web UX); that's a separate,
// later ticket if it happens at all.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { ContactRecordResponse } from './contacts';
import { Note } from './notes';
import { Activity } from './activities';
import { Reminder, ReminderCompletion } from './reminders';
import { BriefingRelationship } from './briefings';
import { LifeEvent } from './lifeEvents';
import { ConversationAgenda } from './conversationAgenda';
import { Gift } from './gifts';
import { FieldValue } from './fieldDefinitions';
import { ExternalIdentity, ExternalActivity } from './externalLinks';
import { Circle } from './circles';
import { Tag } from './tags';

export interface ContactDetailUser {
  enabled_contact_fields: string[];
}

// A LifeEvent enriched with its related_entity_ids' display names,
// batch-resolved server-side once per contact (never per event).
export interface ContactDetailLifeEvent extends LifeEvent {
  related_entity_names: Record<string, string>;
}

export interface ImmichPersonSummary {
  identity: ExternalIdentity;
  person_name: string;
  photo_count: number;
  latest_asset_id?: string;
  latest_at?: string | null;
}

// Present only when the requesting user has an Immich config at all --
// independent of whether this particular contact is linked to an Immich
// person yet (summary is null in that case).
export interface ContactDetailImmich {
  summary: ImmichPersonSummary | null;
}

export interface ContactDetailResponse {
  contact: ContactRecordResponse;
  user: ContactDetailUser;
  notes: Note[];
  activities: Activity[];
  completions: ReminderCompletion[];
  reminders: Reminder[];
  relationship_edges: BriefingRelationship[];
  life_events: ContactDetailLifeEvent[];
  agenda: ConversationAgenda[];
  gifts: Gift[];
  field_values: FieldValue[];
  external_identities: ExternalIdentity[];
  external_activities: ExternalActivity[];
  // This contact's circle memberships and tags -- not the global per-user
  // lists getCircles()/getTags() return.
  circles: Circle[];
  tags: Tag[];
  // Absent (not just null) when the user has no Immich config at all.
  immich?: ContactDetailImmich;
}

// GET /contacts/:id/detail — the M4 contact-detail composite. Every
// collection block is guaranteed `[]` (never absent) by the server contract
// (backend/models/contact_detail.go); normalized here anyway at the api
// boundary rather than trusted, the same defensive posture api/briefings.ts
// takes after the prep view's own regression (CLAUDE.md frontend trap 8).
export async function getContactDetail(id: string | number): Promise<ContactDetailResponse> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}/detail`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  const raw = await response.json();
  return {
    ...raw,
    notes: raw.notes ?? [],
    activities: raw.activities ?? [],
    completions: raw.completions ?? [],
    reminders: raw.reminders ?? [],
    relationship_edges: raw.relationship_edges ?? [],
    life_events: raw.life_events ?? [],
    agenda: raw.agenda ?? [],
    gifts: raw.gifts ?? [],
    field_values: raw.field_values ?? [],
    external_identities: raw.external_identities ?? [],
    external_activities: raw.external_activities ?? [],
    circles: raw.circles ?? [],
    tags: raw.tags ?? [],
  };
}
