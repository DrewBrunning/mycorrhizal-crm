package com.mycorrhizal.crm.model.network

/**
 * Issue #2629: a record the offline mirror can answer for. The flag is set client-side, by the
 * repository, on a record it served from the mirror because the network read failed. It is never
 * on the wire (implementers mark it `@Transient`). A screen shows the shared "Showing saved
 * data" banner from it, never from a guess.
 */
interface ServedFromCache {
    val servedFromCache: Boolean
}

/** True when any row of this list was served from the offline mirror. */
fun Iterable<ServedFromCache>.anyServedFromCache(): Boolean = any { it.servedFromCache }
