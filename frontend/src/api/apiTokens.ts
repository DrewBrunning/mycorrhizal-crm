import { API_BASE_URL, apiFetch, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';

export type ApiTokenScope = 'full' | 'carddav' | 'custom';

export type ApiTokenAccessScope = 'carddav' | 'caldav' | 'api:read' | 'api:write' | 'mcp';

export type ApiTokenPreset = 'full' | 'contacts' | 'calendar' | 'both' | 'api' | 'mcp';

export const API_TOKEN_PRESET_SCOPES: Record<ApiTokenPreset, ApiTokenAccessScope[]> = {
  full: ['carddav', 'caldav', 'api:read', 'api:write', 'mcp'],
  contacts: ['carddav'],
  calendar: ['caldav'],
  both: ['carddav', 'caldav'],
  api: ['api:read', 'api:write'],
  mcp: ['mcp'],
};

export const DEFAULT_API_TOKEN_PRESET: ApiTokenPreset = 'full';

export interface ApiToken {
  id: number;
  name: string;
  created_at: string;
  last_used_at: string | null;
  revoked_at: string | null;
  /** Null only for tokens created before expiry was introduced. */
  expires_at: string | null;
  scope: ApiTokenScope;
  scopes?: ApiTokenAccessScope[];
}

/** Selectable lifetimes; the backend caps this at 365 days. */
export const API_TOKEN_EXPIRY_OPTIONS = [30, 60, 90, 180, 365] as const;

export const DEFAULT_API_TOKEN_EXPIRY_DAYS = 90;

export const DEFAULT_API_TOKEN_SCOPE: ApiTokenScope = 'full';

export interface ApiTokenCreateResponse extends ApiToken {
  token: string;
}

export interface ApiTokensListResponse {
  tokens: ApiToken[];
}

export async function getApiTokens(): Promise<ApiTokensListResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load API tokens.');
  return { tokens: data?.tokens || [] };
}

export async function createApiToken(
  name: string,
  expiresInDays: number = DEFAULT_API_TOKEN_EXPIRY_DAYS,
  scope: ApiTokenScope = DEFAULT_API_TOKEN_SCOPE,
): Promise<ApiTokenCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ name, expires_in_days: expiresInDays, scope }),
  });
  const data = await handleResponse(response, 'Unable to create API token.');
  return data as ApiTokenCreateResponse;
}

/** Issue #2010: create a token with an explicit access set. */
export async function createApiTokenWithScopes(
  name: string,
  expiresInDays: number | null,
  scopes: ApiTokenAccessScope[],
  neverExpires = false,
): Promise<ApiTokenCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      name,
      scopes,
      ...(neverExpires ? { never_expires: true } : { expires_in_days: expiresInDays }),
    }),
  });
  const data = await handleResponse(response, 'Unable to create API token.');
  return data as ApiTokenCreateResponse;
}

export async function revokeApiToken(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  await handleResponse(response, 'Unable to revoke API token.');
}

export interface RevokeAllApiTokensResponse {
  revoked: number;
}

/** Issue #413: end every one of the caller's standing tokens at once (e.g. a lost device). */
export async function revokeAllApiTokens(): Promise<RevokeAllApiTokensResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens/revoke-all`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to revoke all API tokens.');
  return data as RevokeAllApiTokensResponse;
}

/**
 * Issue #413: revoke a token and reissue a new one with the same name and
 * scope in a single step. Like createApiToken, the plaintext is returned
 * exactly once.
 */
export async function rotateApiToken(id: number): Promise<ApiTokenCreateResponse> {
  const response = await apiFetch(`${API_BASE_URL}/api-tokens/${id}/rotate`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to rotate API token.');
  return data as ApiTokenCreateResponse;
}
