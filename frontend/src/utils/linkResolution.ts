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
//
// Kept in sync with the backend by hand (frontend trap 4): if a scheme
// moves in/out of the blocklist here, validateSafeURL in
// backend/middleware/validation.go must change too.
export function isSafeUrlString(url: string): boolean {
  const raw = url.trim();
  if (raw === '') return true;
  const normalized = normalizeSchemeCheck(url);
  const colonIndex = normalized.indexOf(':');
  if (colonIndex > 0) {
    const scheme = normalized.slice(0, colonIndex);
    if (['javascript', 'data', 'vbscript', 'file'].includes(scheme)) {
      return false;
    }
  }
  return true;
}

// Client-side mirror of backend/middleware/validation.go's validateHTTPURL
// (T41): an allowlist, not a blocklist. For the fields whose value means "a
// web page" (a gift's product link, an agenda item's reference article, an
// external system's deep link, an Immich base URL) only http/https are
// accepted — everything else, including an unknown scheme (blob:, intent:,
// ms-msdt:, ...) and a value with no scheme at all, is rejected. An empty
// value is accepted (fields opt into `required` separately).
//
// Kept in sync with the backend by hand (frontend trap 4); the Go table in
// backend/middleware/validation_test.go's TestValidateStruct_HTTPURL and the
// unit tests in linkResolution.test.ts must mirror each other exactly.
export function isHttpUrlString(url: string): boolean {
  const raw = url.trim();
  if (raw === '') return true;
  const normalized = normalizeSchemeCheck(url);
  const colonIndex = normalized.indexOf(':');
  if (colonIndex <= 0) return false;
  const scheme = normalized.slice(0, colonIndex);
  return scheme === 'http' || scheme === 'https';
}

// normalizeSchemeCheck strips everything ≤ U+0020 (which defeats the
// java&Tab;script: obfuscation — the browser's URL parser drops those control
// characters before it reads the scheme, so a validator that keeps them is
// checking a different string than the browser) and lowercases the result,
// ready for a leading-scheme check. Shared by isSafeUrlString and
// isHttpUrlString so the two guards' normalization cannot drift, mirroring
// backend/middleware/validation.go's own shared normalizeSchemeURL.
function normalizeSchemeCheck(value: string): string {
  return Array.from(value)
    .filter((ch) => ch.charCodeAt(0) > 32)
    .join('')
    .toLowerCase();
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
// T34 §5) to a tappable
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
    // Must actually look like a URI, not just lack a dangerous scheme --
    // isSafeUrlString alone accepts a bare handle with no scheme at all
    // (e.g. a stray "alice@example.com" imported into the URI slot), which
    // would otherwise render as a bogus same-origin/relative href.
    return looksLikeAbsoluteUri(service.uri) && isSafeUrlString(service.uri) ? service.uri : null;
  }

  if (service.service && service.user) {
    const match = linkFieldTypes.find(
      (lt) => lt.name.toLowerCase() === service.service!.toLowerCase()
    );
    if (match?.protocol && match.protocol.includes('{value}')) {
      // split/join rather than a single .replace(): a user-authored template
      // may reference {value} more than once (e.g. "…/{value}?ref={value}"),
      // and .replace(string, ...) only substitutes the first occurrence.
      const substituted = match.protocol.split('{value}').join(encodeURIComponent(service.user));
      return isSafeUrlString(substituted) ? substituted : null;
    }
  }

  return null;
}

// The single-line human-readable rendering of an address, shared by the
// display link's copy value and the map-search fallback query — reuses the
// same non-empty-parts join every other address renderer in this codebase
// uses (backend's FormatAddress, this file's own former duplicate). T79
// added the sub-street parts between street and city, matching the backend.
export function formatAddressLine(
  address: Pick<ContactAddress, 'street' | 'pobox' | 'apartment' | 'floor' | 'city' | 'region' | 'postal' | 'country'>
): string {
  return [address.street, address.pobox, address.apartment, address.floor, address.city, address.region, address.postal, address.country]
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
