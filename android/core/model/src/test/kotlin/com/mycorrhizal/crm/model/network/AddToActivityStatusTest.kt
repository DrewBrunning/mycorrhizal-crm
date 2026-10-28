package com.mycorrhizal.crm.model.network

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

// Issue #2136: wire statuses map to the enum; unknown values are failures, never silent successes.
class AddToActivityStatusTest {

    @Test
    fun `each wire status maps to its enum value`() {
        assertEquals(AddToActivityStatus.ADDED, AddToActivityStatus.fromWire("added"))
        assertEquals(
            AddToActivityStatus.ALREADY_MEMBER,
            AddToActivityStatus.fromWire("already_member")
        )
        assertEquals(AddToActivityStatus.NOT_FOUND, AddToActivityStatus.fromWire("not_found"))
        assertEquals(AddToActivityStatus.UNKNOWN, AddToActivityStatus.fromWire("anything_else"))
        assertEquals(AddToActivityStatus.UNKNOWN, AddToActivityStatus.fromWire(""))
    }

    @Test
    fun `only added and already_member count as success`() {
        assertTrue(AddToActivityStatus.ADDED.isSuccess)
        assertTrue(AddToActivityStatus.ALREADY_MEMBER.isSuccess)
        assertFalse(AddToActivityStatus.NOT_FOUND.isSuccess)
        assertFalse(AddToActivityStatus.UNKNOWN.isSuccess)
    }

    @Test
    fun `a result exposes its outcome from the raw status`() {
        assertEquals(AddToActivityStatus.NOT_FOUND, AddToActivityResult(7, "not_found").outcome)
    }

    @Test
    fun `the response models default to empty and compare by value`() {
        val empty = AddContactToActivitiesResponse()
        assertEquals(0, empty.contactId)
        assertTrue(empty.results.isEmpty())
        assertEquals(AddToActivityResult(), AddToActivityResult(0, ""))

        val result = AddToActivityResult(activityId = 4, status = "added")
        val response = AddContactToActivitiesResponse(contactId = 9, results = listOf(result))
        assertEquals(response, response.copy())
        assertEquals(response.hashCode(), response.copy().hashCode())
        assertFalse(response == response.copy(contactId = 10))
        assertFalse(result == result.copy(status = "not_found"))
        assertTrue(response.toString().contains("contactId=9"))
        assertEquals(4, result.component1())
        assertEquals("added", result.component2())
        assertEquals(9, response.component1())
        assertEquals(listOf(result), response.component2())
        assertTrue(result.toString().contains("added"))
    }
}
