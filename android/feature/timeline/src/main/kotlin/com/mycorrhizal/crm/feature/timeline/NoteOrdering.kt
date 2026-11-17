package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.model.network.Note

private const val ISO_DAY_LENGTH = 10

/**
 * Notes in the order the lists show them (issue #2616): the note's own date, newest first, with
 * the id breaking ties newest first. The server pages by `updated_at`, so a note edited long
 * after it was dated would otherwise sit out of order. The calendar day (the first 10 characters
 * of the ISO date, as the screens display it) is compared, so a time or offset never reorders two
 * notes that show the same date; a note with no date sorts last.
 */
internal fun List<Note>.sortedByNoteDate(): List<Note> = sortedWith(
    compareByDescending<Note> { it.date?.take(ISO_DAY_LENGTH).orEmpty() }
        .thenByDescending { it.id }
)
