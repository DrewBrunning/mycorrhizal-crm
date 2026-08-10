package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.ContactRemindersResponse
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.model.network.ReminderCompleteResponse
import com.mycorrhizal.crm.model.network.ReminderRecurrence
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ReminderRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: ReminderRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = ReminderRepositoryImpl(apiClient, db.cachedReminderDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun reminder(id: Int, message: String, recurrence: String = ReminderRecurrence.ONCE) =
        Reminder(id = id, message = message, recurrence = recurrence)

    @Test
    fun `listForContact caches reminders and returns them`() = runTest {
        coEvery { apiClient.listContactReminders(5) } returns Result.success(
            ContactRemindersResponse(
                reminders = listOf(reminder(1, "Call Dana"), reminder(2, "Gift")),
            ),
        )

        val result = repository.listForContact(5)

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().size)
        assertEquals("Call Dana", db.cachedReminderDao().getById(1)?.message)
    }

    @Test
    fun `create caches the created reminder`() = runTest {
        coEvery { apiClient.createReminder(5, any()) } returns Result.success(reminder(1, "Call Dana"))

        val result = repository.create(5, reminder(0, "Call Dana"))

        assertTrue(result.isSuccess)
        assertEquals("Call Dana", db.cachedReminderDao().getById(1)?.message)
    }

    @Test
    fun `complete of a recurring reminder returns the rescheduled reminder`() = runTest {
        coEvery { apiClient.completeReminder(1) } returns Result.success(
            ReminderCompleteResponse(
                message = "Reminder completed",
                reminder = reminder(1, "Call Dana", ReminderRecurrence.WEEKLY),
            ),
        )

        val result = repository.complete(1)

        assertTrue(result.isSuccess)
        assertEquals("Call Dana", result.getOrThrow()?.message)
    }

    @Test
    fun `complete of a once reminder returns null`() = runTest {
        coEvery { apiClient.completeReminder(1) } returns Result.success(
            ReminderCompleteResponse(message = "Reminder completed", reminder = null),
        )

        val result = repository.complete(1)

        assertTrue(result.isSuccess)
        assertNull(result.getOrThrow())
    }

    @Test
    fun `complete propagates a failure`() = runTest {
        coEvery { apiClient.completeReminder(999) } returns Result.failure(
            ApiError.Client(404, "Reminder not found"),
        )

        val result = repository.complete(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(404, (error as ApiError.Client).code)
    }
}
