// Admin API calls for user management

import type { User, UserCreateInput, UsersListResponse, UserUpdateInput } from '../types';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

// Get current authenticated user's information
export async function getCurrentUser(): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/users/me`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Get paginated list of all users (admin only)
export async function getUsers(page: number = 1, limit: number = 25): Promise<UsersListResponse> {
  const params = new URLSearchParams({
    page: page.toString(),
    limit: limit.toString(),
  });

  const response = await apiFetch(`${API_BASE_URL}/admin/users?${params}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Create a new user (admin only)
export async function createUser(data: UserCreateInput): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Update a user (admin only)
export async function updateUser(id: number, data: UserUpdateInput): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify(data),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}

// Trigger reminder emails manually (admin only)
export async function triggerReminders(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/admin/trigger-reminders`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

/**
 * Delete a user (admin only)
 */
export async function deleteUser(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
}

/**
 * Reset a user's two-factor authentication (admin only, issue #592).
 * Operator-side recovery for a user locked out of their own account (TOTP
 * device and recovery codes both lost): disables TOTP and hard-deletes
 * their recovery codes. No proof is required from the target -- the
 * caller's own authenticated admin session is the trust boundary, same as
 * the existing admin password reset above. Idempotent: safe to call on a
 * user with no 2FA enabled.
 */
export async function resetUserTwoFactor(id: number): Promise<User> {
  const response = await apiFetch(`${API_BASE_URL}/admin/users/${id}/reset-2fa`, {
    method: 'POST',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    throw await parseErrorResponse(response);
  }

  return response.json();
}
