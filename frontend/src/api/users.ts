// User settings and preferences API calls
import { apiFetch, API_BASE_URL, getAuthHeaders } from './client';
import { handleResponse } from './errorHandling';

export async function updateLanguage(language: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/users/language`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ language }),
  });

  const data = await handleResponse(response, 'Unable to update language.');
  return data?.message || 'Language updated successfully.';
}

export async function updateDateFormat(dateFormat: string): Promise<string> {
  const response = await apiFetch(`${API_BASE_URL}/users/date-format`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ date_format: dateFormat }),
  });

  const data = await handleResponse(response, 'Unable to update date format.');
  return data?.message || 'Date format updated successfully.';
}

// Returns the user's enabled extended contact fields, or null if never configured
// (in which case the caller should apply DEFAULT_ENABLED_CONTACT_FIELDS).
export async function getEnabledContactFields(): Promise<string[] | null> {
  const response = await apiFetch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  const data = await handleResponse(response, 'Unable to get enabled contact fields.');
  return data?.enabled_contact_fields ?? null;
}

export async function updateEnabledContactFields(fields: string[]): Promise<string[]> {
  const response = await apiFetch(`${API_BASE_URL}/users/enabled-contact-fields`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ fields }),
  });

  const data = await handleResponse(response, 'Unable to update enabled contact fields.');
  return data?.enabled_contact_fields || fields;
}

// T90: points users.self_contact_vcard_uid at one of the caller's own
// contacts, or clears it with a null vcard_uid. The backend 404s a uid that
// doesn't resolve to a non-deleted contact owned by the caller.
export async function updateSelfContact(vcardUid: string | null): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/users/me/self-contact`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify({ vcard_uid: vcardUid }),
  });

  await handleResponse(response, 'Unable to update self contact.');
}
