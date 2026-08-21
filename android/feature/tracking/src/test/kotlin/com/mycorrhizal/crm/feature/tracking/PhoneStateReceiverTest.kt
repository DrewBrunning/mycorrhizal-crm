package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import android.content.Intent
import android.telephony.TelephonyManager
import androidx.test.core.app.ApplicationProvider
import androidx.work.WorkManager
import androidx.work.testing.WorkManagerTestInitHelper
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
class PhoneStateReceiverTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    @Before
    fun setup() {
        // feature/tracking has no manifest of its own, so WorkManager does not
        // auto-initialize under Robolectric here -- see TrackingWorkerSchedulerTest.
        WorkManagerTestInitHelper.initializeTestWorkManager(context)
    }

    @Test
    fun `a phone-state-changed broadcast enqueues the call-log sync worker`() {
        PhoneStateReceiver().onReceive(context, Intent(TelephonyManager.ACTION_PHONE_STATE_CHANGED))

        // Under WorkManagerTestInitHelper's synchronous executor the request may
        // already be in a terminal state by the time we check -- see
        // TrackingWorkerSchedulerTest. Non-empty proves it was enqueued at all.
        val workInfos = WorkManager.getInstance(context)
            .getWorkInfosForUniqueWork(PhoneStateReceiver.UNIQUE_CALL_LOG_SYNC).get()
        assertTrue(workInfos.isNotEmpty())
    }

    @Test
    fun `a non-matching action is a no-op`() {
        PhoneStateReceiver().onReceive(context, Intent("some.other.action"))

        val workInfos = WorkManager.getInstance(context)
            .getWorkInfosForUniqueWork(PhoneStateReceiver.UNIQUE_CALL_LOG_SYNC).get()
        assertTrue(workInfos.isEmpty())
    }
}
