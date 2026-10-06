package com.mycorrhizal.crm.model.network

import com.squareup.moshi.Moshi
import com.squareup.moshi.Types
import java.io.File
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The shared geo: URI parity fixture (/testdata/geo-uri-fixtures.json, also read
 * by the Go and web tests). Go is the validation authority: every input it
 * accepts (valid=true) must be accepted here, and every formatter output must
 * be fixed-point and parse. valid=false cases are Go-only (clients may be more
 * lenient). The file is read via a path relative to the module directory
 * (Gradle runs unit tests with the module as the working directory).
 */
class GeoUriFixtureTest {
    private val fixture: Map<String, Any?> = run {
        val file = File("../../../testdata/geo-uri-fixtures.json")
        assertTrue("fixture not found at ${file.absolutePath}", file.exists())
        val type = Types.newParameterizedType(Map::class.java, String::class.java, Any::class.java)
        Moshi.Builder().build().adapter<Map<String, Any?>>(type).fromJson(file.readText())!!
    }

    @Suppress("UNCHECKED_CAST")
    private fun cases(key: String): List<Map<String, Any?>> =
        fixture[key] as List<Map<String, Any?>>

    private fun Map<String, Any?>.num(key: String) = (this[key] as Double)

    @Test
    fun fixtureIsNonEmpty() {
        assertTrue(cases("parse").isNotEmpty())
        assertTrue(cases("format").isNotEmpty())
        assertTrue(cases("coordinateInput").isNotEmpty())
    }

    @Test
    fun parseGeoUriAcceptsEveryServerValidInput() {
        for (c in cases("parse").filter { it["valid"] == true }) {
            val input = c["input"] as String
            val got = parseGeoUri(input)
            assertNotNull("parseGeoUri must accept $input", got)
            assertEquals(input, c.num("lat"), got!!.latitude, 1e-12)
            assertEquals(input, c.num("lon"), got.longitude, 1e-12)
        }
    }

    @Test
    fun formatGeoUriMatchesFixtureAndParses() {
        for (c in cases("format")) {
            val got = formatGeoUri(c.num("lat"), c.num("lon"))
            assertEquals(c["expected"], got)
            assertFalse("$got uses an exponent", got.removePrefix("geo:").contains(Regex("[eE]")))
            assertNotNull("$got must parse", parseGeoUri(got))
        }
    }

    @Test
    fun parseCoordinateInputMatchesFixture() {
        for (c in cases("coordinateInput")) {
            val input = c["input"] as String
            val got = parseCoordinateInput(input)
            if (c["valid"] == true) {
                assertNotNull("parseCoordinateInput must accept '$input'", got)
                assertEquals(input, c.num("lat"), got!!.latitude, 1e-12)
                assertEquals(input, c.num("lon"), got.longitude, 1e-12)
            } else {
                assertNull("parseCoordinateInput must reject '$input'", got)
            }
        }
    }
}
