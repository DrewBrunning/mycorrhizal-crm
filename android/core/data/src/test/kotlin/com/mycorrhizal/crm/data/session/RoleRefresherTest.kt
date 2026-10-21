package com.mycorrhizal.crm.data.session

import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.network.ForbiddenNotifier
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.UnconfinedTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Issue #1826: an authenticated 403 re-reads the user's role once per debounce
 * window, applies a changed `isAdmin` through `SessionManager.setProfile`, and
 * announces a demotion; a 403 that leaves the role unchanged touches nothing.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class RoleRefresherTest {

    private class Harness(
        scope: TestScope,
        sessionAdmin: Boolean = true,
        loggedIn: Boolean = true,
        private val serverAdmin: Boolean? = false,
        throwOnFetch: Boolean = false,
        profileIdAfterFetch: String = "p1",
    ) {
        val notifier = ForbiddenNotifier()
        var fetches = 0
        var now = 1_000L
        val demotions = mutableListOf<Unit>()
        private var profileReads = 0
        val manager = mockk<SessionManager>(relaxed = true) {
            every { observeSession() } returns flowOf(SessionState(isLoggedIn = loggedIn, isAdmin = sessionAdmin))
            // First read is before the fetch, the second after it.
            coEvery { activeProfileId() } answers { if (profileReads++ == 0) "p1" else profileIdAfterFetch }
        }
        val refresher = RoleRefresher(
            forbiddenNotifier = notifier,
            sessionManager = manager,
            fetchIsAdmin = {
                fetches++
                if (throwOnFetch) error("offline")
                serverAdmin
            },
            debounceMillis = 30_000L,
            nowMillis = { now },
        )

        init {
            refresher.start(scope)
            scope.backgroundScope.launch(UnconfinedTestDispatcher(scope.testScheduler)) {
                refresher.demotions.collect { demotions += it }
            }
        }
    }

    @Test
    fun `a burst of 403s within one window triggers a single re-fetch`() = runTest {
        val h = Harness(this)

        repeat(5) { h.notifier.onForbidden() }
        advanceUntilIdle()

        assertEquals(1, h.fetches)
    }

    @Test
    fun `a 403 after the window has passed re-fetches again`() = runTest {
        val h = Harness(this)

        h.notifier.onForbidden()
        h.now += 29_999
        h.notifier.onForbidden()
        advanceUntilIdle()
        assertEquals("still inside the window", 1, h.fetches)

        h.now += 1
        h.notifier.onForbidden()
        advanceUntilIdle()
        assertEquals("the window is exactly debounceMillis long", 2, h.fetches)
    }

    @Test
    fun `a demotion updates isAdmin and announces it`() = runTest {
        val h = Harness(this, sessionAdmin = true, serverAdmin = false)

        h.notifier.onForbidden()
        advanceUntilIdle()

        coVerify(exactly = 1) { h.manager.setProfile(ProfileUpdate(isAdmin = false)) }
        assertEquals(1, h.demotions.size)
    }

    @Test
    fun `a 403 that leaves the role unchanged writes nothing and announces nothing`() = runTest {
        val h = Harness(this, sessionAdmin = true, serverAdmin = true)

        h.notifier.onForbidden()
        advanceUntilIdle()

        assertEquals(1, h.fetches)
        coVerify(exactly = 0) { h.manager.setProfile(any()) }
        assertEquals(0, h.demotions.size)
    }

    @Test
    fun `a promotion is applied but is not a demotion`() = runTest {
        val h = Harness(this, sessionAdmin = false, serverAdmin = true)

        h.notifier.onForbidden()
        advanceUntilIdle()

        coVerify(exactly = 1) { h.manager.setProfile(ProfileUpdate(isAdmin = true)) }
        assertEquals(0, h.demotions.size)
    }

    @Test
    fun `no session means no re-fetch result is applied`() = runTest {
        val h = Harness(this, loggedIn = false)

        h.notifier.onForbidden()
        advanceUntilIdle()

        assertEquals(0, h.fetches)
        coVerify(exactly = 0) { h.manager.setProfile(any()) }
    }

    @Test
    fun `an unreadable profile leaves the session alone`() = runTest {
        val h = Harness(this, serverAdmin = null)

        h.notifier.onForbidden()
        advanceUntilIdle()

        assertEquals(1, h.fetches)
        coVerify(exactly = 0) { h.manager.setProfile(any()) }
        assertEquals(0, h.demotions.size)
    }

    @Test
    fun `a failing fetch is swallowed and still consumes the window`() = runTest {
        val h = Harness(this, throwOnFetch = true)

        h.notifier.onForbidden()
        h.notifier.onForbidden()
        advanceUntilIdle()

        assertEquals(1, h.fetches)
        coVerify(exactly = 0) { h.manager.setProfile(any()) }
    }

    @Test
    fun `a result for a profile that is no longer active is dropped`() = runTest {
        val h = Harness(this, sessionAdmin = true, serverAdmin = false, profileIdAfterFetch = "p2")

        h.notifier.onForbidden()
        advanceUntilIdle()

        coVerify(exactly = 0) { h.manager.setProfile(any()) }
        assertEquals(0, h.demotions.size)
    }
}
