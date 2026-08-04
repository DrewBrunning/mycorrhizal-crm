// ContactBriefing API — N2 (docs/fork-plan/tickets/22-N2-prep-view.md):
// the read-only "prep view" composition for a contact. One endpoint returns
// everything the user wants to remember before seeing a person, assembled
// server-side from existing data (activities, notes, cadence health, agenda
// items, relationship edges, life events, reminders, upcoming dates). Every
// block degrades to empty when its source is absent.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { RelationshipEdge } from './relationshipEdges';
import { LifeEvent } from './lifeEvents';
import { ConversationAgenda } from './conversationAgenda';
import { Note } from './notes';
import { Reminder } from './reminders';
import { CadencePolicy } from './cadencePolicies';

// Mirrors backend/services/cadence_service.go's CadenceHealth + the briefing's
// BriefingCadenceHealth wire shape.
export interface BriefingCadenceHealth {
  has_qualifying_interaction: boolean;
  last_interaction?: string | null;
  next_due?: string | null;
  overdue_by: number;
}

export interface BriefingCadence {
  policy: CadencePolicy;
  health: BriefingCadenceHealth;
}

export interface BriefingActivity {
  id: number;
  uuid?: string;
  title: string;
  description?: string;
  type?: string;
  location?: string;
  date: string;
}

export interface BriefingRelationship {
  edge: RelationshipEdge;
  other_party_contact_id?: number;
  other_party_name?: string;
  other_party_uid?: string;
  display_token?: string;
}

export interface BriefingUpcomingDate {
  label: 'birthday' | 'anniversary';
  date: string;
  days_until: number;
}

export interface ContactBriefing {
  contact_id: number;
  uid: string;
  name: string;
  photo_thumbnail?: string;
  kind?: string;
  last_activity?: BriefingActivity;
  recent_notes: Note[];
  cadence?: BriefingCadence;
  open_agenda_items: ConversationAgenda[];
  relationships: BriefingRelationship[];
  life_events: LifeEvent[];
  upcoming_reminders: Reminder[];
  upcoming_dates: BriefingUpcomingDate[];
}

// GET /contacts/:id/briefing — the N2 prep-view composition.
//
// The six collection blocks are declared required above and PrepViewPage
// dereferences `.length` on them directly, so they are normalised here rather
// than guarded at each of the ~6 render sites. The server contract
// (backend/models/briefing.go) guarantees `[]`, but this endpoint is the one
// place a contract regression white-screens an entire page, so the boundary
// asserts it instead of trusting it: the blocks previously came back *absent*
// for any contact with no history — i.e. every newly-created contact — and the
// page crashed into its ErrorBoundary on first use.
export async function getContactBriefing(id: string | number): Promise<ContactBriefing> {
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${id}/briefing`,
    { headers: getAuthHeaders() }
  );
  if (!response.ok) throw await parseErrorResponse(response);
  const raw = await response.json();
  return {
    ...raw,
    recent_notes: raw.recent_notes ?? [],
    open_agenda_items: raw.open_agenda_items ?? [],
    relationships: raw.relationships ?? [],
    life_events: raw.life_events ?? [],
    upcoming_reminders: raw.upcoming_reminders ?? [],
    upcoming_dates: raw.upcoming_dates ?? [],
  };
}
