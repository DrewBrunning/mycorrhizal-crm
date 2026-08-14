package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.local.CachedReminder
import com.mycorrhizal.crm.data.local.CachedReminderDao
import com.mycorrhizal.crm.domain.repository.ReminderRepository
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompletion
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
        val response = result.getOrNull()
        val rescheduled = response?.reminder
        if (response != null) {
            if (rescheduled != null) {
                // Recurring reminder: rescheduled, replace the cached row.
                dao.upsert(rescheduled.toCached())
            } else {
                // Once reminder: soft-deleted server-side, drop it from the cache
                // so it doesn't reappear as a blank row on offline reads.
                dao.deleteByIds(listOf(id))
            }
        }
        return result.map { it.reminder }
    }

    override suspend fun delete(id: Int): Result<Unit> {
        val result = apiClient.deleteReminder(id)
        if (result.isSuccess) dao.deleteByIds(listOf(id))
        return result
    }

    override suspend fun listCompletions(contactId: Int): Result<List<ReminderCompletion>> =
        apiClient.listContactReminderCompletions(contactId).map { it.completions }

    override suspend fun deleteCompletion(id: Int): Result<Unit> =
        apiClient.deleteReminderCompletion(id)

    private fun Reminder.toCached(): CachedReminder = CachedReminder(
        id = id,
        message = message,
        remindAt = remindAt,
        recurrence = recurrence,
        completed = completed,
    )
}
