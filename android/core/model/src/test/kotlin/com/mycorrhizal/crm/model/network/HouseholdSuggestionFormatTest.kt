package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Test

class HouseholdSuggestionFormatTest {

    @Test
    fun `null address renders empty`() {
        assertEquals("", formatSuggestionAddress(null))
    }

    @Test
    fun `full address wins over components`() {
        val address = Address(
            full = "1 Main St, Berlin",
            components = listOf(
                AddressComponent(kind = "locality", value = "Berlin"),
                AddressComponent(kind = "country", value = "DE"),
            ),
        )
        assertEquals("1 Main St, Berlin", formatSuggestionAddress(address))
    }

    @Test
    fun `components render in street locality region postcode country order`() {
        val address = Address(
            full = null,
            components = listOf(
                AddressComponent(kind = "country", value = "DE"),
                AddressComponent(kind = "postcode", value = "10115"),
                AddressComponent(kind = "locality", value = "Berlin"),
                AddressComponent(kind = "name", value = "1 Main St"),
                AddressComponent(kind = "region", value = "Berlin"),
            ),
        )
        assertEquals("1 Main St, Berlin, Berlin, 10115, DE", formatSuggestionAddress(address))
    }

    @Test
    fun `blank and unknown kinds are skipped`() {
        val address = Address(
            full = null,
            components = listOf(
                AddressComponent(kind = "name", value = "1 Main St"),
                AddressComponent(kind = "locality", value = "  "),
                AddressComponent(kind = "unknown-kind", value = "ignored"),
            ),
        )
        assertEquals("1 Main St", formatSuggestionAddress(address))
    }

    @Test
    fun `duplicate kinds only render the first value`() {
        val address = Address(
            full = null,
            components = listOf(
                AddressComponent(kind = "name", value = "1 Main St"),
                AddressComponent(kind = "name", value = "2 Second St"),
            ),
        )
        assertEquals("1 Main St", formatSuggestionAddress(address))
    }

    @Test
    fun `null components render empty`() {
        assertEquals("", formatSuggestionAddress(Address(full = null, components = null)))
    }
}
