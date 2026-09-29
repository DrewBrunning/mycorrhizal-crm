// Private Atom feed credentials (issue #1276, ADR 0030). The plaintext
// subscription URL is returned exactly once, by create and rotate.
import { API_BASE_URL, apiFetch, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';

// Mirror backend/models/feed.go's `oneof` validators (frontend trap 4 -- no
// dynamic list endpoint exists). MUST be kept in sync by hand; the
// SameMembers assertions in contractConformance.ts fail `tsc` on drift.
export const FEED_KINDS = ['contact', 'aggregate'] as const;
export const FEED_DETAILS = ['headlines', 'full'] as const;

export type FeedKind = (typeof FEED_KINDS)[number];
export type FeedDetail = (typeof FEED_DETAILS)[number];

// Headlines carries no free text, so it is the safe default (ADR 0030).
export const DEFAULT_FEED_DETAIL: FeedDetail = 'headlines';

export interface Feed {
  id: string;
  name: string;
  kind: FeedKind;
  /** Contact.VCardUID for a `contact` feed; empty for `aggregate`. */
  entity_id: string;
  detail: FeedDetail;
  created_at: string;
  last_accessed_at: string | null;
}

export interface FeedInput {
  name: string;
  kind: FeedKind;
  entity_id?: string;
  detail?: FeedDetail;
}

export interface FeedCreateResponse {
  feed: Feed;
  /** Shown once. Relative (`/api/v1/...`) when the server has no absolute FRONTEND_URL. */
  url: string;
}

export interface RevokeAllFeedsResponse {
  revoked: number;
}

/**
 * The server returns a path-only URL when FRONTEND_URL is the dev sentinel
 * `*`; a feed reader needs an absolute one, so prefix the page's own origin.
 */
export function absoluteFeedUrl(url: string): string {
  return url.startsWith('/') ? `${window.location.origin}${url}` : url;
}

export async function listFeeds(): Promise<Feed[]> {
  const response = await apiFetch(`${API_BASE_URL}/feeds`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load feeds.');
  return (data?.feeds as Feed[] | undefined) ?? [];
}

export async function createFeed(input: FeedInput): Promise<FeedCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/feeds`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  const data = await handleResponse(response, 'Unable to create feed.');
  return data as FeedCreateResponse;
}

/** Revokes the feed and reissues it with the same configuration; new URL shown once. */
export async function rotateFeed(id: string): Promise<FeedCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/feeds/${id}/rotate`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to rotate feed.');
  return data as FeedCreateResponse;
}

export async function revokeFeed(id: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/feeds/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to revoke feed.');
}

export async function revokeAllFeeds(): Promise<RevokeAllFeedsResponse> {
  const response = await apiFetch(`${API_BASE_URL}/feeds/revoke-all`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to revoke all feeds.');
  return { revoked: typeof data?.revoked === 'number' ? data.revoked : 0 };
}
