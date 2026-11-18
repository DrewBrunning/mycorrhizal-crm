package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Test

/** Issue #2664 / #2734: the fire-and-forget discard of an unconfirmed import session. */
class ImportSessionCancellerTest {

    private val apiClient = mockk<ApiClient>()

    private fun TestScope.canceller(): Pair<ImportSessionCanceller, TestScope> {
        coEvery { apiClient.cancelImportSession(any()) } returns Result.success(Unit)
        return ImportSessionCanceller(apiClient) to this
    }

    @Test
    fun `cancel posts the session id once however often it is asked`() =
        runTest(StandardTestDispatcher()) {
            val (c, scope) = canceller()

            c.cancel(scope, "s1")
            c.cancel(scope, "s1")
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelImportSession("s1") }
        }

    @Test
    fun `nothing is sent without a session id`() = runTest(StandardTestDispatcher()) {
        val (c, scope) = canceller()

        c.cancel(scope, null)
        c.cancel(scope, "")
        advanceUntilIdle()

        coVerify(exactly = 0) { apiClient.cancelImportSession(any()) }
    }

    @Test
    fun `nothing is sent once a confirm started`() = runTest(StandardTestDispatcher()) {
        val (c, scope) = canceller()

        c.confirmStarted()
        c.cancel(scope, "s1")
        advanceUntilIdle()

        coVerify(exactly = 0) { apiClient.cancelImportSession(any()) }
    }

    @Test
    fun `a confirm the server refused leaves the session cancellable again`() =
        runTest(StandardTestDispatcher()) {
            val (c, scope) = canceller()

            c.confirmStarted()
            c.confirmRefused()
            c.cancel(scope, "s1")
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.cancelImportSession("s1") }
        }

    @Test
    fun `a failing or throwing cancel is swallowed`() = runTest(StandardTestDispatcher()) {
        coEvery { apiClient.cancelImportSession("boom") } throws RuntimeException("net")
        coEvery {
            apiClient.cancelImportSession(
                "fail"
            )
        } returns Result.failure(RuntimeException("x"))
        val c = ImportSessionCanceller(apiClient)

        c.cancel(this, "fail")
        ImportSessionCanceller(apiClient).cancel(this, "boom")
        advanceUntilIdle()

        coVerify(exactly = 1) { apiClient.cancelImportSession("fail") }
        coVerify(exactly = 1) { apiClient.cancelImportSession("boom") }
    }
}
