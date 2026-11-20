package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.model.network.Note

private const val ISO_DAY_LENGTH = 10

/**
 * Notes in the order the lists show them (issue #2616): the note's own date, newest first, with
 * the id breaking ties newest first. Both lists ask the server for `sort=date` (#2744, #2797), so
 * this is the tiebreak and the fallback for an older server that ignores the parameter and pages
 * by `updated_at`, where a note edited long after it was dated would sit out of order. The calendar day (the first 10 characters
 * of the ISO date, as the screens display it) is compared, so a time or offset never reorders two
 * notes that show the same date; a note with no date sorts last.
 */
internal fun List<Note>.sortedByNoteDate(): List<Note> = sortedWith(
    compareByDescending<Note> { it.date?.take(ISO_DAY_LENGTH).orEmpty() }
        .thenByDescending { it.id }
)
