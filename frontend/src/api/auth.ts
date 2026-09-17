import { API_BASE_URL, apiFetch, getAuthHeaders } from './client';
import { type ErrorDetails, extractDetailMessage, handleResponse } from './errorHandling';

export async function requestPasswordReset(email: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/password-reset/request`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ email }),
  });

  const data = await handleResponse(response, 'Unable to request password reset.');
  return data?.message || 'If an account exists, password reset instructions were sent.';
}

export async function confirmPasswordReset(token: string, password: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/password-reset/confirm`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ token, password }),
  });

  const data = await handleResponse(response, 'Unable to reset password.');
  return data?.message || 'Password reset successful.';
}

export async function changePassword(
  currentPassword: string,
  newPassword: string,
): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/users/change-password`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      current_password: currentPassword,
      new_password: newPassword,
    }),
  });

  const data = await handleResponse(response, 'Unable to change password.');
  return data?.message || 'Password updated successfully.';
}

// Issue #972: self-service account deletion.

export interface AccountDeletionCandidate {
  id: number;
  username: string;
}

// Thrown when the caller is the only admin and other users exist — deleting
// alone would strand them with no admin forever, so the caller must name one
// of `candidates` to promote to admin first (promoteUserId on a retry).
export class AccountDeletionRequiresPromotionError extends Error {
  candidates: AccountDeletionCandidate[];

  constructor(message: string, candidates: AccountDeletionCandidate[]) {
    super(message);
    this.name = 'AccountDeletionRequiresPromotionError';
    this.candidates = candidates;
  }
}

function parseDeletionCandidates(details: ErrorDetails): AccountDeletionCandidate[] {
  const raw =
    details && typeof details === 'object'
      ? (details as Record<string, unknown>).candidates
      : undefined;
  if (!Array.isArray(raw)) {
    return [];
  }
  return raw
    .filter(
      (c): c is AccountDeletionCandidate =>
        typeof c === 'object' &&
        c !== null &&
        typeof (c as Record<string, unknown>).id === 'number' &&
        typeof (c as Record<string, unknown>).username === 'string',
    )
    .map((c) => ({ id: c.id, username: c.username }));
}

// deleteOwnAccount reads the response body itself (rather than delegating to
// handleResponse) because a 409 needs its `details.candidates` array, not
// just the flattened string message handleResponse extracts — the body can
// only be read once, so there is no reusing that helper here.
export async function deleteOwnAccount(
  currentPassword: string,
  totpCode?: string,
  promoteUserId?: number,
): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/account`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
    body: JSON.stringify({
      current_password: currentPassword,
      ...(totpCode ? { totp_code: totpCode } : {}),
      ...(promoteUserId !== undefined ? { promote_user_id: promoteUserId } : {}),
    }),
  });

  const raw = await response.text();
  let data: unknown = null;
  if (raw) {
    try {
      data = JSON.parse(raw);
    } catch {
      data = raw;
    }
  }

  if (response.ok) {
    const message =
      data && typeof data === 'object' ? (data as { message?: string }).message : undefined;
    return message || 'Your account and all its data have been deleted.';
  }

  const errorDetail =
    data && typeof data === 'object'
      ? (data as { error?: { message?: string; details?: ErrorDetails } }).error
      : undefined;

  if (response.status === 409) {
    const candidates = parseDeletionCandidates(errorDetail?.details);
    if (candidates.length > 0) {
      throw new AccountDeletionRequiresPromotionError(
        errorDetail?.message ||
          'Choose another user to promote to admin before deleting your account.',
        candidates,
      );
    }
  }

  let message = 'Unable to delete account.';
  if (errorDetail) {
    const specificMessage = extractDetailMessage(errorDetail.details);
    if (specificMessage) {
      message = specificMessage;
    } else if (typeof errorDetail.message === 'string' && errorDetail.message.trim().length > 0) {
      message = errorDetail.message.trim();
    }
  } else if (typeof data === 'string' && data.trim().length > 0) {
    message = data.trim();
  }
  throw new Error(message);
}
