package com.mycorrhizal.crm

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

@OptIn(ExperimentalCoroutinesApi::class)
class MainViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    @Test
    fun `logged-out session is the initial state`() = runTest(mainDispatcherRule.testDispatcher) {
        val sessionManager = mockk<SessionManager> {
            every { observeSession() } returns MutableStateFlow(SessionState())
        }

        val vm = MainViewModel(sessionManager)
        advanceUntilIdle()

        val state = vm.session.value
        assertFalse(state.isLoggedIn)
        assertTrue(state.userId == null)
    }

    @Test
    fun `a logged-in session is surfaced to the session flow`() = runTest(mainDispatcherRule.testDispatcher) {
        val flow = MutableStateFlow(SessionState())
        val sessionManager = mockk<SessionManager> {
            every { observeSession() } returns flow
        }

        val vm = MainViewModel(sessionManager)
        advanceUntilIdle()

        flow.value = SessionState(
            serverUrl = "https://crm.example.com",
            isLoggedIn = true,
            userId = 7,
            username = "alice",
            isAdmin = true,
            language = "de",
        )
        advanceUntilIdle()

        val state = vm.session.value
        assertTrue(state.isLoggedIn)
        assertEquals(7, state.userId)
        assertEquals("alice", state.username)
        assertTrue(state.isAdmin)
        assertEquals("de", state.language)
    }
}
