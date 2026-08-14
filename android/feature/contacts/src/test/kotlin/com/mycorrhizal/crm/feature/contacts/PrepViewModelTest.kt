package com.mycorrhizal.crm.feature.contacts

import androidx.lifecycle.SavedStateHandle
import app.cash.turbine.test
import com.mycorrhizal.crm.model.network.BriefingCadence
import com.mycorrhizal.crm.model.network.BriefingCadenceHealth
import com.mycorrhizal.crm.model.network.ContactBriefing
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class PrepViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private fun newViewModel(
        apiClient: ApiClient = mockk(),
        contactId: Int = 7,
    ): PrepViewModel {
        return PrepViewModel(apiClient, SavedStateHandle(mapOf("contactId" to contactId)))
    }

    @Test
    fun `loading transitions to success with the fetched briefing`() = runTest(mainDispatcherRule.testDispatcher) {
        val apiClient = mockk<ApiClient>()
        val briefing = ContactBriefing(contactId = 7, uid = "u7", name = "Alice Wonder")
        coEvery { apiClient.getBriefing(7) } returns Result.success(briefing)

        val viewModel = newViewModel(apiClient)

        // State machine: initial → loading → success (M11 test case 2). The
        // three emissions are distinct StateFlow values, so Turbine sees all of them.
        viewModel.uiState.test {
            val initial = awaitItem()
            assertFalse(initial.isLoading)
            assertNull(initial.briefing)

            advanceUntilIdle()

            val loading = awaitItem()
            assertTrue(loading.isLoading)

            val success = awaitItem()
            assertFalse(success.isLoading)
            assertNull(success.error)
            assertEquals("Alice Wonder", success.briefing?.name)
            assertTrue(success.briefing!!.recentNotes.isEmpty())
            assertTrue(success.briefing!!.openAgendaItems.isEmpty())
        }
        coVerify(exactly = 1) { apiClient.getBriefing(7) }
    }

    @Test
    fun `an empty-history briefing is success, not an error`() = runTest(mainDispatcherRule.testDispatcher) {
        // M11 test case 1's ViewModel half: the parse-side normalization is pinned by the
        // ApiClient MockWebServer test (absent/null/[] all decode to empty lists); here we
        // pin that a briefing with nothing populated is still a successful state, so the
        // screen renders its empty states instead of the error/retry surface.
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getBriefing(7) } returns Result.success(
            ContactBriefing(contactId = 7, uid = "u7", name = "Empty"),
        )

        val viewModel = newViewModel(apiClient)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.error)
        assertTrue(state.briefing?.recentNotes?.isEmpty() ?: false)
        assertTrue(state.briefing?.relationships?.isEmpty() ?: false)
        assertTrue(state.briefing?.upcomingDates?.isEmpty() ?: false)
        assertNull(state.briefing?.cadence)
    }

    @Test
    fun `a network failure yields the error state`() = runTest(mainDispatcherRule.testDispatcher) {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getBriefing(7) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))

        val viewModel = newViewModel(apiClient)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.briefing)
        assertEquals("No connection", state.error)
    }

    @Test
    fun `retry re-issues the briefing call after a failure`() = runTest(mainDispatcherRule.testDispatcher) {
        val apiClient = mockk<ApiClient>()
        coEvery { apiClient.getBriefing(7) } returns
            Result.failure(ApiError.Server(500, "boom")) andThen
            Result.success(ContactBriefing(contactId = 7, uid = "u7", name = "Recovered"))

        val viewModel = newViewModel(apiClient)
        advanceUntilIdle()
        assertEquals("Server error (500)", viewModel.uiState.value.error)

        viewModel.load()
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.error)
        assertEquals("Recovered", state.briefing?.name)
        coVerify(exactly = 2) { apiClient.getBriefing(7) }
    }

    @Test
    fun `health card reads server-provided cadence fields unchanged`() = runTest(mainDispatcherRule.testDispatcher) {
        // M11 test case 3: the cadence/health block renders server-provided values; it must
        // never recompute health locally. The ViewModel must pass the BriefingCadence
        // through untouched — assert every health field the screen renders is byte-identical
        // to what the backend returned.
        val apiClient = mockk<ApiClient>()
        val cadence = BriefingCadence(
            health = BriefingCadenceHealth(
                hasQualifyingInteraction = true,
                lastInteraction = "2026-08-10T14:00:00Z",
                nextDue = "2026-09-09T14:00:00Z",
                overdueBy = 2,
            ),
        )
        coEvery { apiClient.getBriefing(7) } returns Result.success(
            ContactBriefing(contactId = 7, uid = "u7", name = "Alice", cadence = cadence),
        )

        val viewModel = newViewModel(apiClient)
        advanceUntilIdle()

        val served = viewModel.uiState.value.briefing?.cadence
        assertEquals(true, served?.health?.hasQualifyingInteraction)
        assertEquals(2, served?.health?.overdueBy)
        assertEquals("2026-08-10T14:00:00Z", served?.health?.lastInteraction)
        assertEquals("2026-09-09T14:00:00Z", served?.health?.nextDue)
    }

    @Test
    fun `missing contact id is an empty-not-error state`() = runTest(mainDispatcherRule.testDispatcher) {
        val apiClient = mockk<ApiClient>()
        val viewModel = newViewModel(apiClient, contactId = 0)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertNull(state.error)
        assertNull(state.briefing)
        coVerify(exactly = 0) { apiClient.getBriefing(any()) }
    }
}
