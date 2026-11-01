// Per-user CardDAV sync scope by circle (issue #1993, ADR 0041).
// A user with no stored scope is in "all" mode, exactly today's behaviour.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export type CardDAVSyncScopeMode = 'all' | 'include' | 'exclude';

export type CardDAVSyncScopeWarningCode =
  | 'scope_empty'
  | 'default_circle_missing'
  | 'exclusion_empty';

export interface CardDAVSyncScopeWarning {
  code: CardDAVSyncScopeWarningCode;
  message: string;
}

export interface CardDAVSyncScope {
  mode: CardDAVSyncScopeMode;
  circle_ids: string[];
  default_circle_id: string | null;
  updated_at: string | null;
  in_scope_count: number;
  total_count: number;
  warnings: CardDAVSyncScopeWarning[];
}

export interface CardDAVSyncScopeInput {
  mode: CardDAVSyncScopeMode;
  circle_ids: string[];
  default_circle_id: string | null;
}

export interface CardDAVSyncScopePreview {
  mode: CardDAVSyncScopeMode;
  circle_ids: string[];
  in_scope_count: number;
  total_count: number;
  warnings: CardDAVSyncScopeWarning[];
}

const SCOPE_URL = `${API_BASE_URL}/users/me/carddav-sync-scope`;

export async function getCardDAVSyncScope(): Promise<CardDAVSyncScope> {
  const response = await apiFetch(SCOPE_URL, { headers: getAuthHeaders() });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function putCardDAVSyncScope(input: CardDAVSyncScopeInput): Promise<CardDAVSyncScope> {
  const response = await apiFetch(SCOPE_URL, {
    method: 'PUT',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function previewCardDAVSyncScope(
  input: CardDAVSyncScopeInput,
): Promise<CardDAVSyncScopePreview> {
  const response = await apiFetch(`${SCOPE_URL}/preview`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
