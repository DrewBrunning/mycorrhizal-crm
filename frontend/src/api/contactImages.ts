// Contact gallery images (ADR 0040 slice c, issue #2004).
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

export interface ContactImage {
  id: number;
  uuid: string;
  contact_id: number;
  contact_vcard_uid: string;
  mime_type: 'image/jpeg' | 'image/png';
  width: number;
  height: number;
  size_bytes: number;
  sensitivity: 'normal' | 'private' | 'secret';
  taken_at: string | null;
  taken_date: string | null;
  created_at: string;
  file_url: string;
  thumbnail_url: string;
}

export interface ContactImagesResponse {
  images: ContactImage[];
  total: number;
}

export function contactImageSrc(path: string): string {
  return `${API_BASE_URL.replace(/\/api\/v1$/, '')}${path}`;
}

export async function getContactImages(contactId: string | number): Promise<ContactImagesResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/images`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function uploadContactImage(
  contactId: string | number,
  file: File,
): Promise<ContactImage> {
  const form = new FormData();
  form.append('file', file);
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/images`, {
    method: 'POST',
    body: form,
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const body: { image: ContactImage } = await response.json();
  return body.image;
}

export async function deleteContactImage(
  contactId: string | number,
  imageId: number,
): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/images/${imageId}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
