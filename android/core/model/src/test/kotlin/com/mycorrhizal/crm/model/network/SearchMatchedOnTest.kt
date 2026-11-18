package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2630 / backend #2757: which contact fields a search hit matched, keyed by contact id. */
class SearchMatchedOnTest {

    @Test
    fun `only hits that report a non-empty reason are mapped`() {
        val result = SearchResult(
            contacts = listOf(
                SearchContactHit(id = 1, matchedOn = listOf("name", "email")),
                SearchContactHit(id = 2, matchedOn = null),
                SearchContactHit(id = 3, matchedOn = emptyList()),
                SearchContactHit(id = 4, matchedOn = listOf("phone"))
            )
        )
        assertEquals(
            mapOf(1 to listOf("name", "email"), 4 to listOf("phone")),
            result.matchedOnByContactId()
        )
    }

    @Test
    fun `a result without a contacts group maps nothing`() {
        assertTrue(SearchResult().matchedOnByContactId().isEmpty())
    }

    @Test
    fun `a hit without an id cannot be attached to a row`() {
        val result = SearchResult(
            contacts = listOf(SearchContactHit(matchedOn = listOf("name")))
        )
        assertTrue(result.matchedOnByContactId().isEmpty())
    }
}
