package com.mycorrhizal.crm.ui.components

/**
 * ADR 0031 / issue #1287: the per-address "find coordinates" lookup, owned by
 * the screen's view model. The lookup is the stateless draft route
 * (`POST /contacts/{id}/addresses/geocode`), so it works for unsaved addresses
 * but still needs the *contact* to exist ([canGeocode] false until it does).
 *
 * Lives in its own file (not AddressEditor.kt) so detekt's
 * MatchingDeclarationName rule — which only counts top-level class/object
 * declarations, and so would otherwise require the file to be named after
 * this data class — stays satisfied.
 */
data class AddressGeocodeState(
    val canGeocode: Boolean = false,
    /**
     * The active profile is on-device (embedded): the backend registers no
     * geocode routes there, so the action is disabled with its own reason.
     */
    val localProfile: Boolean = false,
    /** Row keys (`addressRowKey`) with a lookup in flight. */
    val inFlight: Set<String> = emptySet(),
    /** Row key (`addressRowKey`) -> message for a failed lookup. */
    val errors: Map<String, String> = emptyMap(),
)
