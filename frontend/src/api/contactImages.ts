// Contact gallery images (ADR 0040 slice c, issue #2004).
import {
  API_BASE_URL,
  apiFetch,
  getAuthHeaders,
  getConditionalHeaders,
  parseErrorResponse,
} from './client';

export interface ContactImage {
  id: number;
  uuid: string;
  contact_id: number;
  contact_vcard_uid?: string;
  mime_type: 'image/jpeg' | 'image/png';
  width: number;
  height: number;
  size_bytes: number;
  sensitivity: 'normal' | 'private' | 'secret';
  taken_at?: string | null;
  taken_date?: string | null;
  created_at: string;
  file_url: string;
  thumbnail_url: string;
  // Stored avatar crop (fractions of the full variant); always present, null when unset.
  crop: ImageCrop | null;
}

// Fractions of the full-size variant; the stored image is never cropped
// destructively (ADR 0040 decision 10).
export interface ImageCrop {
  x: number;
  y: number;
  w: number;
  h: number;
}

export type ImageSensitivity = 'normal' | 'private' | 'secret';

// Where a Photos-tab image lives: the contact itself, one of its notes or one
// of its activities. Ids are only unique per owner table, so identity is
// (source.kind, id).
export interface PhotoSource {
  kind: 'contact' | 'note' | 'activity';
  id: number;
  label: string;
}

export interface ContactPhoto {
  id: number;
  source: PhotoSource;
  mime_type: 'image/jpeg' | 'image/png';
  width: number;
  height: number;
  size_bytes: number;
  sensitivity: ImageSensitivity;
  taken_at?: string | null;
  taken_date?: string | null;
  created_at: string;
  file_url: string;
  thumbnail_url: string;
  // Set for contact-gallery images only.
  crop: ImageCrop | null;
}

export interface ContactPhotosResponse {
  photos: ContactPhoto[];
  total: number;
}

export interface MainImageResponse {
  message: string;
  main_image_id?: number;
  // The contact revision after the write; the next conditional write must echo it.
  revision: number;
}

export interface AdoptImageInput {
  source_kind: 'note' | 'activity';
  source_id: number;
  image_id: number;
}

export function contactImageSrc(path: string): string {
  return `${API_BASE_URL.replace(/\/api\/v1$/, '')}${path}`;
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

// The aggregated Photos tab: own, note and activity images, newest first.
export async function getContactPhotos(contactId: string | number): Promise<ContactPhotosResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/photos`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// Sets the main image (and the derived avatar). `revision` is the contact's
// current revision, sent as If-Match; a stale one is a 412 the caller refetches
// on (ADR 0008).
export async function setContactMainImage(
  contactId: string | number,
  imageId: number,
  revision?: number,
): Promise<MainImageResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/main-image`, {
    method: 'PUT',
    headers: getConditionalHeaders(revision),
    body: JSON.stringify({ image_id: imageId }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

export async function clearContactMainImage(
  contactId: string | number,
  revision?: number,
): Promise<MainImageResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/main-image`, {
    method: 'DELETE',
    headers: getConditionalHeaders(revision),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// `crop` absent leaves the stored crop alone, null clears it, a rectangle sets it.
export async function patchContactImage(
  contactId: string | number,
  imageId: number,
  patch: { crop?: ImageCrop | null; sensitivity?: ImageSensitivity },
): Promise<ContactImage> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/images/${imageId}`, {
    method: 'PATCH',
    headers: getAuthHeaders(),
    body: JSON.stringify(patch),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const body: { image: ContactImage } = await response.json();
  return body.image;
}

// Copies a note or activity image into the contact's own gallery. A
// near-duplicate returns the existing image with `deduplicated: true`.
export async function adoptContactImage(
  contactId: string | number,
  input: AdoptImageInput,
): Promise<{ image: ContactImage; deduplicated: boolean }> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/images/adopt`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify(input),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}
