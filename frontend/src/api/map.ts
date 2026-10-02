// Contact Map API (ADR 0031, issue #1286): the tile-style bootstrap, the
// single-address geocode trigger, and the plottable-points loader.
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';
import { type Contact, getAllContacts } from './contacts';

// GET /config/map -- public; the MapLibre style JSON URL.
export interface MapConfig {
  tile_style_url: string;
}

// POST /contacts/:id/addresses/:addressId/geocode response.
export interface GeocodeAddressResult {
  address_id: string;
  coordinates: string;
  cached: boolean;
}

export interface LatLng {
  lat: number;
  lng: number;
}

// One plottable address. `sensitivity` is carried for display only: the map
// is the owner's own view, so it is never used to filter.
export interface MapPoint {
  contactId: number;
  contactName: string;
  addressId: string;
  label: string;
  lat: number;
  lng: number;
}

// A plain decimal ("51.5", "-0.12"); rejects "", "1.2.3", "--1", "1e5".
function decimal(text: string): number | null {
  if (!/^[-0-9.]+$/.test(text)) return null;
  const n = Number(text);
  return Number.isFinite(n) ? n : null;
}

function toLatLng(lat: number | null, lng: number | null): LatLng | null {
  if (lat === null || lng === null) return null;
  if (lat < -90 || lat > 90 || lng < -180 || lng > 180) return null;
  return { lat, lng };
}

// Parses an RFC 5870 geo: URI ("geo:51.5007,-0.1246", optionally with
// altitude and ;u=/;crs= parameters) into a validated lat/lng. Returns null
// for anything malformed or out of range, so a bad stored value is skipped
// rather than plotted at a nonsense position.
export function parseGeoUri(uri: string | undefined | null): LatLng | null {
  const text = uri?.trim() ?? '';
  if (text.slice(0, 4).toLowerCase() !== 'geo:') return null;
  const parts = text.slice(4).split(/[;?]/, 1)[0].split(',');
  if (parts.length < 2 || parts.length > 3) return null;
  if (parts.length === 3 && decimal(parts[2]) === null) return null;
  return toLatLng(decimal(parts[0]), decimal(parts[1]));
}

export function formatGeoUri(lat: number, lng: number): string {
  return `geo:${lat},${lng}`;
}

// Parses a manually typed "lat, lng" pair (comma or whitespace separated).
// Returns null when it is not a valid in-range coordinate.
export function parseCoordinateInput(input: string): LatLng | null {
  const parts = input
    .trim()
    .split(/[,\s]+/)
    .filter(Boolean);
  if (parts.length !== 2) return null;
  return toLatLng(decimal(parts[0]), decimal(parts[1]));
}

export async function getMapConfig(): Promise<MapConfig> {
  const response = await apiFetch(`${API_BASE_URL}/config/map`);
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  return response.json();
}

// Triggers exactly one geocode lookup for one saved address. `includeSensitive`
// is the explicit opt-in the backend requires for a private/secret address.
export async function geocodeAddress(
  contactId: string | number,
  addressId: string,
  includeSensitive = false,
): Promise<GeocodeAddressResult> {
  const query = includeSensitive ? '?include_sensitive=true' : '';
  const response = await apiFetch(
    `${API_BASE_URL}/contacts/${contactId}/addresses/${encodeURIComponent(addressId)}/geocode${query}`,
    { method: 'POST', headers: getAuthHeaders() },
  );
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  return response.json();
}

interface JSContactAddress {
  components?: { kind: string; value: string }[];
  coordinates?: string;
  contexts?: Record<string, boolean>;
  full?: string;
}

interface JSContactCard {
  uid?: string;
  addresses?: Record<string, JSContactAddress>;
}

function displayName(c: Contact): string {
  return [c.firstname, c.lastname].filter(Boolean).join(' ') || c.nickname || '';
}

function addressLabel(a: JSContactAddress): string {
  if (a.full) return a.full;
  const find = (kind: string) => a.components?.find((c) => c.kind === kind)?.value;
  return [find('name') ?? find('number'), find('locality'), find('region'), find('country')]
    .filter(Boolean)
    .join(', ');
}

// Loads every plottable address of the caller's own (non-archived) contacts.
//
// There is no bulk "addresses with coordinates" endpoint, and the contact list
// is a slim projection without addresses. The JSContact export with
// sections=addresses is the one existing bulk read that carries each address's
// geo: URI. It is requested with include_sensitive=true on purpose: the map is
// the owner's own view of their own data, not an export/sync/share, so it is
// not sensitivity-gated (ADR 0031 section 3, the same rule as the CSV export).
// Cards are joined to the contact list by vCard UID for the numeric id (needed
// to link to the contact page) and the display name.
export async function getMapPoints(): Promise<MapPoint[]> {
  const [contacts, response] = await Promise.all([
    getAllContacts({ limit: 100 }),
    apiFetch(`${API_BASE_URL}/export/jscontact?sections=addresses&include_sensitive=true`, {
      headers: getAuthHeaders(),
    }),
  ]);
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  const cards: JSContactCard[] = await response.json();

  const byUid = new Map<string, Contact>();
  for (const c of contacts) {
    if (c.uid && c.ID != null) byUid.set(c.uid, c);
  }

  const points: MapPoint[] = [];
  for (const card of cards) {
    const contact = card.uid ? byUid.get(card.uid) : undefined;
    if (!contact || contact.ID == null) continue;
    for (const [addressId, addr] of Object.entries(card.addresses ?? {})) {
      const pos = parseGeoUri(addr.coordinates);
      if (!pos) continue;
      points.push({
        contactId: contact.ID,
        contactName: displayName(contact),
        addressId,
        label: addressLabel(addr),
        ...pos,
      });
    }
  }
  return points;
}
