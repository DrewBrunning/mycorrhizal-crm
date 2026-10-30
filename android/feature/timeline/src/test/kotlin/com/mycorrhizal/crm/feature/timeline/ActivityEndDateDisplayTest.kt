package com.mycorrhizal.crm.feature.timeline

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2006: a multi-day activity shows its range in the list and the contact timeline. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1200dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ActivityEndDateDisplayTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val trip = Activity(
        id = 1,
        title = "Lisbon trip",
        type = "visit",
        date = "2026-08-10T00:00:00Z",
        endDate = "2026-08-14T00:00:00Z"
    )

    @Test
    fun `the activity list row shows a numeric range in full`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "iso") {
                    ActivityListItem(
                        activity = trip,
                        onClick = {},
                        onContactClick = {},
                        onDelete = {}
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("2026-08-10 – 2026-08-14", substring = true)
            .assertIsDisplayed()
    }

    @Test
    fun `the activity list row collapses the month in a month-name format`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "us-mmm") {
                    ActivityListItem(
                        activity = trip,
                        onClick = {},
                        onContactClick = {},
                        onDelete = {}
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("Aug 10–14, 2026", substring = true).assertIsDisplayed()
    }

    @Test
    fun `a single-day activity list row shows one day`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "iso") {
                    ActivityListItem(
                        activity = trip.copy(endDate = null),
                        onClick = {},
                        onContactClick = {},
                        onDelete = {}
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("2026-08-10", substring = true).assertIsDisplayed()
        composeTestRule.onNodeWithText("2026-08-14", substring = true).assertDoesNotExist()
    }

    @Test
    fun `the contact timeline row shows the range for a multi-day activity`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "iso") {
                    TimelineSection(
                        items = listOf(TimelineItem.ActivityItem(trip)),
                        onEditActivity = {},
                        onEditNote = {},
                        onEditReminder = {},
                        onCompleteReminder = {}
                    )
                }
            }
        }
        composeTestRule.onNodeWithText("2026-08-10 – 2026-08-14", substring = true)
            .assertIsDisplayed()
    }

    @Test
    fun `the contact timeline row stays as before for a single-day activity`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TimelineSection(
                    items = listOf(TimelineItem.ActivityItem(trip.copy(endDate = null))),
                    onEditActivity = {},
                    onEditNote = {},
                    onEditReminder = {},
                    onCompleteReminder = {}
                )
            }
        }
        composeTestRule.onNodeWithText("–", substring = true).assertDoesNotExist()
    }
}
