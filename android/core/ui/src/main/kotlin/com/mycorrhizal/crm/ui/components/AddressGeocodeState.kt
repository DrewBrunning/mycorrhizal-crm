package com.mycorrhizal.crm.ui.components

/**
 * ADR 0031 / issue #1287: the per-address "find coordinates" lookup, owned by
 * the screen's view model. [canGeocode] is false until the contact exists (the
 * backend geocodes a *saved* address by contact id + address id).
 *
 * Lives in its own file (not AddressEditor.kt) so detekt's
 * MatchingDeclarationName rule — which only counts top-level class/object
 * declarations, and so would otherwise require the file to be named after
 * this data class — stays satisfied.
 */
data class AddressGeocodeState(
    val canGeocode: Boolean = false,
    /** Address ids with a lookup in flight. */
    val inFlight: Set<String> = emptySet(),
    /** Address id -> message for a failed lookup. */
    val errors: Map<String, String> = emptyMap(),
)
