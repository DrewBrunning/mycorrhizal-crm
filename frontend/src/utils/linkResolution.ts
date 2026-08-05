// Tappable-field link building (T34): tel:/sms:/mailto: builders, the
// client-side safe-URL guard, and OnlineService/address -> link resolution.
import { CardOnlineService, ContactAddress } from '../api/contacts';
import { LinkFieldType } from '../api/linkFieldTypes';

export function buildTelLink(number: string): string {
  return `tel:${number}`;
}

export function buildSmsLink(number: string): string {
  return `sms:${number}`;
}

export function buildMailtoLink(email: string): string {
  return `mailto:${email}`;
}

// Client-side mirror of backend/middleware/validation.go's validateSafeURL.
// The backend's `safeurl` validator only ever sees the STORED template
// (e.g. "https://wa.me/{value}") — a template with a safe scheme can still
// produce an unsafe href once {value} is substituted with a handle at
// render time (or, for a raw passthrough OnlineService.URI/Card.Links
// entry, the value was never validated server-side as an href at all).
// This must run on every built/substituted/passthrough URL before it is
// used as an <a href>, not just on registry Protocol templates.
export function isSafeUrlString(url: string): boolean {
  const raw = url.trim();
  if (raw === '') return true;
  const normalized = Array.from(raw)
    .filter((ch) => ch.charCodeAt(0) > 32)
    .join('')
    .toLowerCase();
  const colonIndex = normalized.indexOf(':');
  if (colonIndex > 0) {
    const scheme = normalized.slice(0, colonIndex);
    if (['javascript', 'data', 'vbscript', 'file'].includes(scheme)) {
      return false;
    }
  }
  return true;
}

// True when the value looks like an absolute URI ("scheme:...") rather
// than a bare handle/string — guards Card.Links/IMPP rendering against
// turning a non-URI value (e.g. a handle with no scheme) into a nonsense
// href. isSafeUrlString alone isn't sufficient for this: it accepts any
// value without a dangerous scheme, including plain text with none at all.
export function looksLikeAbsoluteUri(value: string): boolean {
  return /^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(value.trim());
}

// Resolves an OnlineService (SocialProfiles/OtherOnlineServices/IMPP,
// docs/fork-plan/tickets/43-T34-contact-field-linking.md §5) to a tappable
// href, or null if it isn't resolvable:
//   1. A full URI is already a complete profile link — use it directly.
//   2. Else, case-insensitively match `service` against the user's
//      LinkFieldType registry by name; if matched and `user` (the handle)
//      is populated, and the matched template actually contains the
//      {value} placeholder, substitute it.
//   3. Else null — the field is not tappable, only copyable.
// Every candidate href is passed through isSafeUrlString before being
// returned, regardless of which branch produced it.
export function resolveOnlineServiceLink(
  service: CardOnlineService,
  linkFieldTypes: LinkFieldType[]
): string | null {
  if (service.uri && service.uri.trim()) {
    return isSafeUrlString(service.uri) ? service.uri : null;
  }

  if (service.service && service.user) {
    const match = linkFieldTypes.find(
      (lt) => lt.name.toLowerCase() === service.service!.toLowerCase()
    );
    if (match?.protocol && match.protocol.includes('{value}')) {
      const substituted = match.protocol.replace('{value}', encodeURIComponent(service.user));
      return isSafeUrlString(substituted) ? substituted : null;
    }
  }

  return null;
}

// The single-line human-readable rendering of an address, shared by the
// display link's copy value and the map-search fallback query — reuses the
// same non-empty-parts join every other address renderer in this codebase
// uses (backend's FormatAddress, this file's own former duplicate).
export function formatAddressLine(address: Pick<ContactAddress, 'street' | 'city' | 'region' | 'postal' | 'country'>): string {
  return [address.street, address.city, address.region, address.postal, address.country]
    .filter((part) => part && part.trim())
    .join(', ');
}

// Builds the address -> map link (§7): a geo: URI when coordinates are
// present, else a web map search built from the formatted address. A web
// link (not app-scheme detection) degrades correctly on both platforms,
// since Apple/Google Maps register as handlers for their own web URLs via
// universal/app links — no UA-sniffing needed. Returns null when there is
// nothing to link to (no coordinates and no formattable parts).
export function buildAddressLink(address: ContactAddress): string | null {
  if (address.coordinates && address.coordinates.trim()) {
    return isSafeUrlString(address.coordinates) ? address.coordinates : null;
  }
  const formatted = formatAddressLine(address);
  if (!formatted) return null;
  return `https://maps.google.com/?q=${encodeURIComponent(formatted)}`;
}
