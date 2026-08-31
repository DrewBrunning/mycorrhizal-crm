// Export API functions
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

/**
 * Helper function to download a file from an API response. Exported so
 * other export-shaped API modules (e.g. api/audit.ts's exportAuditLog) can
 * reuse the same download mechanics instead of duplicating them.
 */
export async function downloadFileFromResponse(
  response: Response,
  defaultFilename: string,
): Promise<void> {
  // Get the filename from Content-Disposition header or use default
  const contentDisposition = response.headers.get('Content-Disposition');
  let filename = defaultFilename;
  if (contentDisposition) {
    const filenameMatch = contentDisposition.match(/filename=([^;]+)/);
    if (filenameMatch) {
      filename = filenameMatch[1].replace(/"/g, '').trim();
    }
  }

  // Get the blob data
  const blob = await response.blob();

  // Create a download link and trigger download
  const url = window.URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  window.URL.revokeObjectURL(url);
}

/**
 * Export all user data as CSV
 * Downloads a CSV file containing all contacts, activities, notes, relationships, and reminders
 */
export async function exportDataAsCsv(): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/export`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, 'mycorrhizal-export.csv');
}

// --- T9: selective field export + sensitivity gating -----------------

export type ExportFormat = 'vcf4' | 'vcf3' | 'jscontact';

// The wire format tokens the backend preflight endpoint accepts (DATA-02,
// issue #442). Maps onto ExportFormat.
export const PREFLIGHT_FORMAT: Record<ExportFormat, string> = {
  vcf4: 'vcard4',
  vcf3: 'vcard3',
  jscontact: 'jscontact',
};

// One fidelity-loss event on one export (DATA-02, issue #442): which contact
// lost which field to which format, and why (the DATA-01 matrix bucket +
// reason). Mirrors backend models.LossReport.
export interface ExportLossReport {
  format: 'vcard4' | 'vcard3' | 'jscontact';
  contact_id: number;
  contact_name: string;
  vcard_uid: string;
  severity: 'warn';
  concept: string;
  bucket: 'unsupported' | 'lossy';
  reason: string;
  message: string;
}

// The X-Mycorrhizal-Export-Loss-Report header payload on a file-download
// export (DATA-02, issue #442). Mirrors controllers.exportLossHeader.
export interface ExportLossHeader {
  format: string;
  count: number;
  truncated: boolean;
  diagnostics: ExportLossReport[];
}

// The response of GET /export/preflight (DATA-02, issue #442): what an export
// with the given params would lose, computed without producing the file.
export interface ExportLossPreflightResponse {
  format: string;
  contact_count: number;
  diagnostics: ExportLossReport[];
}

// Decode the X-Mycorrhizal-Export-Loss-Report response header on a file
// download. Returns null when the header is absent (no loss report attached).
export function parseExportLossHeader(response: Response): ExportLossHeader | null {
  const raw = response.headers.get('X-Mycorrhizal-Export-Loss-Report');
  if (!raw) return null;
  try {
    return JSON.parse(decodeURIComponent(raw)) as ExportLossHeader;
  } catch {
    return null;
  }
}

// Preflight an export: ask the backend "if I export these contacts to this
// format with this selection, what will be lost?" without producing the file
// (DATA-02, issue #442).
export async function exportLossPreflight(
  format: ExportFormat,
  selection: ExportSelection,
  vcardUID?: string,
): Promise<ExportLossPreflightResponse> {
  const params = new URLSearchParams();
  params.set('format', PREFLIGHT_FORMAT[format]);
  params.set('sections', selection.sections.join(','));
  if (selection.includeSensitive) {
    params.set('include_sensitive', 'true');
  }
  if (vcardUID) {
    params.set('vcard_uid', vcardUID);
  }

  const response = await apiFetch(`${API_BASE_URL}/export/preflight?${params.toString()}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  return response.json();
}

export interface ExportFieldSection {
  token: string;
  sensitive: boolean;
}

// Coarse-grained export field sections (Google-Contacts-style picker, not
// per-value). MUST stay in sync with backend/models/field_selection.go's
// section tokens and sensitiveSections set (CLAUDE.md frontend trap #4) —
// this is the hand-maintained mirror of the backend's oneof token list.
export const EXPORT_FIELD_SECTIONS: ExportFieldSection[] = [
  { token: 'emails', sensitive: false },
  { token: 'phones', sensitive: false },
  { token: 'addresses', sensitive: false },
  { token: 'organizations', sensitive: false },
  { token: 'anniversaries', sensitive: false },
  { token: 'media', sensitive: false },
  { token: 'online_services', sensitive: false },
  { token: 'links', sensitive: false },
  { token: 'notes', sensitive: false },
  { token: 'keywords', sensitive: false },
  { token: 'related_to', sensitive: true },
  { token: 'personal_info', sensitive: true },
  { token: 'speak_to_as', sensitive: false },
  { token: 'members', sensitive: false },
  { token: 'languages', sensitive: false },
  { token: 'custom_fields', sensitive: true },
];

export interface ExportSelection {
  sections: string[];
  // Explicit opt-in override: include private/secret items in the
  // sensitivity-bearing sections just this time. Never implied by merely
  // checking a section — the UI gates this behind a deliberate reveal action.
  includeSensitive: boolean;
}

/**
 * Export all contacts in the given format (vCard 3.0, vCard 4.0, or JSContact),
 * filtered to the chosen field sections. Identity data (name/UID) is always
 * included by the backend regardless of selection.
 */
export async function exportContacts(
  format: ExportFormat,
  selection: ExportSelection,
): Promise<void> {
  const params = new URLSearchParams();
  params.set('sections', selection.sections.join(','));
  if (selection.includeSensitive) {
    params.set('include_sensitive', 'true');
  }

  let endpoint: string;
  let defaultFilename: string;
  if (format === 'jscontact') {
    endpoint = `${API_BASE_URL}/export/jscontact`;
    defaultFilename = 'mycorrhizal-contacts.jscontact.json';
  } else {
    endpoint = `${API_BASE_URL}/export/vcf`;
    if (format === 'vcf3') {
      params.set('version', '3');
      defaultFilename = 'mycorrhizal-contacts-v3.vcf';
    } else {
      defaultFilename = 'mycorrhizal-contacts.vcf';
    }
  }

  const response = await apiFetch(`${endpoint}?${params.toString()}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, defaultFilename);
}

/**
 * Export a single contact in the given format. Adds ?vcard_uid= to scope the
 * export to one contact, then delegates to the existing export endpoint.
 */
export async function exportContact(
  format: ExportFormat,
  vcardUID: string,
  selection?: ExportSelection,
): Promise<void> {
  const sections = selection?.sections ?? EXPORT_FIELD_SECTIONS.map((s) => s.token);
  const includeSensitive = selection?.includeSensitive ?? false;

  const params = new URLSearchParams();
  params.set('vcard_uid', vcardUID);
  sections.forEach((s) => {
    params.append('sections', s);
  });
  if (includeSensitive) {
    params.set('include_sensitive', 'true');
  }

  let endpoint: string;
  let defaultFilename: string;
  if (format === 'jscontact') {
    endpoint = `${API_BASE_URL}/export/jscontact`;
    defaultFilename = `${vcardUID}.json`;
  } else {
    endpoint = `${API_BASE_URL}/export/vcf`;
    if (format === 'vcf3') {
      params.set('version', '3');
      defaultFilename = `${vcardUID}.vcf`;
    } else {
      defaultFilename = `${vcardUID}.vcf`;
    }
  }

  const response = await apiFetch(`${endpoint}?${params.toString()}`, {
    method: 'GET',
    headers: getAuthHeaders(),
  });

  if (!response.ok) {
    const error = await parseErrorResponse(response);
    throw error;
  }

  await downloadFileFromResponse(response, defaultFilename);
}

/**
 * Export all contacts as VCF (vCard 4.0), all sections included
 * Downloads a VCF file containing all contacts with their photos. Sends every
 * section token (the pre-T9 "no selection" default), with sensitivity items
 * still opt-out: the backend's default-exclude keeps private/secret
 * data out unless include_sensitive is explicitly sent.
 */
export async function exportContactsAsVcf(): Promise<void> {
  await exportContacts('vcf4', {
    sections: EXPORT_FIELD_SECTIONS.map((s) => s.token),
    includeSensitive: false,
  });
}
