package com.mycorrhizal.crm.model.network

import com.mycorrhizal.crm.model.MoshiProvider
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** ADR 0024 part 5 (issue #2521): the gift shopping list wire models (`models/gift_shopping.go`). */
class GiftShoppingListModelTest {

    private val moshi = MoshiProvider.get()

    @Test
    fun `the response parses items with and without a linked gift`() {
        val response = moshi.adapter(GiftShoppingListResponse::class.java).fromJson(
            """{"gift_shopping_list":[
              {"contact_id":7,"contact_name":"Dana White","obligation_id":"o1","label":"Birthday gift",
               "date":"2026-12-18","days_until":6,"status":"purchased","linked_gift_id":"g1"},
              {"contact_id":8,"contact_name":"Sam","obligation_id":"o2","label":"Xmas","date":"2026-12-25",
               "days_until":0,"status":"needed"}],"days":30}"""
        )!!
        assertEquals(30, response.days)
        val first = response.items[0]
        assertEquals(7, first.contactId)
        assertEquals("Dana White", first.contactName)
        assertEquals("o1", first.obligationId)
        assertEquals("2026-12-18", first.date)
        assertEquals(6, first.daysUntil)
        assertEquals("purchased", first.status)
        assertEquals("g1", first.linkedGiftId)
        assertNull(response.items[1].linkedGiftId)
        assertEquals(0, response.items[1].daysUntil)
    }

    @Test
    fun `a null or missing list is an empty list`() {
        val adapter = moshi.adapter(GiftShoppingListResponse::class.java)
        assertTrue(adapter.fromJson("""{"days":90}""")!!.items.isEmpty())
        assertTrue(adapter.fromJson("""{"gift_shopping_list":null,"days":90}""")!!.items.isEmpty())
    }

    @Test
    fun `the status set mirrors the backend including needed`() {
        assertEquals(
            listOf("needed", "idea", "purchased", "given", "received"),
            GiftShoppingStatuses.ALL
        )
    }
}

/** ADR 0024 part 3 (issue #2521): the upcoming-occasions aggregate (`models/occasion_upcoming.go`). */
class UpcomingOccasionsModelTest {
    private val moshi = com.mycorrhizal.crm.model.MoshiProvider.get()

    @Test
    fun `every source parses and kind is only present on obligations`() {
        val r = moshi.adapter(UpcomingOccasionsResponse::class.java).fromJson(
            """{"occasions":[
            {"contact_id":1,"contact_name":"Ann","source":"birthday","label":"Birthday","date":"2026-12-01","days_until":3},
            {"contact_id":2,"contact_name":"Bo","source":"obligation","label":"Xmas card","date":"2026-12-25","days_until":27,"kind":"card"}],
            "days":30}"""
        )!!
        assertEquals(30, r.days)
        assertEquals("birthday", r.items[0].source)
        assertNull(r.items[0].kind)
        assertEquals("card", r.items[1].kind)
        assertEquals(27, r.items[1].daysUntil)
        assertEquals(2, r.items[1].contactId)
    }

    @Test
    fun `a null occasions list is empty`() {
        val adapter = moshi.adapter(UpcomingOccasionsResponse::class.java)
        assertTrue(adapter.fromJson("""{"occasions":null,"days":90}""")!!.items.isEmpty())
    }

    @Test
    fun `the source set mirrors the backend`() {
        assertEquals(
            listOf("birthday", "anniversary", "life_event", "obligation"),
            UpcomingOccasionSources.ALL
        )
    }
}
