// Attachment API calls -- N7.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';

export interface Attachment {
  // gorm.Model fields serialize with their Go names (no json tags on the
  // embedded model) — the frontend mirrors the raw wire shape like the other
  // gorm.Model-backed APIs (e.g. activities).
  ID: number;
  CreatedAt: string;
  UpdatedAt: string;
  contact_vcard_uid: string;
  original_name: string;
  content_type: string;
  size_bytes: number;
}

export interface AttachmentListResponse {
  attachments: Attachment[];
  total: number;
}

// GET /contacts/:id/attachments
export async function listContactAttachments(contactId: number | string): Promise<AttachmentListResponse> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/attachments`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
  return response.json();
}

// POST /contacts/:id/attachments
export async function uploadAttachment(contactId: number | string, file: File): Promise<Attachment> {
  const formData = new FormData();
  formData.append('file', file);
  const response = await apiFetch(`${API_BASE_URL}/contacts/${contactId}/attachments`, {
    method: 'POST',
    body: formData,
  });
  if (!response.ok) throw await parseErrorResponse(response);
  const result = await response.json();
  return result.attachment;
}

// DELETE /attachments/:id
export async function deleteAttachment(id: number): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/attachments/${id}`, {
    method: 'DELETE',
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}

// The download URL is a same-origin link so the session cookie authenticates
// it; the browser handles the Content-Disposition from the server.
export function attachmentDownloadUrl(id: number): string {
  return `${API_BASE_URL}/attachments/${id}/download`;
}

// Renders a byte count as a compact human string (KB/MB).
export function formatAttachmentSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
