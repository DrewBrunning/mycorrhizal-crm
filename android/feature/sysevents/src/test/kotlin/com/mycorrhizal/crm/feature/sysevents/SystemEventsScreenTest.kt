package com.mycorrhizal.crm.feature.sysevents

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ErrorBucket
import com.mycorrhizal.crm.model.network.JobRunHealth
import com.mycorrhizal.crm.model.network.SubsystemHealth
import com.mycorrhizal.crm.model.network.SystemEvent
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SystemEventsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun event(id: Long, type: String, correlation: String = "chain-A") = SystemEvent(
        id = id,
        occurredAt = "2026-08-27T10:00:00Z",
        eventType = type,
        severity = "error",
        component = "contact_sync",
        correlationId = correlation,
    )

    @Test
    fun `rows render the localized event-type label and open on click`() {
        var clicked: Long? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SystemEventList(
                    events = listOf(event(1, "sync_failed"), event(2, "job_completed")),
                    canLoadMore = false,
                    isLoadingMore = false,
                    onLoadMore = {},
                    onRowClick = { clicked = it.id },
                )
            }
        }

        composeTestRule.onNodeWithText("Sync failed").assertIsDisplayed()
        composeTestRule.onNodeWithText("Job completed").assertIsDisplayed()
        composeTestRule.onNodeWithTag("sysevents-row-1").performClick()
        assertEquals(1L, clicked)
    }

    @Test
    fun `load-more button only shows when canLoadMore`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SystemEventList(
                    events = listOf(event(1, "sync_failed")),
                    canLoadMore = true,
                    isLoadingMore = false,
                    onLoadMore = {},
                    onRowClick = {},
                )
            }
        }
        composeTestRule.onNodeWithTag("sysevents-load-more").assertIsDisplayed()
    }

    @Test
    fun `clear-filters button is disabled with no active filters and enabled with one`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SystemEventsFilterToolbar(
                    component = null,
                    severity = null,
                    eventType = null,
                    correlationInput = "",
                    hasFilters = false,
                    onComponentChange = {},
                    onSeverityChange = {},
                    onEventTypeChange = {},
                    onCorrelationChange = {},
                    onClearFilters = {},
                )
            }
        }
        composeTestRule.onNodeWithTag("sysevents-clear-filters").assertIsNotEnabled()
    }

    @Test
    fun `filter toolbar Clear invokes the callback when filters are active`() {
        var cleared = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                SystemEventsFilterToolbar(
                    component = "scheduler",
                    severity = null,
                    eventType = null,
                    correlationInput = "",
                    hasFilters = true,
                    onComponentChange = {},
                    onSeverityChange = {},
                    onEventTypeChange = {},
                    onCorrelationChange = {},
                    onClearFilters = { cleared = true },
                )
            }
        }
        composeTestRule.onNodeWithTag("sysevents-clear-filters").assertIsEnabled().performClick()
        assert(cleared)
    }

    @Test
    fun `subsystem health section renders a card per subsystem and filters on tap`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                SubsystemHealthSection(
                    health = listOf(
                        SubsystemHealth(
                            subsystem = "contact_sync",
                            status = "failing",
                            consecutiveFailures = 9,
                            incidentFirstFailureAt = "2026-08-27T17:19:00Z",
                            lastError = "carddav auth rejected",
                        ),
                        SubsystemHealth(subsystem = "backup", status = "healthy"),
                    ),
                    onSelectSubsystem = { picked = it },
                    onRefresh = {},
                )
            }
        }

        composeTestRule.onNodeWithText("CardDAV sync").assertIsDisplayed()
        composeTestRule.onNodeWithText("Consecutive failures: 9").assertIsDisplayed()
        composeTestRule.onNodeWithText("carddav auth rejected").assertIsDisplayed()
        composeTestRule.onNodeWithTag("subsystem-health-card-contact_sync").performClick()
        assertEquals("contact_sync", picked)
    }

    @Test
    fun `subsystem health section is hidden until data arrives`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                SubsystemHealthSection(health = emptyList(), onSelectSubsystem = {}, onRefresh = {})
            }
        }
        composeTestRule.onNodeWithTag("sysevents-subsystem-health").assertDoesNotExist()
    }

    @Test
    fun `background jobs section renders a card per job with failure detail`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                BackgroundJobsSection(
                    jobs = listOf(
                        JobRunHealth(
                            jobName = "daily_reminders",
                            status = "failing",
                            consecutiveFailures = 4,
                            lastError = "2 notification send(s) failed",
                            lastDurationMs = 40000,
                        ),
                        JobRunHealth(jobName = "calendar_sync", status = "healthy", lastDurationMs = 820),
                    ),
                    onRefresh = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Daily reminders").assertIsDisplayed()
        composeTestRule.onNodeWithText("Calendar sync").assertIsDisplayed()
        composeTestRule.onNodeWithText("Failed 4 times in a row").assertIsDisplayed()
        composeTestRule.onNodeWithText("2 notification send(s) failed").assertIsDisplayed()
        composeTestRule.onNodeWithTag("background-job-card-daily_reminders").assertIsDisplayed()
    }

    @Test
    fun `background jobs section is hidden until data arrives`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                BackgroundJobsSection(jobs = emptyList(), onRefresh = {})
            }
        }
        composeTestRule.onNodeWithTag("sysevents-background-jobs").assertDoesNotExist()
    }

    @Test
    fun `error aggregation section renders a card per cause and views its events on tap`() {
        var viewed: List<Long>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ErrorAggregationSection(
                    buckets = listOf(
                        ErrorBucket(
                            component = "contact_sync",
                            cause = "carddav authentication failed (http <n>)",
                            sampleError = "CardDAV authentication failed (HTTP 401)",
                            count = 17,
                            recurring = true,
                            eventIds = listOf(1, 2, 3),
                        ),
                    ),
                    onViewEvents = { viewed = it },
                    onRefresh = {},
                )
            }
        }

        composeTestRule.onNodeWithText("17").assertIsDisplayed()
        composeTestRule.onNodeWithText("CardDAV authentication failed (HTTP 401)").assertIsDisplayed()
        composeTestRule.onNodeWithText("Recurring").assertIsDisplayed()
        composeTestRule.onNodeWithTag("error-bucket-view-contact_sync").performClick()
        assertEquals(listOf(1L, 2L, 3L), viewed)
    }

    @Test
    fun `error aggregation section is hidden until data arrives`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ErrorAggregationSection(buckets = emptyList(), onViewEvents = {}, onRefresh = {})
            }
        }
        composeTestRule.onNodeWithTag("sysevents-error-aggregation").assertDoesNotExist()
    }
}
