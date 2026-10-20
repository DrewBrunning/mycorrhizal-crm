package com.mycorrhizal.crm.feature.timeline

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import java.util.Locale
import java.util.TimeZone

/**
 * Issue #1752: the activity and note list rows render their date through
 * DateFormat with the user's preference, not as a raw ISO slice. The device
 * zone is pinned west of UTC so a calendar day stored at 00:00Z (the way a
 * picked activity/note date is saved) is proven to keep its UTC date (#1778).
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ListDatePreferenceTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val originalZone = TimeZone.getDefault()
    private val originalLocale = Locale.getDefault()

    @Before
    fun pin() {
        TimeZone.setDefault(TimeZone.getTimeZone("America/Chicago"))
        Locale.setDefault(Locale.ENGLISH)
    }

    @After
    fun restore() {
        TimeZone.setDefault(originalZone)
        Locale.setDefault(originalLocale)
    }

    private fun activityRow(format: String?) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                val row = @androidx.compose.runtime.Composable {
                    ActivityListItem(
                        activity = Activity(id = 1, title = "Coffee", date = "2026-05-19T00:00:00Z"),
                        onClick = {},
                        onContactClick = {},
                        onDelete = {},
                    )
                }
                if (format == null) row() else CompositionLocalProvider(LocalDateFormat provides format) { row() }
            }
        }
    }

    private fun noteRow(format: String) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides format) {
                    NoteListItem(
                        note = Note(id = 1, content = "Met at the fair", date = "2026-05-19T00:00:00Z"),
                        onClick = {},
                        onDelete = {},
                    )
                }
            }
        }
    }

    @Test
    fun `activity row shows the european date by default, keeping its UTC calendar day`() {
        activityRow(null)
        composeTestRule.onNodeWithText("19.05.2026").assertIsDisplayed()
    }

    @Test
    fun `activity row follows the iso and long-month preferences`() {
        activityRow("iso")
        composeTestRule.onNodeWithText("2026-05-19").assertIsDisplayed()
    }

    @Test
    fun `activity row follows a month-name preference`() {
        activityRow("us-mmmm")
        composeTestRule.onNodeWithText("May 19, 2026").assertIsDisplayed()
    }

    @Test
    fun `note row follows the preference`() {
        noteRow("us")
        composeTestRule.onNodeWithText("05/19/2026").assertIsDisplayed()
    }
}
