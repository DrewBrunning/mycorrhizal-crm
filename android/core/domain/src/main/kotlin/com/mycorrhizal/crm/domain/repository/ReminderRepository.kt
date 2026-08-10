package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.Reminder

/**
 * Reminder data access. Online-first: writes go to the server and the
 * returned record is mirrored into the local cache.
 */
interface ReminderRepository {
    /** A contact's reminders. */
    suspend fun listForContact(contactId: Int): Result<List<Reminder>>

    /** A single reminder. */
    suspend fun get(id: Int): Result<Reminder>

    /** Create a reminder on a contact; returns the created reminder. */
    suspend fun create(contactId: Int, reminder: Reminder): Result<Reminder>

    /** Update a reminder; returns the updated reminder. */
    suspend fun update(id: Int, reminder: Reminder): Result<Reminder>

    /** Mark a reminder complete; recurring reminders reschedule and return the reminder. */
    suspend fun complete(id: Int): Result<Reminder?>
}
