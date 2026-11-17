package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.model.network.Note
import org.junit.Assert.assertEquals
import org.junit.Test

class NoteOrderingTest {

    @Test
    fun `sorts by date newest first, not by creation order`() {
        val notes = listOf(
            Note(id = 4, date = "2026-11-05T00:00:00Z"),
            Note(id = 3, date = "2026-11-08T00:00:00Z"),
            Note(id = 2, date = "2026-10-24T00:00:00Z"),
            Note(id = 1, date = "2026-11-12T00:00:00Z")
        )

        assertEquals(listOf(1, 3, 4, 2), notes.sortedByNoteDate().map { it.id })
    }

    @Test
    fun `the same day is a tie broken by id newest first whatever the time or offset`() {
        val notes = listOf(
            Note(id = 1, date = "2026-11-05T23:30:00Z"),
            Note(id = 2, date = "2026-11-05T00:00:00Z"),
            Note(id = 3, date = "2026-11-05T08:00:00+02:00")
        )

        assertEquals(listOf(3, 2, 1), notes.sortedByNoteDate().map { it.id })
    }

    @Test
    fun `an undated note sorts after every dated one`() {
        val notes = listOf(
            Note(id = 9, date = null),
            Note(id = 1, date = "2020-01-01T00:00:00Z"),
            Note(id = 8, date = "")
        )

        assertEquals(listOf(1, 9, 8), notes.sortedByNoteDate().map { it.id })
    }

    @Test
    fun `an empty list stays empty`() {
        assertEquals(emptyList<Note>(), emptyList<Note>().sortedByNoteDate())
    }
}
