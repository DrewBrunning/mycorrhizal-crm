// M3 dashboard composite — one call replacing the
// birthdays/random-contacts/upcoming-reminders/overdue-cadences Promise.all
// DashboardPage used to fire, plus its per-reminder contact-name N+1.

import type { OverdueCadence } from './cadencePolicies';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import type { ContactSyncConflict } from './contactSyncConflicts';
import type { Birthday, Contact } from './contacts';
import type { OverdueDataDecayPolicy } from './dataDecayPolicies';
import type { ReachOutSuggestion } from './reachOutSuggestions';
import type { Reminder } from './reminders';

// A Reminder enriched with its contact's display name (nickname-preferred,
// falling back to firstname+lastname) so the dashboard never needs a second
// per-reminder fetch.
export interface DashboardReminder extends Reminder {
  contact_name: string;
}

export interface DashboardResponse {
  birthdays: Birthday[];
  random_contacts: Contact[];
  upcoming_reminders: DashboardReminder[];
  overdue: OverdueCadence[];
  favorites: Contact[];
  // Issue #177: pending event-driven reach-out suggestions.
  reach_out_suggestions: ReachOutSuggestion[];
  // Issue #395: pending CardDAV sync conflicts (overwritten local edits).
  contact_sync_conflicts: ContactSyncConflict[];
  // Issue #352: contacts whose info is due for re-verification.
  data_decay_overdue: OverdueDataDecayPolicy[];
  // Issue #1791: every list block above is capped server-side at 25 rows;
  // these are each block's size before the cap ("showing 25 of N").
  birthdays_total: number;
  upcoming_reminders_total: number;
  overdue_total: number;
  reach_out_suggestions_total: number;
  contact_sync_conflicts_total: number;
  data_decay_overdue_total: number;
}

export async function getDashboard(): Promise<DashboardResponse> {
  const response = await apiFetch(`${API_BASE_URL}/dashboard`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
