package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #2629: the client-only "served from the offline mirror" marker and its list helper. */
class ServedFromCacheTest {

    @Test
    fun `a list is saved data when any row was served from the mirror`() {
        val rows = listOf(Circle(id = "a"), Circle(id = "b", servedFromCache = true))

        assertTrue(rows.anyServedFromCache())
    }

    @Test
    fun `a fresh or empty list is not saved data`() {
        assertFalse(listOf(Circle(id = "a"), Circle(id = "b")).anyServedFromCache())
        assertFalse(emptyList<Circle>().anyServedFromCache())
    }

    @Test
    fun `the marker never travels on the wire`() {
        val json = com.squareup.moshi.Moshi.Builder().build()
            .adapter(Circle::class.java).toJson(Circle(id = "a", servedFromCache = true))

        assertFalse(json.contains("servedFromCache"))
    }
}
