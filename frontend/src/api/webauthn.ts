import {
  createCredential,
  getAssertion,
  type WireCreationOptions,
  type WireRequestOptions,
} from '../webauthnCeremony';
import { API_BASE_URL, apiFetch, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';

// Issue #594: passkey enrollment + management. Login lives in ../auth.ts
// (loginWithPasskey) because it mints the session.

export interface Passkey {
  id: string;
  name: string;
  created_at: string;
  last_used_at?: string | null;
}

export interface PasskeyEnrollment {
  id: string;
  name: string;
  created_at: string;
  // Present (non-empty) only when this passkey was the account's first
  // second factor — shown exactly once.
  recovery_codes: string[];
}

export async function listPasskeys(): Promise<Passkey[]> {
  const response = await apiFetch(`${API_BASE_URL}/webauthn/credentials`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });
  const data = await handleResponse(response, 'Unable to load passkeys.');
  return data?.credentials || [];
}

// registerPasskey runs the whole ceremony: begin → navigator.credentials.create
// → finish. A dismissed browser prompt rejects with the browser's own
// DOMException (see isCeremonyCancelled) so the caller can tell it apart from
// a backend failure.
//
// Issue #1337: once the account already holds a second factor (TOTP or a
// passkey) the backend refuses begin without a live `proof`, which is sent
// alongside the name.
export async function registerPasskey(
  name?: string,
  proof?: SecondFactorProof,
): Promise<PasskeyEnrollment> {
  const beginResponse = await apiFetch(`${API_BASE_URL}/webauthn/register/begin`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ ...(name?.trim() ? { name: name.trim() } : {}), ...proof }),
  });
  const options = (await handleResponse(
    beginResponse,
    'Unable to start passkey registration.',
  )) as WireCreationOptions;

  const credential = await createCredential(options);

  const finishResponse = await apiFetch(`${API_BASE_URL}/webauthn/register/finish`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(credential),
  });
  const data = await handleResponse(finishResponse, 'Unable to register passkey.');
  return { ...(data as PasskeyEnrollment), recovery_codes: data?.recovery_codes || [] };
}

export type SecondFactorProof = { code: string } | { assertion: Record<string, unknown> };
export type PasskeyRemovalProof = SecondFactorProof;

// proveWithAnyPasskey begins a proof ceremony over ALL of the caller's passkeys
// (nothing is being removed) and returns the assertion the enrollment
// endpoints accept in place of a TOTP/recovery code (issue #1337).
export async function proveWithAnyPasskey(): Promise<{ assertion: Record<string, unknown> }> {
  const response = await apiFetch(`${API_BASE_URL}/webauthn/assert/begin`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({}),
  });
  const options = (await handleResponse(
    response,
    'Unable to start passkey verification.',
  )) as WireRequestOptions;
  return { assertion: await getAssertion(options) };
}

// proveWithOtherPasskey begins a proof ceremony and returns the assertion the
// delete endpoint accepts in place of a TOTP/recovery code. `excludeId` is the
// passkey being removed: the backend leaves it out of allowCredentials so the
// browser cannot pick the very credential that would be rejected.
export async function proveWithOtherPasskey(
  excludeId: string,
): Promise<{ assertion: Record<string, unknown> }> {
  const response = await apiFetch(`${API_BASE_URL}/webauthn/assert/begin`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ exclude_id: excludeId }),
  });
  const options = (await handleResponse(
    response,
    'Unable to start passkey verification.',
  )) as WireRequestOptions;
  return { assertion: await getAssertion(options) };
}

export async function removePasskey(id: string, proof: PasskeyRemovalProof): Promise<void> {
  const response = await apiFetch(
    `${API_BASE_URL}/webauthn/credentials/${encodeURIComponent(id)}`,
    {
      method: 'DELETE',
      headers: getAuthHeaders(),
      body: JSON.stringify(proof),
    },
  );
  await handleResponse(response, 'Unable to remove passkey.');
}
