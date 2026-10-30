package com.mycorrhizal.crm.feature.timeline

import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.GeoPulseRepository
import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.GeoPulseConfigResponse
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

/** A [GeoPulseRepository] whose `GET /geopulse/config` answers [config]. */
internal fun geoPulseWith(config: Result<GeoPulseConfigResponse>): GeoPulseRepository =
    mockk<GeoPulseRepository>().also { coEvery { it.getConfig() } returns config }

// M9 item 1: the Activities drawer inbox — GET /activities?include=contacts (every contact's
// activities), distinct from ActivitiesViewModel's per-contact list.
class ActivitiesInboxViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val activityRepository = mockk<ActivityRepository>()
    private val geoPulseRepository = geoPulseWith(Result.failure(ApiError.Server(500, "x")))

    @Test
    fun `loads every contact's activities on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(
                    Activity(id = 1, title = "Coffee with Dana"),
                    Activity(id = 2, title = "Call with Carol")
                ),
                nextCursor = null
            )
        )

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertFalse(state.isLoading)
        assertEquals(2, state.activities.size)
        assertNull(state.error)
    }

    @Test
    fun `failure surfaces the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listAll() } returns Result.failure(
            ApiError.Client(500, "server error")
        )

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error?.message)
        assertTrue(vm.uiState.value.activities.isEmpty())
    }

    // Ticket's pagination trap: a test that only asserts "more items appeared" would pass
    // against a re-fetch of page 1 — assert both pages are present with no duplicates.
    @Test
    fun `loadMore appends the next page without duplicating`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(Activity(id = 1, title = "One")),
                nextCursor = "cursor-2"
            )
        )
        coEvery { activityRepository.listAll(cursor = "cursor-2") } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(Activity(id = 2, title = "Two")),
                nextCursor = null
            )
        )

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        val ids = vm.uiState.value.activities.map { it.id }
        assertEquals(listOf(1, 2), ids)
        assertNull(vm.uiState.value.nextCursor)
        coVerify(exactly = 1) { activityRepository.listAll(cursor = "cursor-2") }
    }

    @Test
    fun `loadMore is a no-op with no next cursor`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(Activity(id = 1, title = "One")),
                nextCursor = null
            )
        )

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()
        vm.loadMore()
        advanceUntilIdle()

        // Only the initial load() call — loadMore() short-circuited on a null cursor.
        coVerify(exactly = 1) { activityRepository.listAll(cursor = null, limit = null) }
    }

    @Test
    fun `delete removes the activity from the list on success`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(
                    Activity(id = 1, title = "One"),
                    Activity(id = 2, title = "Two")
                ),
                nextCursor = null
            )
        )
        coEvery { activityRepository.delete(1, any()) } returns Result.success(Unit)

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()

        assertEquals(listOf(2), vm.uiState.value.activities.map { it.id })
        assertNull(vm.uiState.value.deletingId)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `a failed delete keeps the row and surfaces the error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(Activity(id = 1, title = "One")),
                nextCursor = null
            )
        )
        coEvery {
            activityRepository.delete(
                1,
                any()
            )
        } returns Result.failure(ApiError.Client(500, "delete failed"))

        val vm = ActivitiesInboxViewModel(activityRepository, geoPulseRepository)
        advanceUntilIdle()
        vm.delete(1)
        advanceUntilIdle()

        assertEquals(listOf(1), vm.uiState.value.activities.map { it.id })
        assertNull(vm.uiState.value.deletingId)
        assertEquals("delete failed", vm.uiState.value.error?.message)
    }

    // Issue #2205 (web #2179): the "Log from location history" action only shows once GeoPulse is
    // configured; a failed or unconfigured config read keeps it hidden (safe default).
    private fun vmWithGeoPulse(config: Result<GeoPulseConfigResponse>): ActivitiesInboxViewModel {
        coEvery { activityRepository.listAll() } returns Result.success(
            ActivitiesPage(activitiesRaw = emptyList(), nextCursor = null)
        )
        return ActivitiesInboxViewModel(activityRepository, geoPulseWith(config))
    }

    @Test
    fun `geoPulseConfigured is true when the config has an api key`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = vmWithGeoPulse(
            Result.success(GeoPulseConfigResponse(baseUrl = "https://g.example", hasApiKey = true))
        )
        advanceUntilIdle()
        assertTrue(vm.uiState.value.geoPulseConfigured)
    }

    @Test
    fun `geoPulseConfigured is false when no api key is stored`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = vmWithGeoPulse(Result.success(GeoPulseConfigResponse(hasApiKey = false)))
        advanceUntilIdle()
        assertFalse(vm.uiState.value.geoPulseConfigured)
    }

    @Test
    fun `geoPulseConfigured stays false when the config fetch fails`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = vmWithGeoPulse(Result.failure(ApiError.Server(500, "boom")))
        advanceUntilIdle()
        assertFalse(vm.uiState.value.geoPulseConfigured)
    }
}
