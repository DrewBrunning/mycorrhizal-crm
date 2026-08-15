// Notification channel API calls — N9: per-user ntfy/Gotify config, the per-user
// channel toggles, per-channel test notifications, and Web Push device
// registrations. The channels listed here must stay in sync with the backend
// models.AllNotificationChannels (no dynamic type-list endpoint exists).
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export type NotificationChannel = 'email' | 'ntfy' | 'gotify' | 'push';

export interface NotificationConfig {
  ntfy_url: string;
  ntfy_topic: string;
  gotify_url: string;
  gotify_has_token: boolean;
  notify_ntfy: boolean;
  notify_gotify: boolean;
  notify_push: boolean;
  vapid_public_key: string;
}

export interface NotificationConfigInput {
  ntfy_url?: string;
  ntfy_topic?: string;
  gotify_url?: string;
  // Write-only: empty on update keeps the stored token.
  gotify_token?: string;
  notify_ntfy?: boolean;
  notify_gotify?: boolean;
  notify_push?: boolean;
}

export interface PushSubscription {
  id: number;
  endpoint: string;
  p256dh: string;
  auth: string;
  device_label: string;
  created_at: string;
}

export interface PushSubscriptionInput {
  endpoint: string;
  p256dh: string;
  auth: string;
  device_label?: string;
}

export interface NotificationTestResult {
  ok: boolean;
  error?: string;
}

// DeviceRegistration is a mobile push device token registered by a native
// client (M2, M2) — the FCM/APNS
// counterpart to PushSubscription's Web Push (VAPID) shape. The web app never
// registers a device; it only lists/deletes the ones the Android/iOS app
// registered.
export interface DeviceRegistration {
  id: number;
  token: string;
  client: 'fcm' | 'apns';
  device_label: string;
  created_at: string;
  updated_at: string;
}

export async function getNotificationConfig(): Promise<NotificationConfig> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/config`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function saveNotificationConfig(input: NotificationConfigInput): Promise<NotificationConfig> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/config`, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Sends a test notification through the given channel using the user's saved
// config. A diagnosed failure (unconfigured, unreachable, private address
// blocked) is still a 200 with ok:false.
export async function testNotificationChannel(channel: NotificationChannel): Promise<NotificationTestResult> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/config/test`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ channel }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function getPushSubscriptions(): Promise<PushSubscription[]> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/push-subscriptions`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.subscriptions || [];
}

export async function createPushSubscription(input: PushSubscriptionInput): Promise<PushSubscription> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/push-subscriptions`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function deletePushSubscription(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/push-subscriptions/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// Mobile device registrations (M2). No create here — enrollment happens on
// the native client after it obtains a platform push token; the web app only
// views and deletes.
export async function getDeviceRegistrations(): Promise<DeviceRegistration[]> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/devices`, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.devices || [];
}

export async function deleteDeviceRegistration(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/notifications/devices/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
