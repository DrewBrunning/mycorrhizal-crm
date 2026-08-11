// M3 dashboard composite (docs/fork-plan/tickets/
// 82-M3-dashboard-overview-endpoint.md) — one call replacing the
// birthdays/random-contacts/upcoming-reminders/overdue-cadences Promise.all
// DashboardPage used to fire, plus its per-reminder contact-name N+1.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { Birthday, Contact } from './contacts';
import { Reminder } from './reminders';
import { OverdueCadence } from './cadencePolicies';

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
}

export async function getDashboard(): Promise<DashboardResponse> {
  const response = await apiFetch(`${API_BASE_URL}/dashboard`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
