package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.work.WorkInfo
import androidx.work.WorkManager
import androidx.work.testing.WorkManagerTestInitHelper
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/**
 * feature/tracking's own test manifest doesn't declare WorkManager's
 * androidx.startup.InitializationProvider (it has no manifest of its own at
 * all -- see NotificationBuilderTest's launch-intent stub for the same root
 * cause), so WorkManager does NOT auto-initialize here. Plain
 * WorkManager.initialize(...) also isn't enough on its own: it throws
 * "already initialized" on the second call within the same Robolectric JVM
 * fork, because the singleton persists across test methods/classes here (the
 * same class of leak as DataStore's -- see TrackingSettingsRepositoryImplTest
 * and DataStoreSessionPrefsStorageTest in core/data). WorkManagerTestInitHelper
 * is built specifically to reset cleanly between tests; first
 * WorkManager-assertion test in this repo.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class TrackingWorkerSchedulerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Before
    fun setup() {
        WorkManagerTestInitHelper.initializeTestWorkManager(context)
    }

    private fun uniqueWorkState(name: String): List<WorkInfo> =
        WorkManager.getInstance(context).getWorkInfosForUniqueWork(name).get()

    // WorkManagerTestInitHelper wires a synchronous executor, so enqueued work
    // may already be in a terminal state (succeeded/failed -- these bare
    // Hilt-assisted workers have no real WorkerFactory here) by the time we
    // check. The point of this test is that schedulePeriodic() enqueued a
    // request under the right unique name at all, not what state it's since
    // moved to -- the workers' own doWork() behavior is covered by
    // ReminderNotificationWorkerTest/NotificationWorkersTest/CallLogSyncWorkerTest.
    @Test
    fun `schedulePeriodic enqueues all four periodic workers`() {
        TrackingWorkerScheduler.schedulePeriodic(context)

        assertTrue(uniqueWorkState(TrackingWorkerScheduler.UNIQUE_INTERACTION_SYNC).isNotEmpty())
        assertTrue(uniqueWorkState(TrackingWorkerScheduler.UNIQUE_REMINDER_CHECK).isNotEmpty())
        assertTrue(uniqueWorkState(TrackingWorkerScheduler.UNIQUE_CADENCE_CHECK).isNotEmpty())
        assertTrue(uniqueWorkState(TrackingWorkerScheduler.UNIQUE_BIRTHDAY_CHECK).isNotEmpty())
    }

    @Test
    fun `schedulePeriodic is idempotent -- calling it twice does not duplicate work`() {
        TrackingWorkerScheduler.schedulePeriodic(context)
        TrackingWorkerScheduler.schedulePeriodic(context)

        assertEquals(1, uniqueWorkState(TrackingWorkerScheduler.UNIQUE_INTERACTION_SYNC).size)
    }
}
