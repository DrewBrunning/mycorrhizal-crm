package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.app.NotificationManager
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.BirthdaysResponse
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.OverdueCadencesResponse
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.Shadows
import org.robolectric.annotation.Config

/**
 * CadenceCheckWorker and BirthdayCheckWorker, copied from
 * ReminderNotificationWorkerTest.kt's pattern (same source file,
 * NotificationWorkers.kt).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class NotificationWorkersTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()
    private val manager = context.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
    private val shadow = Shadows.shadowOf(manager)

    // -- CadenceCheckWorker --

    @Test
    fun `cadence worker posts one notification per overdue contact under a distinct id`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listOverdueCadences() } returns Result.success(
            OverdueCadencesResponse(
                overdue = listOf(
                    OverdueCadence(contactId = 1, contactName = "Jane"),
                    OverdueCadence(contactId = 2, contactName = "Bob"),
                ),
            ),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = CadenceCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertNotNull(shadow.getNotification(AlertNotificationIds.CADENCE))
        assertNotNull(shadow.getNotification(AlertNotificationIds.CADENCE + 1))
    }

    @Test
    fun `cadence worker falls back to a generic name when the contact name is blank`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listOverdueCadences() } returns Result.success(
            OverdueCadencesResponse(overdue = listOf(OverdueCadence(contactId = 1, contactName = ""))),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = CadenceCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertNotNull(shadow.getNotification(AlertNotificationIds.CADENCE))
    }

    @Test
    fun `cadence worker posts nothing when notifications are disabled`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns false

        val worker = CadenceCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
    }

    @Test
    fun `cadence worker posts nothing for an empty overdue list`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listOverdueCadences() } returns Result.success(OverdueCadencesResponse(overdue = emptyList()))
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = CadenceCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
    }

    // -- BirthdayCheckWorker --

    @Test
    fun `birthday worker posts one notification per birthday under a distinct id`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listUpcomingBirthdays() } returns Result.success(
            BirthdaysResponse(
                birthdays = listOf(
                    Birthday(contactId = 1, name = "Jane"),
                    Birthday(contactId = 2, name = "Bob"),
                ),
            ),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = BirthdayCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertNotNull(shadow.getNotification(AlertNotificationIds.BIRTHDAYS))
        assertNotNull(shadow.getNotification(AlertNotificationIds.BIRTHDAYS + 1))
    }

    @Test
    fun `birthday worker falls back to a generic name when the name is blank`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listUpcomingBirthdays() } returns Result.success(
            BirthdaysResponse(birthdays = listOf(Birthday(contactId = 1, name = ""))),
        )
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = BirthdayCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertNotNull(shadow.getNotification(AlertNotificationIds.BIRTHDAYS))
    }

    @Test
    fun `birthday worker posts nothing when notifications are disabled`() = runTest {
        val apiClient = mockk<ApiClient>(relaxed = true)
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns false

        val worker = BirthdayCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
    }

    @Test
    fun `birthday worker posts nothing for an empty list`() = runTest {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.listUpcomingBirthdays() } returns Result.success(BirthdaysResponse(birthdays = emptyList()))
        val settings = mockk<TrackingSettingsRepository>()
        coEvery { settings.notificationsEnabled() } returns true

        val worker = BirthdayCheckWorker(
            appContext = context,
            workerParams = mockk(relaxed = true),
            apiClient = apiClient,
            trackingSettings = settings,
        )
        worker.doWork()

        assertTrue(shadow.allNotifications.isEmpty())
    }
}
