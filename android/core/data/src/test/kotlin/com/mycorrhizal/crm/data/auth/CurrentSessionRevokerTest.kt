package com.mycorrhizal.crm.data.auth

import com.mycorrhizal.crm.model.network.SessionInfo
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Test

/**
 * Issue #957 (finding #3): the server-side session revoke on logout. Uses
 * `GET /sessions`' `current: true` row rather than decoding the JWT locally.
 */
class CurrentSessionRevokerTest {

    @Test
    fun `revokes the session flagged current`() = runTest {
        val api = mockk<ApiClient>()
        coEvery { api.listSessions() } returns Result.success(
            listOf(
                SessionInfo(id = "sid-other", current = false),
                SessionInfo(id = "sid-this", current = true),
            ),
        )
        coEvery { api.revokeSession("sid-this") } returns Result.success(Unit)

        CurrentSessionRevoker(api).revoke()

        coVerify(exactly = 1) { api.revokeSession("sid-this") }
        coVerify(exactly = 0) { api.revokeSession("sid-other") }
    }

    @Test
    fun `does nothing when no session is flagged current`() = runTest {
        val api = mockk<ApiClient>()
        coEvery { api.listSessions() } returns Result.success(
            listOf(SessionInfo(id = "sid-other", current = false)),
        )

        CurrentSessionRevoker(api).revoke()

        coVerify(exactly = 0) { api.revokeSession(any()) }
    }

    @Test
    fun `does nothing when the list call fails`() = runTest {
        val api = mockk<ApiClient>()
        coEvery { api.listSessions() } returns Result.failure(RuntimeException("network error"))

        // Must not throw -- this is a best-effort teardown step.
        CurrentSessionRevoker(api).revoke()

        coVerify(exactly = 0) { api.revokeSession(any()) }
    }

    @Test
    fun `a revoke failure does not throw`() = runTest {
        val api = mockk<ApiClient>()
        coEvery { api.listSessions() } returns Result.success(listOf(SessionInfo(id = "sid-this", current = true)))
        coEvery { api.revokeSession("sid-this") } returns Result.failure(RuntimeException("network error"))

        CurrentSessionRevoker(api).revoke()

        coVerify(exactly = 1) { api.revokeSession("sid-this") }
    }
}
