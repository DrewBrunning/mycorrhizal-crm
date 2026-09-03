import { API_BASE_URL, apiFetch, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';

// CardDAV contact subscriptions. Mirrors api/calendars.ts (the CalDAV analog),
// minus the past_days/future_days window fields, which have no contacts analog.

export interface ContactSubscription {
  id: number;
  name: string;
  url: string;
  username: string;
  has_password: boolean;
  sync_enabled: boolean;
  last_synced_at: string | null;
  last_sync_status: '' | 'success' | 'error';
  last_sync_error: string;
  created_at: string;
  // Sync-health last-known-good state (issue #390).
  last_attempt_at: string | null;
  last_success_at: string | null;
  last_failure_at: string | null;
  consecutive_failures: number;
  incident_first_failure_at: string | null;
  last_run_duration_ms: number | null;
  last_run_stats: Record<string, number>;
  // Terminal (permanent-until-human) failure state (INT-04, issue #467).
  terminal_failure_at: string | null;
  terminal_reason: string;
  // Unreviewed local edits overwritten by sync (issue #395).
  pending_conflicts: number;
}

export interface ContactSubscriptionInput {
  name: string;
  url: string;
  username?: string;
  password?: string;
  clear_password?: boolean;
  sync_enabled?: boolean;
}

export interface ContactSyncResult {
  message: string;
  created: number;
  updated: number;
  archived: number;
  skipped: number;
}

export async function getContactSubscriptions(): Promise<ContactSubscription[]> {
  const response = await apiFetch(`${API_BASE_URL}/contact-subscriptions`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load contact subscriptions.');
  return data?.contact_subscriptions || [];
}

export async function createContactSubscription(
  input: ContactSubscriptionInput,
): Promise<ContactSubscription> {
  const response = await apiFetch(`${API_BASE_URL}/contact-subscriptions`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  return handleResponse(
    response,
    'Unable to add contact subscription.',
  ) as Promise<ContactSubscription>;
}

export async function updateContactSubscription(
  id: number,
  input: ContactSubscriptionInput,
): Promise<ContactSubscription> {
  const response = await apiFetch(`${API_BASE_URL}/contact-subscriptions/${id}`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  return handleResponse(
    response,
    'Unable to update contact subscription.',
  ) as Promise<ContactSubscription>;
}

export async function deleteContactSubscription(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contact-subscriptions/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to delete contact subscription.');
}

export async function syncContactSubscription(id: number): Promise<ContactSyncResult> {
  const response = await apiFetch(
    `${API_BASE_URL}/contact-subscriptions/${id}/sync`,
    {
      method: 'POST',
      headers: getAuthHeaders(),
    },
    120000,
  );
  return handleResponse(
    response,
    'Unable to sync contact subscription.',
  ) as Promise<ContactSyncResult>;
}
