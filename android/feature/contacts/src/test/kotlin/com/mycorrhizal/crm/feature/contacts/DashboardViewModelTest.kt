package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.Birthday
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.DashboardRandomContact
import com.mycorrhizal.crm.model.network.DashboardReminder
import com.mycorrhizal.crm.model.network.DashboardResponse
import com.mycorrhizal.crm.model.network.OverdueCadence
import com.mycorrhizal.crm.model.network.ReminderCompleteResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.every
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class DashboardViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private fun newViewModel(): Triple<DashboardViewModel, ApiClient, AuthRepository> {
        val apiClient = mockk<ApiClient>()
        val authRepository = mockk<AuthRepository>()
        every { authRepository.observeSession() } returns flowOf(SessionState())
        return Triple(DashboardViewModel(apiClient, authRepository), apiClient, authRepository)
    }

    private fun fullDashboard() = DashboardResponse(
        birthdays = listOf(Birthday(name = "Alice", contactId = 1L)),
        randomContacts = listOf(DashboardRandomContact(id = 3, firstname = "Bob", lastname = "Smith", nickname = "Bobby")),
        upcomingReminders = listOf(
            DashboardReminder(id = 7, message = "Call Dana", remindAt = "2026-08-15T09:00:00Z", contactId = 3, contactName = "Bobby Smith"),
        ),
        overdue = listOf(
            OverdueCadence(
                policy = CadencePolicy(id = "c1", entityId = "u3"),
                health = CadenceHealth(overdueBy = 3),
                contactId = 3L,
                contactName = "Bobby Smith",
            ),
        ),
        // Issue #212: the favorites quick-access block (web #173).
        favorites = listOf(
            DashboardRandomContact(id = 9, firstname = "Zebra", lastname = "Smith", nickname = "Z"),
        ),
    )

    @Test
    fun `one getDashboard call populates every widget and the legacy endpoints are never called`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(fullDashboard())

            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertFalse(state.isLoading)
            assertEquals(1, state.birthdays.size)
            assertEquals(1, state.upcomingReminders.size)
            assertEquals(1, state.randomContacts.size)
            assertEquals(1, state.overdueCadences.size)
            assertEquals(1, state.favorites.size)
            assertEquals("Zebra", state.favorites[0].firstname)
            // The M3 embedded contact name survives into the widget.
            assertEquals("Bobby Smith", state.upcomingReminders[0].contactName)

            coVerify(exactly = 1) { apiClient.getDashboard() }
            // The legacy fan-out M3 replaced must be absent — a half-migrated
            // dashboard that still calls any of these would pass otherwise.
            coVerify(exactly = 0) { apiClient.listUpcomingBirthdays() }
            coVerify(exactly = 0) { apiClient.listOverdueCadences() }
            coVerify(exactly = 0) { apiClient.listUpcomingReminders() }
        }

    @Test
    fun `an empty composite renders empty widgets without an error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(DashboardResponse())

            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertFalse(state.isLoading)
            assertNull(state.error)
            assertTrue(state.birthdays.isEmpty())
            assertTrue(state.upcomingReminders.isEmpty())
            assertTrue(state.randomContacts.isEmpty())
            assertTrue(state.overdueCadences.isEmpty())
            assertTrue(state.favorites.isEmpty())
        }

    @Test
    fun `a dashboard fetch failure surfaces the error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.failure(ApiError.Server(500, "boom"))

            advanceUntilIdle()

            val state = viewModel.uiState.value
            assertFalse(state.isLoading)
            assertEquals("Server error (500)", state.error)
        }

    @Test
    fun `completing a reminder removes it from the widget before the call resolves`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(
                DashboardResponse(
                    upcomingReminders = listOf(
                        DashboardReminder(id = 1, message = "Call Dana", contactName = "Bobby Smith"),
                        DashboardReminder(id = 2, message = "Water plants", contactName = "Alice"),
                    ),
                ),
            )
            coEvery { apiClient.completeReminder(1, false) } coAnswers {
                // The optimistic removal must have already happened by the
                // time the API call executes — a fetch-then-update
                // implementation would observe the reminder still present.
                assertTrue(viewModel.uiState.value.upcomingReminders.none { it.id == 1 })
                Result.success(ReminderCompleteResponse(message = "Reminder completed"))
            }
            advanceUntilIdle()

            viewModel.completeReminder(1)
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.completeReminder(1, false) }
            assertEquals(listOf(2), viewModel.uiState.value.upcomingReminders.map { it.id })
            assertNull(viewModel.uiState.value.completingId)
        }

    @Test
    fun `a failed complete restores the reminder at its original position`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(
                DashboardResponse(
                    upcomingReminders = listOf(
                        DashboardReminder(id = 1, message = "Call Dana", contactName = "Bobby Smith"),
                        DashboardReminder(id = 2, message = "Water plants", contactName = "Alice"),
                        DashboardReminder(id = 3, message = "Pay rent", contactName = "Alice"),
                    ),
                ),
            )
            coEvery { apiClient.completeReminder(2, false) } returns
                Result.failure(ApiError.Server(500, "boom"))
            advanceUntilIdle()

            viewModel.completeReminder(2)
            advanceUntilIdle()

            val state = viewModel.uiState.value
            // Restored to its original middle position, not merely re-appended.
            assertEquals(listOf(1, 2, 3), state.upcomingReminders.map { it.id })
            // A failed action is a transient actionError, never the dashboard-wide
            // load error — the widgets must keep rendering.
            assertEquals("Server error (500)", state.actionError)
            assertNull(state.error)
            assertNull(state.completingId)
        }

    @Test
    fun `onActionErrorShown clears the transient action error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(fullDashboard())
            coEvery { apiClient.completeReminder(7, false) } returns
                Result.failure(ApiError.Server(500, "boom"))
            advanceUntilIdle()

            viewModel.completeReminder(7)
            advanceUntilIdle()
            assertEquals("Server error (500)", viewModel.uiState.value.actionError)

            viewModel.onActionErrorShown()
            assertNull(viewModel.uiState.value.actionError)
        }

    @Test
    fun `skipping a reminder calls complete with skip true and removes it optimistically`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val (viewModel, apiClient, _) = newViewModel()
            coEvery { apiClient.getDashboard() } returns Result.success(fullDashboard())
            coEvery { apiClient.completeReminder(7, true) } returns
                Result.success(ReminderCompleteResponse(message = "Reminder skipped"))
            advanceUntilIdle()

            viewModel.completeReminder(7, skip = true)
            advanceUntilIdle()

            coVerify(exactly = 1) { apiClient.completeReminder(7, true) }
            assertTrue(viewModel.uiState.value.upcomingReminders.isEmpty())
            assertNull(viewModel.uiState.value.completingId)
        }

    @Test
    fun `dateFormat reflects the signed-in user's date_format preference`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val apiClient = mockk<ApiClient>()
            val authRepository = mockk<AuthRepository>()
            every { authRepository.observeSession() } returns flowOf(SessionState(dateFormat = "us"))
            coEvery { apiClient.getDashboard() } returns Result.success(DashboardResponse())

            val viewModel = DashboardViewModel(apiClient, authRepository)
            advanceUntilIdle()

            assertEquals("us", viewModel.uiState.value.dateFormat)
        }

    @Test
    fun `a retry tapped while a load is in flight is a no-op`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val apiClient = mockk<ApiClient>()
            val authRepository = mockk<AuthRepository>()
            every { authRepository.observeSession() } returns flowOf(SessionState())
            // Init load fails (drives the error state), the retry succeeds.
            coEvery { apiClient.getDashboard() } returns
                Result.failure(ApiError.Server(500, "boom")) andThen
                Result.success(DashboardResponse())

            val viewModel = DashboardViewModel(apiClient, authRepository)
            advanceUntilIdle()
            assertEquals("Server error (500)", viewModel.uiState.value.error)

            viewModel.load()
            viewModel.load()
            advanceUntilIdle()

            // Exactly one retry despite two taps (the second saw the in-flight Job and bailed).
            assertNull(viewModel.uiState.value.error)
            coVerify(exactly = 2) { apiClient.getDashboard() }
        }
}
