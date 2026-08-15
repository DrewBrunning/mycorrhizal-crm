// Duplicate-scan API calls -- T93
// (T93).
// Mirrors backend/models/duplicate.go's DTOs by hand -- no dynamic schema
// endpoint exists anywhere in this codebase, so this must be kept in sync
// manually if the backend shape changes.
import { apiFetch, API_BASE_URL, getAuthHeaders, parseErrorResponse } from './client';
import { Contact, ContactSummaryDTO, summaryToLegacyContact } from './contacts';

export type DuplicateReason = 'email' | 'name' | 'phone';

// DuplicatePair is one candidate pair from GET /contacts/duplicates. a and b
// are the same slim summaries the list endpoint returns, already mapped down
// to the flat Contact shape so the review surface can render them with
// existing components (avatars, name rows, ...).
export interface DuplicatePair {
  a: Contact;
  b: Contact;
  reasons: DuplicateReason[];
  confidence: number;
}

export interface DuplicatePairsResponse {
  pairs: DuplicatePair[];
  total: number;
  page: number;
  limit: number;
}

interface DuplicatePairDTO {
  a: ContactSummaryDTO;
  b: ContactSummaryDTO;
  reasons: DuplicateReason[];
  confidence: number;
}

// getDuplicatePairs runs the scan. Already-dismissed pairs never come back.
export async function getDuplicatePairs(params: { page?: number; limit?: number } = {}): Promise<DuplicatePairsResponse> {
  const queryParams = new URLSearchParams({
    page: (params.page ?? 1).toString(),
    limit: (params.limit ?? 50).toString(),
  });
  const response = await apiFetch(`${API_BASE_URL}/contacts/duplicates?${queryParams.toString()}`, {
    headers: getAuthHeaders(),
  });
  if (!response.ok) throw await parseErrorResponse(response);

  const data: { pairs: DuplicatePairDTO[]; total: number; page: number; limit: number } = await response.json();
  return {
    pairs: (data.pairs || []).map((p) => ({
      a: summaryToLegacyContact(p.a),
      b: summaryToLegacyContact(p.b),
      reasons: p.reasons || [],
      confidence: p.confidence,
    })),
    total: data.total,
    page: data.page,
    limit: data.limit,
  };
}

// dismissDuplicatePair records that the pair is not a duplicate, so the scan
// never offers it again. Idempotent server-side.
export async function dismissDuplicatePair(uidA: string, uidB: string): Promise<void> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/duplicates/dismiss`, {
    method: 'POST',
    headers: getAuthHeaders(),
    body: JSON.stringify({ uid_a: uidA, uid_b: uidB }),
  });
  if (!response.ok) throw await parseErrorResponse(response);
}
