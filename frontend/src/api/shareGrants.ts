// Standing-share (ShareGrant) API calls -- ADR 0042, issue #2292. A grant is a
// revocable, read-only, live authorization from one user to another on the
// same instance. This is NOT the one-time ContactShare flow (api/contactShares.ts),
// which is a frozen copy and stays untouched.
//
// Every response is Cache-Control: no-store server-side; nothing here caches.
import {
  API_BASE_URL,
  apiFetch,
  getAuthHeaders,
  getConditionalHeaders,
  parseErrorResponse,
} from './client';

// Hand-maintained mirrors of the backend's closed sets (frontend trap #4):
// backend/models/share_grant.go entity types and statuses.
export const SHARE_GRANT_ENTITY_TYPES = ['contact', 'note', 'activity', 'circle'] as const;
export type ShareGrantEntityType = (typeof SHARE_GRANT_ENTITY_TYPES)[number];

export type ShareGrantStatus = 'pending' | 'active' | 'declined' | 'revoked' | 'expired';

export type ShareGrantRevokedReason = 'owner_revoked' | 'entity_deleted' | 'recipient_deleted';

export interface ShareGrantInput {
  to_user_id: number;
  entity_type: ShareGrantEntityType;
  entity_id: string;
  // Contact grants only: a non-empty, explicit list (there is no "all" default).
  sections?: string[];
  expires_at?: string | null;
}

export interface ShareGrantPatchInput {
  expires_at?: string | null;
  sections?: string[];
}

// A grant as its OWNER sees it ("Shared by me").
export interface ShareGrantOwnerView {
  id: string;
  to_user_id: number;
  to_username: string;
  entity_type: ShareGrantEntityType;
  entity_id: string;
  label: string;
  sections: string[];
  reconfirm_sections: string[];
  status: ShareGrantStatus;
  expires_at: string | null;
  responded_at: string | null;
  revoked_at: string | null;
  revoked_reason: ShareGrantRevokedReason | null;
  // Circle grants only: the live member count (ADR 0042 decision 9).
  member_count: number | null;
  created_at: string;
  updated_at: string;
  revision: number;
}

// A grant as its RECIPIENT sees it ("Shared with me"). Never carries the
// entity id; a pending grant exposes only sender, type and label.
export interface ShareGrantRecipientView {
  id: string;
  from_username: string;
  entity_type: ShareGrantEntityType;
  label: string;
  sections: string[];
  status: ShareGrantStatus;
  expires_at: string | null;
  responded_at: string | null;
  created_at: string;
}

export interface ShareGrantPage<T> {
  share_grants: T[];
  next_cursor: string;
  limit: number;
}

export interface ShareGrantContent {
  grant_id: string;
  entity_type: ShareGrantEntityType;
  owner_username: string;
  // The neutral Card serialized as a JSContact object, filtered to the granted sections.
  contact?: JSContactCard;
  note?: { content: string; date: string };
  activity?: {
    title: string;
    description: string;
    location: string;
    date: string;
    end_date: string | null;
    type: string;
  };
  circle?: {
    name: string;
    description: string;
    color: string;
    member_count: number;
    members: { display_name: string }[];
    members_truncated: boolean;
  };
}

// An RFC 9553 JSContact Card. Only displayed, never edited, so it stays loose.
export type JSContactCard = Record<string, unknown>;

export interface ShareBlock {
  user_id: number;
  username: string;
  created_at: string;
}

export interface ShareGrantNotification {
  id: number;
  kind: 'pending' | 'revoked';
  entity_type: ShareGrantEntityType;
  sender_username: string;
  created_at: string;
}

export interface ShareGrantNotifications {
  pending_count: number;
  notifications: ShareGrantNotification[];
}

interface PageParams {
  cursor?: string;
  limit?: number;
}

async function readJSON<T>(response: Response): Promise<T> {
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json() as Promise<T>;
}

export async function createShareGrant(
  input: ShareGrantInput,
  idempotencyKey?: string,
): Promise<ShareGrantOwnerView> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants`, {
    method: 'POST',
    headers: {
      ...getAuthHeaders(),
      ...(idempotencyKey ? { 'Idempotency-Key': idempotencyKey } : {}),
    },
    body: JSON.stringify(input),
  });
  return (await readJSON<{ share_grant: ShareGrantOwnerView }>(response)).share_grant;
}

async function listGrants<T>(
  direction: 'outgoing' | 'incoming',
  params: PageParams,
): Promise<ShareGrantPage<T>> {
  const query = new URLSearchParams();
  if (params.cursor) query.set('cursor', params.cursor);
  if (params.limit) query.set('limit', String(params.limit));
  const qs = query.toString();
  const response = await apiFetch(
    `${API_BASE_URL}/share-grants/${direction}${qs ? `?${qs}` : ''}`,
    {
      headers: getAuthHeaders(),
    },
  );
  const body = await readJSON<Partial<ShareGrantPage<T>>>(response);
  return {
    share_grants: body.share_grants ?? [],
    next_cursor: body.next_cursor ?? '',
    limit: body.limit ?? 0,
  };
}

export function listOutgoingShareGrants(
  params: PageParams = {},
): Promise<ShareGrantPage<ShareGrantOwnerView>> {
  return listGrants<ShareGrantOwnerView>('outgoing', params);
}

export function listIncomingShareGrants(
  params: PageParams = {},
): Promise<ShareGrantPage<ShareGrantRecipientView>> {
  return listGrants<ShareGrantRecipientView>('incoming', params);
}

// Owner: only expires_at and a NARROWING sections list are accepted; widening
// is a 409 (create a new grant). `revision` is echoed as If-Match (ADR 0008).
export async function updateShareGrant(
  id: string,
  patch: ShareGrantPatchInput,
  revision?: number,
): Promise<ShareGrantOwnerView> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/${id}`, {
    method: 'PATCH',
    headers: getConditionalHeaders(revision),
    body: JSON.stringify(patch),
  });
  return (await readJSON<{ share_grant: ShareGrantOwnerView }>(response)).share_grant;
}

async function recipientAction(
  id: string,
  action: 'accept' | 'leave' | 'decline',
  body?: unknown,
): Promise<ShareGrantRecipientView> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/${id}/${action}`, {
    method: 'POST',
    headers: getAuthHeaders(),
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  return (await readJSON<{ share_grant: ShareGrantRecipientView }>(response)).share_grant;
}

async function ownerAction(
  id: string,
  action: 'revoke' | 'reconfirm',
): Promise<ShareGrantOwnerView> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/${id}/${action}`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  return (await readJSON<{ share_grant: ShareGrantOwnerView }>(response)).share_grant;
}

export const acceptShareGrant = (id: string) => recipientAction(id, 'accept');
export const leaveShareGrant = (id: string) => recipientAction(id, 'leave');
// Declining with block:true also blocks the sender and ends every pending or
// active grant from them. The backend body is optional; we always send it so
// the intent is explicit.
export const declineShareGrant = (id: string, opts: { block?: boolean } = {}) =>
  recipientAction(id, 'decline', { block: opts.block === true });
export const revokeShareGrant = (id: string) => ownerAction(id, 'revoke');
export const reconfirmShareGrant = (id: string) => ownerAction(id, 'reconfirm');

export async function getShareGrantContent(id: string): Promise<ShareGrantContent> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/${id}/content`, {
    headers: getAuthHeaders(),
  });
  return (await readJSON<{ share_grant_content: ShareGrantContent }>(response)).share_grant_content;
}

export async function listShareBlocks(): Promise<ShareBlock[]> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/blocks`, {
    headers: getAuthHeaders(),
  });
  return (await readJSON<{ blocks?: ShareBlock[] }>(response)).blocks ?? [];
}

export async function deleteShareBlock(userId: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/blocks/${userId}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

export async function getShareGrantNotifications(): Promise<ShareGrantNotifications> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/notifications`, {
    headers: getAuthHeaders(),
  });
  const body = await readJSON<Partial<ShareGrantNotifications>>(response);
  return { pending_count: body.pending_count ?? 0, notifications: body.notifications ?? [] };
}

export async function markShareGrantNotificationsRead(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/share-grants/notifications/read`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// Preview of the exact grant projection for a contact (ADR 0042 decision 14):
// the single-contact JSContact export is built by the same
// RecordForContactFiltered + ApplyFieldSelection path the grant read uses, with
// include_sensitive off. An empty selection is refused HERE because the export
// treats "no sections" as ALL sections.
export async function getShareJSContactPreview(
  vcardUID: string,
  sections: string[],
): Promise<JSContactCard | null> {
  if (sections.length === 0) throw new Error('A share preview needs at least one section');
  const query = new URLSearchParams({ vcard_uid: vcardUID, sections: sections.join(',') });
  const response = await apiFetch(`${API_BASE_URL}/export/jscontact?${query.toString()}`, {
    headers: getAuthHeaders(),
  });
  const cards = await readJSON<JSContactCard[]>(response);
  return cards[0] ?? null;
}
