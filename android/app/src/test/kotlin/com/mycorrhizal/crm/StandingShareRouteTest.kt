package com.mycorrhizal.crm

import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Issue #2518: the route the note, activity and circle entry points navigate to. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class StandingShareRouteTest {

    @Test
    fun `the route carries the type and the id`() {
        assertEquals("standing-share/note/abc-123", standingShareRoute("note", "abc-123"))
    }

    @Test
    fun `a reserved character in the id cannot break out of its path segment`() {
        assertEquals("standing-share/circle/a%2Fb%3Fc", standingShareRoute("circle", "a/b?c"))
    }

    @Test
    fun `the route pattern names the two arguments the screen reads`() {
        assertEquals("standing-share/{entityType}/{entityId}", STANDING_SHARE_ROUTE)
    }
}
