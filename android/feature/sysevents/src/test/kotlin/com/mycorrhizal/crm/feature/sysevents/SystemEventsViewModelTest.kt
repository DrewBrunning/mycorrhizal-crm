package com.mycorrhizal.crm.feature.sysevents

import com.mycorrhizal.crm.domain.repository.SystemEventRepository
import com.mycorrhizal.crm.model.network.ErrorAggregationResponse
import com.mycorrhizal.crm.model.network.ErrorBucket
import com.mycorrhizal.crm.model.network.JobRunHealth
import com.mycorrhizal.crm.model.network.JobRunHealthResponse
import com.mycorrhizal.crm.model.network.SubsystemHealth
import com.mycorrhizal.crm.model.network.SubsystemHealthResponse
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
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import java.io.IOException

class SystemEventsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<SystemEventRepository>()

    @Before
    fun stubPanelsByDefault() {
        coEvery { repository.subsystemHealth() } returns Result.success(SubsystemHealthResponse())
        coEvery { repository.errorAggregation(any()) } returns Result.success(ErrorAggregationResponse())
        coEvery { repository.jobRunHealth() } returns Result.success(JobRunHealthResponse())
    }

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
                ids = any(),
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
    fun `subsystem health is fetched on open and lands in state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            coEvery { repository.subsystemHealth() } returns Result.success(
                SubsystemHealthResponse(
                    subsystems = listOf(
                        SubsystemHealth(subsystem = "contact_sync", status = "failing", consecutiveFailures = 9),
                        SubsystemHealth(subsystem = "backup", status = "healthy"),
                    ),
                ),
            )

            val vm = viewModel()
            advanceUntilIdle()

            assertEquals(2, vm.uiState.value.subsystemHealth.size)
            assertEquals("contact_sync", vm.uiState.value.subsystemHealth.first().subsystem)
            assertEquals(9, vm.uiState.value.subsystemHealth.first().consecutiveFailures)
        }

    @Test
    fun `refreshSubsystemHealth re-fetches the panel without touching the list`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            val vm = viewModel()
            advanceUntilIdle()

            coEvery { repository.subsystemHealth() } returns Result.success(
                SubsystemHealthResponse(
                    subsystems = listOf(SubsystemHealth(subsystem = "webhook", status = "unknown")),
                ),
            )
            vm.refreshSubsystemHealth()
            advanceUntilIdle()

            assertEquals(listOf("webhook"), vm.uiState.value.subsystemHealth.map { it.subsystem })
            coVerify(atLeast = 2) { repository.subsystemHealth() }
        }

    @Test
    fun `a subsystem-health failure is swallowed and leaves the list unaffected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = listOf(event(1))))
            coEvery { repository.subsystemHealth() } returns Result.failure(IOException("nope"))

            val vm = viewModel()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.subsystemHealth.isEmpty())
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.events.size)
        }

    @Test
    fun `job-run health is fetched on open and lands in state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            coEvery { repository.jobRunHealth() } returns Result.success(
                JobRunHealthResponse(
                    jobs = listOf(
                        JobRunHealth(jobName = "daily_reminders", status = "failing", consecutiveFailures = 4),
                        JobRunHealth(jobName = "calendar_sync", status = "healthy"),
                    ),
                ),
            )

            val vm = viewModel()
            advanceUntilIdle()

            assertEquals(2, vm.uiState.value.jobRunHealth.size)
            assertEquals("daily_reminders", vm.uiState.value.jobRunHealth.first().jobName)
            assertEquals(4, vm.uiState.value.jobRunHealth.first().consecutiveFailures)
        }

    @Test
    fun `a job-run-health failure is swallowed and leaves the list unaffected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = listOf(event(1))))
            coEvery { repository.jobRunHealth() } returns Result.failure(IOException("nope"))

            val vm = viewModel()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.jobRunHealth.isEmpty())
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.events.size)
        }

    @Test
    fun `error aggregation is fetched on open and lands in state`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            coEvery { repository.errorAggregation(any()) } returns Result.success(
                ErrorAggregationResponse(
                    totalEvents = 17,
                    buckets = listOf(
                        ErrorBucket(component = "contact_sync", count = 17, recurring = true, eventIds = listOf(1, 2, 3)),
                    ),
                ),
            )

            val vm = viewModel()
            advanceUntilIdle()

            assertEquals(1, vm.uiState.value.errorBuckets.size)
            assertEquals(17, vm.uiState.value.errorBuckets.first().count)
            assertTrue(vm.uiState.value.errorBuckets.first().recurring)
        }

    @Test
    fun `a error-aggregation failure is swallowed and leaves the list unaffected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = listOf(event(1))))
            coEvery { repository.errorAggregation(any()) } returns Result.failure(IOException("nope"))

            val vm = viewModel()
            advanceUntilIdle()

            assertTrue(vm.uiState.value.errorBuckets.isEmpty())
            assertNull(vm.uiState.value.error)
            assertEquals(1, vm.uiState.value.events.size)
        }

    @Test
    fun `applyEventIds queries the timeline by those ids alone and widens the window`() =
        runTest(mainDispatcherRule.testDispatcher) {
            stubList(SystemEventsResponse(systemEvents = emptyList()))
            val vm = viewModel()
            advanceUntilIdle()

            vm.applyComponent("scheduler")
            advanceUntilIdle()

            vm.applyEventIds(listOf(11L, 12L, 13L))
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(listOf(11L, 12L, 13L), state.eventIds)
            assertNull(state.component)
            assertTrue(state.hasActiveFilters)
            coVerify {
                repository.list(
                    component = null,
                    severity = null,
                    eventType = null,
                    correlationId = null,
                    ids = listOf(11L, 12L, 13L),
                    limit = 500,
                )
            }

            // A component filter change clears the id drill-down.
            vm.applyComponent("notification")
            advanceUntilIdle()
            assertTrue(vm.uiState.value.eventIds.isEmpty())
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
                    ids = any(),
                    limit = any(),
                )
            } returns Result.failure(IOException("boom"))

            val vm = viewModel()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.isLoading)
            assert(vm.uiState.value.error != null)
        }
}
