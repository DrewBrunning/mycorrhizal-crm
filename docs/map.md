---
title: Contact map
nav_order: 25
has_children: false
---

# Contact map

The contact map plots every contact address that has coordinates on a slippy map, so you can see where
the people you care about live. It is part of the web app (**Map** in the navigation, `/map`) and the
Android app (**Map** in the drawer or rail). The design is
[ADR 0031](adrs/0031-contact-map-provider-and-geocoding.md).

## Using the map

- **Only addresses with coordinates are plotted.** A contact with an address but no coordinates does not
  appear. Archived and deleted contacts are not plotted.
- **Each pin is one address.** Selecting it shows the contact's name and the formatted address, with an
  **Open contact** button.
- **Empty state.** If nothing has coordinates yet, the page tells you to add coordinates to an address on a
  contact's page.
- **Android also has a list view.** The top-bar toggle switches between the map and a plain list of the same
  points, because a map canvas cannot be traversed with TalkBack. Tapping a list row opens the contact.
- **The 5000-point limit.** The map is bounded at 5000 addresses. Past that it shows the first 5000 (in
  contact-ID order) and a notice, "Showing the first 5000 addresses only; the map is limited to this many
  points." Nothing is dropped silently.

## Adding coordinates to an address

On a contact's edit form each address has a **Coordinates (latitude, longitude)** field, for example
`51.5007, -0.1246`. Latitude must be -90 to 90 and longitude -180 to 180, separated by a comma; anything else is
flagged as invalid. Coordinates are stored on the address as an RFC 5870 `geo:` URI, so they travel with the
address through vCard, JSContact and CardDAV (see [CardDAV](carddav.md) for how CardDAV carries them).
A contact imported with a `GEO` value is plottable straight away.

### Find coordinates (geocoding)

Next to the field, **Find coordinates** asks a geocoding service to look the address up for you.

- It is **off by default**. The operator must set `GEOCODER_PROVIDER` (see
  [Operator setup](#operator-setup)); until then the lookup fails with "geocoding is not enabled on this
  server".
- It works on a **saved** address. On a new, unsaved address the editor asks you to save first.
- It is **one address per click.** The app never geocodes in the background or in bulk, and the only thing
  sent to the provider is the address text of that one address.
- On success the coordinates are written to the address (replacing any that were there) and shown in the
  field.
- Repeat lookups of the same address within 24 hours are answered from a small in-memory cache on the server
  (up to 512 entries, cleared on restart); nothing is written to the database for this.
- Failures give a short reason: no result found, the provider is rate limiting (try again shortly), the
  provider rejected the server's key, or the provider could not be reached.
- **In an Android local (on-device) profile** the geocoding endpoint does not exist, because it is an outbound
  integration and local profiles are storage only. Enter coordinates by hand instead.

## Address sensitivity

Each address carries a sensitivity: `normal` (the default), `private` or `secret`. Anything above `normal` is
treated as data you do not want to leave your instance:

| Where | Private or secret address (and its coordinates) |
|---|---|
| vCard 3.0 / 4.0 and JSContact exports | Withheld unless you opt in (`include_sensitive=true`; in the export field picker, "Enable sensitive fields") |
| Contact shares to another user | Withheld unless you opt in |
| CardDAV sync | Not served to clients, and preserved on the server when a client writes the card back |
| MCP server | Withheld unless the call passes `include_sensitive: true` (see [MCP](mcp.md#sensitivity)) |
| Find coordinates | Refused through the apps: a private or secret address is never sent to the geocoder. Enter coordinates by hand |
| **Your own map** | **Shown.** The map is your own view of your own data, not an export or a copy |
| Flat CSV export and account bundle backups | **Included**, each labelled. These are your own full backups |

So marking an address private keeps it out of every copy that leaves the instance or reaches someone else, but
you still see it on your own map.

As of v1.4.0 neither the web nor the Android editor has a control for setting an address's sensitivity. The
value is set through the REST API (`sensitivity` on an entry in the contact record's `addresses`; see the
[API reference](api-reference.md)), and the editors keep an existing value when you save.

## Operator setup

Everything works with no configuration: the map loads tiles from the free hosted
[OpenFreeMap](https://openfreemap.org/) service and geocoding is off. The settings below are instance-wide, are
listed with their exact defaults in the [configuration reference](configuration-reference.md), and need a
restart.

| Variable | Default | Meaning |
|---|---|---|
| `MAP_TILE_STYLE_URL` | `https://tiles.openfreemap.org/styles/liberty` | Absolute http(s) URL of a [MapLibre](https://maplibre.org/) style JSON. The web and Android maps both load tiles from it. A value that is not an absolute http(s) URL stops the server from booting. |
| `GEOCODER_PROVIDER` | `none` | `none`, `nominatim` or `maptiler`. With `none`, no address text ever leaves the instance for geocoding. Any other value is a boot error. |
| `GEOCODER_API_KEY` | empty | Required when the provider is `maptiler` (the server refuses to boot without it); unused for `nominatim`. |

- **Tiles.** The browser (or the Android app) fetches map tiles directly from the host named in the style.
  Tile requests describe the visible part of the map, not individual contacts or addresses. Clients learn the
  style URL from the public, unauthenticated `GET /api/v1/config/map`, which returns only
  `{"tile_style_url": "..."}`.
- **The map needs the tile host to be reachable from the user's browser.** v1.4.1 and later derive the allowed
  tile origin from `MAP_TILE_STYLE_URL` automatically. On v1.4.0 the bundled nginx Content-Security-Policy is
  not derived from it, so if the web map is blank or says "The map could not be displayed", check the CSP
  first. See [Deployment](deployment.md#contact-map-and-geocoding).
- **Nominatim** is the public OpenStreetMap instance (`nominatim.openstreetmap.org`). It needs no key, but its
  usage policy allows at most one request per second, and the server throttles itself to that. That suits
  occasional per-address lookups, which is all this feature does. Choose `maptiler` (your own key) for more
  headroom.
- **Outbound traffic.** Geocoder calls use the same SSRF-guarded client as the other integrations. See
  [Integrations](integration-ownership.md#geocoder) for ownership and failure behaviour.
