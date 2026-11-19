package com.mycorrhizal.crm.network

import java.time.DateTimeException
import java.time.ZoneId
import java.time.ZoneOffset

/** Mirrors the backend's `viewerTZPattern` allowlist (`reminder_clock.go`, issue #2762). */
private val IANA_ZONE_PATTERN = Regex("^[A-Za-z0-9_+\\-]+(/[A-Za-z0-9_+\\-]+)*$")

/** Mirrors the backend's `viewerTZMaxLen`. */
private const val MAX_ZONE_LENGTH = 64

/**
 * The `tz` query value for [zoneId], or `null` when the server would answer 400
 * `validation_timezone_invalid`: the backend refuses anything outside its character allowlist,
 * over 64 characters, `Local`, and any name its zoneinfo does not know. A fixed offset such as
 * `Z` or `GMT+05:00` is not an IANA zone name and is never sent; the server then falls back to its
 * own reminder zone, as it did before the parameter existed.
 */
internal fun viewerTimeZoneOrNull(zoneId: String): String? {
    if (zoneId.isEmpty() || zoneId.length > MAX_ZONE_LENGTH) return null
    if (!IANA_ZONE_PATTERN.matches(zoneId) || zoneId.equals("Local", ignoreCase = true)) return null
    val zone = try {
        ZoneId.of(zoneId)
    } catch (_: DateTimeException) {
        return null
    }
    return if (zone is ZoneOffset) null else zoneId
}
