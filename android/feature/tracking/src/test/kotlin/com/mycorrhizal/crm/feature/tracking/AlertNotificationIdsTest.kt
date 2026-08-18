package com.mycorrhizal.crm.feature.tracking

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AlertNotificationIdsTest {

    @Test
    fun `same reminder and due time map to the same id`() {
        assertEquals(
            AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00Z"),
            AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00Z"),
        )
    }

    @Test
    fun `a different due time of the same reminder maps to a different id`() {
        assertNotEquals(
            AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00Z"),
            AlertNotificationIds.forReminder(42, "2026-08-19T06:00:00Z"),
        )
    }

    @Test
    fun `a different reminder maps to a different id`() {
        assertNotEquals(
            AlertNotificationIds.forReminder(1, "2026-08-18T06:00:00Z"),
            AlertNotificationIds.forReminder(2, "2026-08-18T06:00:00Z"),
        )
    }

    @Test
    fun `ids live outside the static channel id band`() {
        val id = AlertNotificationIds.forReminder(1, "2026-08-18T06:00:00Z")
        assertTrue(id >= 0x2D0000)
        assertTrue(id <= 0x2DFFFF)
    }

    @Test
    fun `the FCM due time and the polling remind_at collapse to the same id`() {
        // The FCM push carries `due_at` as canonical UTC (no sub-seconds); the
        // polling worker sees Go's RFC3339Nano `remind_at` with nanoseconds and
        // a local offset. They are the same instant and must target one id or
        // the poll-boundary double-notify survives (issue #152 constraint).
        val fcm = AlertNotificationIds.forReminder(42, "2026-08-18T14:14:28Z")
        val poll = AlertNotificationIds.forReminder(42, "2026-08-18T09:14:28.638126406-05:00")
        assertEquals(fcm, poll)
    }

    @Test
    fun `a sub-second difference within the same second is still one id`() {
        assertEquals(
            AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00.123456Z"),
            AlertNotificationIds.forReminder(42, "2026-08-18T06:00:00Z"),
        )
    }

    @Test
    fun `an unparseable due time still hashes deterministically`() {
        assertEquals(
            AlertNotificationIds.forReminder(42, "not-a-timestamp"),
            AlertNotificationIds.forReminder(42, "not-a-timestamp"),
        )
    }
}
