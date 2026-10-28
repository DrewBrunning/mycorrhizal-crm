package com.mycorrhizal.crm.feature.tracking

import android.app.Application
import android.content.Context
import androidx.test.core.app.ApplicationProvider
import androidx.work.ListenableWorker
import androidx.work.WorkerParameters
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config

/** Issue #2066: the persisted replacement for `onNewToken`'s unstructured launch. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class)
class FcmTokenRegistrationWorkerTest {

    private val context = ApplicationProvider.getApplicationContext<Context>()

    private fun worker(
        manager: DeviceRegistrationManager,
        token: String?
    ): FcmTokenRegistrationWorker {
        val params = mockk<WorkerParameters>(relaxed = true)
        every { params.inputData } returns
            (token?.let { FcmTokenRegistrationWorker.inputFor(it) } ?: androidx.work.Data.EMPTY)
        return FcmTokenRegistrationWorker(context, params, manager)
    }

    @Test
    fun `registers the token carried in the input data`() = runTest {
        val manager = mockk<DeviceRegistrationManager>()
        coEvery { manager.register(tokenOverride = "t1") } returns kotlin.Result.success(Unit)

        assertEquals(ListenableWorker.Result.success(), worker(manager, "t1").doWork())
        coVerify(exactly = 1) { manager.register(tokenOverride = "t1") }
    }

    @Test
    fun `retries when registration fails so the token is not dropped`() = runTest {
        val manager = mockk<DeviceRegistrationManager>()
        coEvery { manager.register(tokenOverride = "t1") } returns
            kotlin.Result.failure(RuntimeException("offline"))

        assertEquals(ListenableWorker.Result.retry(), worker(manager, "t1").doWork())
    }

    @Test
    fun `fails without a token and never calls the server`() = runTest {
        val manager = mockk<DeviceRegistrationManager>()

        assertEquals(ListenableWorker.Result.failure(), worker(manager, null).doWork())
        coVerify(exactly = 0) { manager.register(any()) }
    }
}
