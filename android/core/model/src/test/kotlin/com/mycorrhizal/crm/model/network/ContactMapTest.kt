package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ContactMapTest {

    @Test
    fun `parseGeoUri parses plain, altitude and parameterised URIs`() {
        assertEquals(LatLng(51.5007, -0.1246), parseGeoUri("geo:51.5007,-0.1246"))
        assertEquals(LatLng(-33.0, 151.0), parseGeoUri("GEO:-33,151"))
        assertEquals(LatLng(48.2, 16.3), parseGeoUri("geo:48.2,16.3,190"))
        assertEquals(LatLng(48.2, 16.3), parseGeoUri("geo:48.2,16.3;u=35"))
        assertEquals(LatLng(90.0, 180.0), parseGeoUri("  geo:90,180  "))
    }

    @Test
    fun `parseGeoUri rejects malformed and out-of-range values`() {
        listOf(
            null, "", "geo:", "geo:abc,def", "geo:91,0", "geo:-91,0", "geo:0,181", "geo:0,-181",
            "http://x/1,2", "geo:1", "geo:1,2,x", "geo:1,2,3,4", "geo:1.2.3,4", "geo:--1,2",
        ).forEach { assertNull("expected null for $it", parseGeoUri(it)) }
    }

    @Test
    fun `formatGeoUri round-trips through parseGeoUri`() {
        assertEquals("geo:51.5,-0.12", formatGeoUri(51.5, -0.12))
        assertEquals(LatLng(51.5, -0.12), parseGeoUri(formatGeoUri(51.5, -0.12)))
    }

    @Test
    fun `formatGeoUri never emits scientific notation`() {
        // Regression for #1443: Double.toString switches to `1.0E-4` below 1e-3,
        // which parseGeoUri's DECIMAL rejects, so the value fails to round-trip.
        listOf(
            0.0001 to -0.0004,
            0.001 to -0.001,
            0.00009 to 0.0,
            0.0 to 0.0,
            51.5 to -0.12,
            -33.8688 to 151.2093,
            90.0 to 180.0,
        ).forEach { (lat, lng) ->
            val uri = formatGeoUri(lat, lng)
            // The `geo:` scheme itself contains an `e`; the coordinates must not.
            val coords = uri.removePrefix("geo:")
            assertFalse("scientific notation leaked into '$uri'", coords.contains('E') || coords.contains('e'))
            assertEquals("round-trip failed for '$uri'", LatLng(lat, lng), parseGeoUri(uri))
        }
    }

    @Test
    fun `formatGeoUri emits the fixed-point form the backend stores`() {
        assertEquals("geo:0.0001,-0.0004", formatGeoUri(0.0001, -0.0004))
        assertEquals("geo:51.5,-0.12", formatGeoUri(51.5, -0.12))
        assertEquals("geo:0,0", formatGeoUri(0.0, -0.0))
    }

    @Test
    fun `parseCoordinateInput accepts comma or whitespace separated pairs`() {
        assertEquals(LatLng(51.5007, -0.1246), parseCoordinateInput("51.5007, -0.1246"))
        assertEquals(LatLng(51.5007, -0.1246), parseCoordinateInput("51.5007,-0.1246"))
        assertEquals(LatLng(10.0, 20.0), parseCoordinateInput("  10 20 "))
        assertEquals(LatLng(-90.0, 180.0), parseCoordinateInput("-90, 180"))
    }

    @Test
    fun `parseCoordinateInput rejects everything else`() {
        listOf("", "12", "1,2,3", "a, b", "91, 0", "0, 181", "-91, 0", "1.2.3, 4", "1;2")
            .forEach { assertNull("expected null for '$it'", parseCoordinateInput(it)) }
    }

    @Test
    fun `isGeocodable is false only for private and secret addresses`() {
        assertTrue(Address().isGeocodable)
        assertTrue(Address(sensitivity = "").isGeocodable)
        assertTrue(Address(sensitivity = "normal").isGeocodable)
        assertFalse(Address(sensitivity = "private").isGeocodable)
        assertFalse(Address(sensitivity = "secret").isGeocodable)
    }

    @Test
    fun `Address round-trips sensitivity through Moshi so a save cannot erase it`() {
        val adapter = MoshiProvider.get().adapter(Address::class.java)
        val parsed = adapter.fromJson("""{"id":"a","sensitivity":"secret","coordinates":"geo:1,2"}""")
        assertEquals("secret", parsed?.sensitivity)
        assertTrue(adapter.toJson(parsed).contains("\"sensitivity\":\"secret\""))
    }

    @Test
    fun `ContactMapResponse decodes the spec shape and tolerates null points`() {
        val adapter = MoshiProvider.get().adapter(ContactMapResponse::class.java)
        val full = adapter.fromJson(
            """{"points":[{"contact_id":2,"contact_uid":"u","contact_name":"Ada","address_id":"a","label":"10 Downing St","coordinates":"geo:51.5,-0.12"}],"truncated":true}""",
        )
        assertNotNull(full)
        assertTrue(full!!.truncated)
        assertEquals(1, full.pointsOrEmpty.size)
        assertEquals(2, full.pointsOrEmpty[0].contactId)
        assertEquals("a", full.pointsOrEmpty[0].addressId)
        assertEquals("geo:51.5,-0.12", full.pointsOrEmpty[0].coordinates)

        val empty = adapter.fromJson("""{"points":null,"truncated":false}""")
        assertTrue(empty!!.pointsOrEmpty.isEmpty())
        assertFalse(empty.truncated)
    }

    @Test
    fun `MapConfig and GeocodeDraftResponse decode their wire names`() {
        val moshi = MoshiProvider.get()
        assertEquals(
            "https://tiles.example/style",
            moshi.adapter(MapConfig::class.java).fromJson("""{"tile_style_url":"https://tiles.example/style"}""")?.tileStyleUrl,
        )
        val geo = moshi.adapter(GeocodeDraftResponse::class.java)
            .fromJson("""{"coordinates":"geo:1,2","cached":true}""")
        assertEquals("geo:1,2", geo?.coordinates)
        assertTrue(geo!!.cached)
    }
}
