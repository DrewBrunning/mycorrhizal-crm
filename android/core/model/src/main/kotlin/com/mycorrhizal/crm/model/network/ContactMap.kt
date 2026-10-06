package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Json
import com.squareup.moshi.JsonClass
import java.math.BigDecimal
import java.math.RoundingMode

// ADR 0031 / issue #1287: the contact map's wire types.

/** `GET /config/map` (public) — the MapLibre style JSON URL the instance serves. */
@JsonClass(generateAdapter = true)
data class MapConfig(
    @Json(name = "tile_style_url") val tileStyleUrl: String = "",
)

/**
 * `GET /contacts/map` — one item per plottable address of the caller's own
 * non-archived contacts. Not sensitivity-gated: the map is the owner's own
 * view of their own data (ADR 0031 section 3). Bounded server-side; [truncated]
 * is true when the list is partial.
 *
 * [points] is nullable for the same reason as the other list DTOs
 * (a Moshi non-null list rejects an explicit JSON `null`); callers use
 * [pointsOrEmpty].
 */
@JsonClass(generateAdapter = true)
data class ContactMapResponse(
    val points: List<ContactMapPoint>? = null,
    val truncated: Boolean = false,
) {
    val pointsOrEmpty: List<ContactMapPoint> get() = points ?: emptyList()
}

@JsonClass(generateAdapter = true)
data class ContactMapPoint(
    @Json(name = "contact_id") val contactId: Int = 0,
    @Json(name = "contact_uid") val contactUid: String = "",
    @Json(name = "contact_name") val contactName: String = "",
    @Json(name = "address_id") val addressId: String = "",
    val label: String = "",
    /** A `geo:` URI; see [parseGeoUri]. */
    val coordinates: String = "",
)

/**
 * `POST /contacts/:id/addresses/geocode` request: the stateless draft lookup.
 * Only the postal fields the geocoder reads plus the sensitivity; no id or
 * coordinate, because this body never writes an address.
 */
@JsonClass(generateAdapter = true)
data class GeocodeDraftRequest(
    val street: String = "",
    val city: String = "",
    val region: String = "",
    val postal: String = "",
    val country: String = "",
    val sensitivity: String = "",
)

/** `POST /contacts/:id/addresses/geocode` response: the coordinate, NOT stored server-side. */
@JsonClass(generateAdapter = true)
data class GeocodeDraftResponse(
    val coordinates: String = "",
    val cached: Boolean = false,
)

/** A validated WGS84 position. */
data class LatLng(val latitude: Double, val longitude: Double)

// The grammar the Go server's strconv.ParseFloat accepts for stored coordinates:
// optional sign, ".5"/"5." forms and an exponent ("+48.2", "4.8e1", "1E-7").
// Must stay a superset of what the server accepts (testdata/geo-uri-fixtures.json);
// a stricter client silently drops a stored valid point.
private val DECIMAL = Regex("^[+-]?([0-9]+\\.?[0-9]*|\\.[0-9]+)([eE][+-]?[0-9]+)?$")

private fun decimal(text: String): Double? =
    text.takeIf { DECIMAL.matches(it) }?.toDoubleOrNull()?.takeIf { it.isFinite() }

private fun latLngOrNull(lat: Double?, lng: Double?): LatLng? {
    if (lat == null || lng == null) return null
    if (lat < -90.0 || lat > 90.0 || lng < -180.0 || lng > 180.0) return null
    return LatLng(lat, lng)
}

/**
 * Parses an RFC 5870 `geo:` URI (`geo:51.5007,-0.1246`, optionally with an
 * altitude and `;u=`/`;crs=` parameters) into a range-checked [LatLng], or null
 * for anything malformed or out of range — so a bad stored value is skipped,
 * never plotted at a nonsense position. Mirrors the web's `parseGeoUri`.
 */
fun parseGeoUri(uri: String?): LatLng? {
    val text = uri?.trim().orEmpty()
    if (!text.startsWith("geo:", ignoreCase = true)) return null
    val parts = text.substring(4).split(';', '?').first().split(',')
    if (parts.size !in 2..3) return null
    if (parts.size == 3 && decimal(parts[2]) == null) return null
    return latLngOrNull(decimal(parts[0]), decimal(parts[1]))
}

/**
 * Renders one coordinate as fixed-point decimal, rounded to six places (~0.1 m)
 * with trailing zeros trimmed — the web/backend `formatCoord` shape. A plain
 * `Double.toString` emits scientific notation below `1e-3` (e.g. `1.0E-4`),
 * which [parseGeoUri]'s [DECIMAL] rejects, so a small coordinate would round-trip
 * into a value the UI flags invalid.
 */
private fun formatCoord(value: Double): String =
    BigDecimal(value)
        .setScale(6, RoundingMode.HALF_UP)
        .stripTrailingZeros()
        .toPlainString()

/** Formats a position as the `geo:` URI the backend stores. */
fun formatGeoUri(latitude: Double, longitude: Double): String =
    "geo:${formatCoord(latitude)},${formatCoord(longitude)}"

/**
 * Parses a hand-typed "lat, lng" pair (comma and/or whitespace separated) into
 * a range-checked [LatLng], or null when it is not a valid coordinate.
 */
fun parseCoordinateInput(input: String): LatLng? {
    val parts = input.trim().split(Regex("[,\\s]+")).filter { it.isNotEmpty() }
    if (parts.size != 2) return null
    return latLngOrNull(decimal(parts[0]), decimal(parts[1]))
}
