package com.mycorrhizal.crm.feature.sysevents

import com.mycorrhizal.crm.domain.repository.SystemEventRepository
import com.mycorrhizal.crm.model.network.SystemEvent
import com.mycorrhizal.crm.model.network.SystemEventsResponse
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import java.io.IOException

class SystemEventsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<SystemEventRepository>()

    private fun viewModel() = SystemEventsViewModel(repository)

    private fun event(id: Long = 1, type: String = "sync_completed", correlation: String = "chain-A") =
        SystemEvent(
            id = id,
            occurredAt = "2026-08-27T10:00:00Z",
            eventType = type,
            severity = "info",
            component = "contact_sync",
            correlationId = correlation,
        )

    private fun stubList(response: SystemEventsResponse) {
        coEvery {
            repository.list(
                component = any(),
                severity = any(),
                eventType = any(),
                correlationId = any(),
                limit = any(),
            )
        } returns Result.success(response)
    }

    @Test
    fun `load fetches the default window and surfaces the events`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = listOf(event(1), event(2, type = "sync_failed"))))

            val vm = viewModel()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assertEquals(2, vm.uiState.value.events.size)
            assertNull(vm.uiState.value.error)
            coVerify {
                repository.list(
                    component = null,
                    severity = null,
                    eventType = null,
                    correlationId = null,
                    limit = 100,
                )
            }
        }

    @Test
    fun `applyComponent re-fetches with the component filter and resets the window`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            val vm = viewModel()
            advanceUntilIdle()

            vm.applyComponent("scheduler")
            advanceUntilIdle()

            assertEquals("scheduler", vm.uiState.value.component)
            coVerify {
                repository.list(
                    component = "scheduler",
                    severity = null,
                    eventType = null,
                    correlationId = null,
                    limit = 100,
                )
            }
        }

    @Test
    fun `showRelated queries by correlation id and drops the other filters`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            val vm = viewModel()
            advanceUntilIdle()

            vm.applyComponent("scheduler")
            vm.applySeverity("error")
            advanceUntilIdle()

            vm.showRelated("job:purge_deleted:abc")
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals("job:purge_deleted:abc", state.correlationId)
            assertNull(state.component)
            assertNull(state.severity)
            coVerify {
                repository.list(
                    component = null,
                    severity = null,
                    eventType = null,
                    correlationId = "job:purge_deleted:abc",
                    limit = 500,
                )
            }
        }

    @Test
    fun `correlation id filter is debounced`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            val vm = viewModel()
            advanceUntilIdle()

            vm.onCorrelationIdChange("c")
            vm.onCorrelationIdChange("ch")
            vm.onCorrelationIdChange("chain-Z")
            advanceTimeBy(100)
            // Not yet applied.
            assertEquals("", vm.uiState.value.correlationId)

            advanceUntilIdle()
            assertEquals("chain-Z", vm.uiState.value.correlationId)
            coVerify(exactly = 1) {
                repository.list(
                    component = null,
                    severity = null,
                    eventType = null,
                    correlationId = "chain-Z",
                    limit = 100,
                )
            }
        }

    @Test
    fun `a list failure surfaces an error and keeps isLoading false`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery {
                repository.list(
                    component = any(),
                    severity = any(),
                    eventType = any(),
                    correlationId = any(),
                    limit = any(),
                )
            } returns Result.failure(IOException("boom"))

            val vm = viewModel()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assert(vm.uiState.value.error != null)
        }
}
