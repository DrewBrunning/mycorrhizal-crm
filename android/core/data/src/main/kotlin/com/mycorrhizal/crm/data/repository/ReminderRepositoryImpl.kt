package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedReminder
import com.mycorrhizal.crm.data.local.CachedReminderDao
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.ApiClient
import javax.inject.Inject

/**
 * Online-first reminder access. Writes go to the server; successful responses
 * are mirrored into the Room cache.
 */
class ReminderRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val dao: CachedReminderDao,
) : ReminderRepository {

    override suspend fun listForContact(contactId: Int): Result<List<Reminder>> {
        val result = apiClient.listContactReminders(contactId)
        val response = result.getOrNull()
        if (response != null) {
            dao.upsertAll(response.reminders.map { it.toCached() })
        }
        return result.map { response -> response.reminders }
    }

    override suspend fun get(id: Int): Result<Reminder> = apiClient.getReminder(id)

    override suspend fun create(contactId: Int, reminder: Reminder): Result<Reminder> {
        val result = apiClient.createReminder(contactId, reminder)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun update(id: Int, reminder: Reminder): Result<Reminder> {
        val result = apiClient.updateReminder(id, reminder)
        result.getOrNull()?.let { dao.upsert(it.toCached()) }
        return result
    }

    override suspend fun complete(id: Int): Result<Reminder?> {
        val result = apiClient.completeReminder(id)
        result.getOrNull()?.reminder?.let { dao.upsert(it.toCached()) }
        // A completed once-reminder is soft-deleted server-side; drop it from
        // the cache so it disappears from the list.
        if (result.getOrNull()?.reminder == null) {
            dao.upsert(CachedReminder(id = id, completed = true, message = null))
        }
        return result.map { it.reminder }
    }

    private fun Reminder.toCached(): CachedReminder = CachedReminder(
        id = id,
        message = message,
        remindAt = remindAt,
        recurrence = recurrence,
        completed = completed,
    )
}
