package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** #2499: the suggestion and search-hit models behave as values (pairs are matched by equality). */
class EntitySuggestionsModelTest {

    private val pair = EntitySuggestionPair(
        reason = "exact_name",
        a = EntitySuggestionEntity(id = "a", kind = EntityKind.PET, name = "Rex"),
        b = EntitySuggestionEntity(id = "b", kind = EntityKind.PET, name = "Rex")
    )

    @Test
    fun `a pair is equal to an identical pair and unequal once either side changes`() {
        assertEquals(pair, pair.copy())
        assertEquals(pair.hashCode(), pair.copy().hashCode())
        assertNotEquals(pair, pair.copy(a = pair.a.copy(id = "z")))
        assertTrue(pair.toString().contains("Rex"))
    }

    @Test
    fun `every suggestion model has safe defaults for a sparse payload`() {
        val entity = EntitySuggestionEntity()
        assertEquals("", entity.id)
        assertEquals(EntityKind.ORGANISATION, entity.kind)
        assertEquals("", entity.name)
        assertEquals(EntitySuggestionEntity(), EntitySuggestionPair().a)
        assertEquals("", EntitySuggestionPair().reason)
        val response = EntitySuggestionsResponse()
        assertEquals("pairs", response.mode)
        assertEquals(0, response.limit)
        assertEquals(null, response.nextCursor)
        assertEquals(EntityDismissResponse(), EntityDismissResponse(idLow = "", idHigh = ""))
    }

    @Test
    fun `a response exposes an empty list when the server sent null`() {
        assertTrue(EntitySuggestionsResponse(pairs = null).pairsOrEmpty.isEmpty())
        val full = EntitySuggestionsResponse(mode = "pairs", pairs = listOf(pair), limit = 50)
        assertEquals(listOf(pair), full.pairsOrEmpty)
        assertEquals(full, full.copy())
        assertNotEquals(full, full.copy(nextCursor = "c"))
        assertTrue(full.toString().contains("pairs"))
    }

    @Test
    fun `dismiss input and response are plain values`() {
        val input = EntityDismissInput(entityA = "a", entityB = "b")
        assertEquals(input, input.copy())
        assertEquals(input.hashCode(), input.copy().hashCode())
        assertNotEquals(input, input.copy(entityB = "c"))
        assertTrue(input.toString().contains("entityA"))
        val response = EntityDismissResponse(idLow = "a", idHigh = "b")
        assertEquals(response, response.copy())
        assertEquals(response.hashCode(), response.copy().hashCode())
        assertNotEquals(response, response.copy(idHigh = "c"))
        assertTrue(response.toString().contains("idLow"))
    }

    @Test
    fun `a search entity hit defaults to a normal organisation and compares by value`() {
        val hit = SearchEntityHit()
        assertEquals(EntityKind.ORGANISATION, hit.kind)
        assertEquals("normal", hit.sensitivity)
        val named = SearchEntityHit(id = "e1", kind = EntityKind.PLACE, name = "Cabin")
        assertEquals(named, named.copy())
        assertEquals(named.hashCode(), named.copy().hashCode())
        assertNotEquals(named, named.copy(name = "Lodge"))
        assertTrue(named.toString().contains("Cabin"))
        assertTrue(SearchResult().entities.isEmpty())
        assertEquals(listOf(named), SearchResult(entities = listOf(named)).entities)
    }
}
