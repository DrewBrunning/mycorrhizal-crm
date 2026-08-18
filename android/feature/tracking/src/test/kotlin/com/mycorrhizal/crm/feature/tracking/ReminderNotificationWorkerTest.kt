package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ContactRemindersResponse
import com.mycorrhizal.crm.model.network.Reminder
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

/**
 * Pins the ReminderNotificationWorker's post-issue-#152 behavior: each reminder
 * is posted under the (reminder_id, due_at) idempotent notification id so the
 * FCM push and this polling worker replace rather than duplicate each other at
 * the poll boundary, and a disabled notification opt-in is a silent no-op.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class ReminderNotificationWorkerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val shadow = Shadows.shadowOf(manager)

    @Test
    fun `posts a reminder under the idempotent notification id`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listUpcomingReminders() } returns Result.success(
            ContactRemindersResponse(
                reminders = listOf(
                    Reminder(id = 42, remindAt = "2026-08-18T14:14:28Z", message = "Call Jane", contactId = 7),
                ),
            ),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = ReminderNotificationWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        val expectedId = AlertNotificationIds.forReminder(42, "2026-08-18T14:14:28Z")
        assertNotNull("notification must land under the idempotent id", shadow.getNotification(expectedId))
    }

    @Test
    fun `a disabled notification opt-in posts nothing`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns false

        val worker = ReminderNotificationWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
    }

    @Test
    fun `an empty upcoming-reminders list posts nothing`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listUpcomingReminders() } returns Result.success(
            ContactRemindersResponse(reminders = emptyList()),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = ReminderNotificationWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
        assertNull(shadow.getNotification(0))
    }
}
