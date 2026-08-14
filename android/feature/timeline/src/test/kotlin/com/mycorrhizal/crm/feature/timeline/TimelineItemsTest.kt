package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompletion
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class TimelineItemsTest {

    @Test
    fun `merges activities notes and reminders newest first`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            activities = listOf(
                Activity(id = 1, title = "Old call", date = "2026-08-01T10:00:00Z"),
                Activity(id = 2, title = "New visit", date = "2026-08-05T10:00:00Z"),
            ),
            notes = listOf(Note(id = 3, content = "Loves climbing", date = "2026-08-03T10:00:00Z")),
            reminders = listOf(
                Reminder(id = 4, message = "Call Dana", remindAt = "2026-08-04T10:00:00Z", recurrence = ReminderRecurrence.WEEKLY),
            ),
        )

        val items = contact.toTimelineItems()

        assertEquals(4, items.size)
        // Newest first.
        assertTrue(items[0] is TimelineItem.ActivityItem)
        assertEquals("New visit", (items[0] as TimelineItem.ActivityItem).activity.title)
        assertTrue(items[1] is TimelineItem.ReminderItem)
        assertTrue(items[2] is TimelineItem.NoteItem)
        assertTrue(items[3] is TimelineItem.ActivityItem)
    }

    @Test
    fun `completed once reminders are excluded from the timeline`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            reminders = listOf(
                Reminder(id = 1, message = "Done once", remindAt = "2026-08-04T10:00:00Z", recurrence = ReminderRecurrence.ONCE, completed = true),
                Reminder(id = 2, message = "Pending once", remindAt = "2026-08-04T10:00:00Z", recurrence = ReminderRecurrence.ONCE, completed = false),
            ),
        )

        val items = contact.toTimelineItems()

        assertEquals(1, items.size)
        assertEquals("Pending once", (items[0] as TimelineItem.ReminderItem).reminder.message)
    }

    @Test
    fun `deleted activities and notes are excluded`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            activities = listOf(Activity(id = 1, title = "Deleted", date = "2026-08-01T10:00:00Z", deleted = true)),
            notes = listOf(Note(id = 2, content = "Deleted note", date = "2026-08-01T10:00:00Z", deleted = true)),
        )

        val items = contact.toTimelineItems()

        assertEquals(0, items.size)
    }

    @Test
    fun `rows without a date sort last`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            activities = listOf(Activity(id = 1, title = "Undated", date = "")),
            notes = listOf(Note(id = 2, content = "Dated", date = "2026-08-01T10:00:00Z")),
        )

        val items = contact.toTimelineItems()

        assertEquals(2, items.size)
        assertTrue(items[0] is TimelineItem.NoteItem)
        assertTrue(items[1] is TimelineItem.ActivityItem)
    }

    @Test
    fun `item keys are unique across types`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            activities = listOf(Activity(id = 7, title = "Call", date = "2026-08-01T10:00:00Z")),
            notes = listOf(Note(id = 7, content = "Note", date = "2026-08-02T10:00:00Z")),
        )

        val keys = contact.toTimelineItems().map { it.key }

        assertEquals(2, keys.distinct().size)
    }

    @Test
    fun `completions join the timeline and sort by completed_at`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            notes = listOf(Note(id = 1, content = "Old note", date = "2026-08-01T10:00:00Z")),
        )
        val completions = listOf(
            ReminderCompletion(id = 10, contactId = 5, message = "Done earlier", completedAt = "2026-08-02T10:00:00Z"),
            ReminderCompletion(id = 11, contactId = 5, message = "Done later", completedAt = "2026-08-05T10:00:00Z"),
        )

        val items = contact.toTimelineItems(completions)

        assertEquals(3, items.size)
        assertTrue(items[0] is TimelineItem.CompletionItem)
        assertEquals("Done later", (items[0] as TimelineItem.CompletionItem).completion.message)
        assertTrue(items[1] is TimelineItem.CompletionItem)
        assertEquals("Done earlier", (items[1] as TimelineItem.CompletionItem).completion.message)
        assertTrue(items[2] is TimelineItem.NoteItem)
    }

    @Test
    fun `completion keys are unique and don't collide with same-id reminder rows`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(name = Name(full = "Dana White")),
            reminders = listOf(
                Reminder(id = 7, message = "Call Dana", remindAt = "2026-08-04T10:00:00Z", recurrence = ReminderRecurrence.WEEKLY),
            ),
        )
        val completions = listOf(
            ReminderCompletion(id = 7, contactId = 5, message = "Done", completedAt = "2026-08-05T10:00:00Z"),
        )

        val keys = contact.toTimelineItems(completions).map { it.key }

        assertEquals(2, keys.distinct().size)
    }
}
