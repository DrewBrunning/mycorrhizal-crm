// Per-channel notification delivery health API — issue #422, the frontend
// half of GET /admin/notification-health. Admin-only, read-only, instance-wide.
// The backend derives this on read by folding notification_deliveries + the
// per-user channel config; there is nothing to write.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// The notification channels, mirroring backend/models/notification.go's
// AllNotificationChannels and backend/openapi.yaml's
// NotificationChannelHealth.channel enum EXACTLY. No dynamic type-list
// endpoint exists — this is a hand-maintained mirror of the backend
// (CLAUDE.md frontend trap #4). Keep the three in sync, and in this order (the
// API preserves it).
export type NotificationChannelName = 'email' | 'ntfy' | 'gotify' | 'push';

export const NOTIFICATION_CHANNELS: NotificationChannelName[] = [
  'email',
  'ntfy',
  'gotify',
  'push',
];

// unconfigured = nothing to deliver on; no_devices = push provisioned but no
// browser subscription / mobile device can receive; healthy = configured and
// the last terminal delivery succeeded; failing = configured and the last
// terminal delivery failed.
export type NotificationChannelStatus =
  | 'unconfigured'
  | 'no_devices'
  | 'healthy'
  | 'failing';

export interface NotificationChannelHealth {
  channel: NotificationChannelName;
  status: NotificationChannelStatus;
  configured: boolean;
  // True exactly when status is healthy — the other statuses are each a
  // different reason the channel cannot deliver right now.
  reachable: boolean;
  enabled_user_count: number;
  // Push only: currently-receivable endpoints (web subscriptions + FCM
  // devices when FCM is configured); 0 for non-push channels.
  device_count: number;
  // Push only: the server has an FCM service account file (M2).
  fcm_configured: boolean;
  last_attempt_at: string | null;
  last_sent_at: string | null;
  last_failed_at: string | null;
  // Unbroken failed-delivery run since the last success; non-zero exactly
  // when status is failing.
  consecutive_failures: number;
  // Length-capped error of the most recent failed delivery; empty unless
  // status is failing.
  last_error: string;
  attempted_count: number;
  delivered_count: number;
}

export interface NotificationChannelHealthResponse {
  channels: NotificationChannelHealth[];
}

// GET /admin/notification-health — one entry per delivery channel, in
// NOTIFICATION_CHANNELS order.
export async function getNotificationChannelHealth(): Promise<NotificationChannelHealthResponse> {
  const response = await apiFetch(`${API_BASE_URL}/admin/notification-health`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
