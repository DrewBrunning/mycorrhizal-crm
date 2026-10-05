// Contact Map API (ADR 0031, issue #1286): the tile-style bootstrap, the
// single-address geocode trigger, and the plottable-points loader.
import type { ContactMapResponse } from '../generated/openapi';
import { API_BASE_URL, apiFetch, getAuthHeaders, parseErrorResponse } from './client';

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

// One plottable address.
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

// Fixed-point coordinate formatting: rounds to six decimal places (~0.1 m),
// half away from zero, then trims trailing zeros — matching the backend's
// `formatCoord` (math.Round) and Android's BigDecimal/HALF_UP formatter.
// `Number.prototype.toString` switches to scientific notation below 1e-3
// (e.g. `1e-7`), which `decimal()` rejects, so a small coordinate written here
// would fail to parse on reload.
function formatCoord(value: number): string {
  const scaled = value * 1e6;
  const rounded = (scaled < 0 ? -Math.round(-scaled) : Math.round(scaled)) / 1e6;
  return rounded.toFixed(6).replace(/\.?0+$/, '');
}

export function formatGeoUri(lat: number, lng: number): string {
  return `geo:${formatCoord(lat)},${formatCoord(lng)}`;
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

// GET /contacts/map (issue #1427): one item per plottable address of the
// caller's own non-archived contacts. Not sensitivity-gated (ADR 0031 section
// 3): the map is the owner's own view of their own data. The server already
// validated each coordinate; the client re-parses so a bad value can never be
// plotted at a nonsense position.
export interface MapPointsResult {
  points: MapPoint[];
  // True when the server hit its point ceiling and the list is partial.
  truncated: boolean;
}

export async function getMapPoints(): Promise<MapPointsResult> {
  const response = await apiFetch(`${API_BASE_URL}/contacts/map`, { headers: getAuthHeaders() });
  if (!response.ok) {
    throw await parseErrorResponse(response);
  }
  const data: ContactMapResponse = await response.json();
  const points: MapPoint[] = [];
  for (const p of data.points) {
    const pos = parseGeoUri(p.coordinates);
    if (!pos) continue;
    points.push({
      contactId: p.contact_id,
      contactName: p.contact_name,
      addressId: p.address_id,
      label: p.label,
      ...pos,
    });
  }
  return { points, truncated: data.truncated };
}
