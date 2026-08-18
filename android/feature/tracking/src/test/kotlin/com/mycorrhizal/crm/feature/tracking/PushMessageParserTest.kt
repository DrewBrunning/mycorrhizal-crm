package com.mycorrhizal.crm.feature.tracking

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PushMessageParserTest {

    @Test
    fun `reminder data payload parses channel deep link and idempotence key`() {
        val push = PushMessageParser.parse(
            data = mapOf(
                "type" to "reminder",
                "reminder_id" to "42",
                "due_at" to "2026-08-18T06:00:00Z",
                "deep_link" to "mycorrhizal://contacts/7",
            ),
            title = "Reminder",
            body = "Jane: Call about dinner",
        )

        requireNotNull(push)
        assertEquals("Reminder", push.title)
        assertEquals("Jane: Call about dinner", push.body)
        assertEquals(MycorrhizalNotificationChannels.REMINDERS, push.channel)
        assertEquals("42:2026-08-18T06:00:00Z", push.idempotenceKey)
        assertEquals("mycorrhizal://contacts/7", push.deepLink)
    }

    @Test
    fun `an empty payload is nothing to display`() {
        assertNull(PushMessageParser.parse(emptyMap()))
    }

    @Test
    fun `a reminder without a reminder id carries no idempotence key`() {
        // e.g. the server's Settings "test push" — a bare notification.
        val push = PushMessageParser.parse(
            data = mapOf("type" to "reminder"),
            title = "Test",
            body = "Hello",
        )

        requireNotNull(push)
        assertNull(push.idempotenceKey)
        assertEquals(MycorrhizalNotificationChannels.REMINDERS, push.channel)
    }

    @Test
    fun `an explicit channel is honored over the type default`() {
        val push = PushMessageParser.parse(
            data = mapOf("type" to "reminder", "channel" to MycorrhizalNotificationChannels.CADENCE),
            title = "T",
            body = "B",
        )

        requireNotNull(push)
        assertEquals(MycorrhizalNotificationChannels.CADENCE, push.channel)
    }

    @Test
    fun `a blank deep link is dropped`() {
        val push = PushMessageParser.parse(
            data = mapOf("type" to "reminder", "deep_link" to "  "),
            title = "T",
            body = "B",
        )

        requireNotNull(push)
        assertNull(push.deepLink)
    }
}
