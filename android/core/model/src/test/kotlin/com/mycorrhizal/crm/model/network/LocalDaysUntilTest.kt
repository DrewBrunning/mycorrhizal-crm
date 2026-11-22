package com.mycorrhizal.crm.model.network

import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Test

/** Issue #2617: occasion countdowns are counted from the device's day, not the server's zone. */
class LocalDaysUntilTest {

    private val today = LocalDate.of(2026, 11, 13)

    @Test
    fun `counts whole days from the device date to the occurrence date`() {
        assertEquals(0, localDaysUntil("2026-11-13", serverDaysUntil = 5, today = today))
        assertEquals(1, localDaysUntil("2026-11-14", serverDaysUntil = 0, today = today))
        assertEquals(20, localDaysUntil("2026-12-03", serverDaysUntil = 19, today = today))
    }

    @Test
    fun `a server that already rolled to tomorrow does not make tomorrow read as today`() {
        // The audit repro: device Nov 13, server (UTC) already Nov 14 -> server says "Today".
        val eve = UpcomingOccasion(
            contactName = "Eve",
            source = UpcomingOccasionSources.BIRTHDAY,
            date = "2026-11-14",
            daysUntil = 0
        )
        assertEquals(1, eve.daysUntilOn(today))
    }

    @Test
    fun `a gift item is counted from the device date too`() {
        val gift = GiftShoppingItem(date = "2026-12-03", daysUntil = 19)
        assertEquals(20, gift.daysUntilOn(today))
    }

    @Test
    fun `a date already past on the device is clamped to today`() {
        assertEquals(0, localDaysUntil("2026-11-12", serverDaysUntil = 0, today = today))
    }

    @Test
    fun `an unparseable date keeps the server count`() {
        assertEquals(7, localDaysUntil("", serverDaysUntil = 7, today = today))
        assertEquals(7, localDaysUntil("--12-25", serverDaysUntil = 7, today = today))
        assertEquals(7, localDaysUntil("2026-13-45", serverDaysUntil = 7, today = today))
    }
}
