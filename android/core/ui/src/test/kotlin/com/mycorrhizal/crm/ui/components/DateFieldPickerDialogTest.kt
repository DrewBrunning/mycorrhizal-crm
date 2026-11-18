package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.time.LocalDate
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #2654: the date picker dialog offers typed entry (in the user's date format), shows its
 * header in that format, and starts the week where the device or the format says.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1200dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DateFieldPickerDialogTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private var picked: String? = null

    private fun open(
        dateFormat: String,
        value: String? = "2026-08-10",
        selectableFrom: LocalDate? = null
    ) {
        picked = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides dateFormat) {
                    DateField(
                        value = value,
                        onValueChange = { picked = it },
                        label = "Date",
                        mode = DateFieldMode.DATE_ONLY,
                        selectableFrom = selectableFrom,
                        testTag = "date"
                    )
                }
            }
        }
        composeTestRule.onNodeWithTag("date-picker-open").performClick()
    }

    @Test
    fun `the dialog offers a visible toggle to typed entry`() {
        open("eu")

        composeTestRule.onNodeWithContentDescription("Type a date").assertIsDisplayed()
    }

    @Test
    fun `the dialog header shows the selected day in the user's format`() {
        open("eu")

        // The field behind the dialog and the dialog header both show 10.08.2026.
        composeTestRule.onAllNodesWithText("10.08.2026").assertCountEquals(2)
        composeTestRule.onAllNodesWithText("Aug 10, 2026").assertCountEquals(0)
    }

    @Test
    fun `typed entry starts in the user's format and a typed day is confirmed`() {
        open("eu")

        composeTestRule.onNodeWithContentDescription("Type a date").performClick()
        composeTestRule.onNodeWithTag("date-picker-typed-input").assert(
            SemanticsMatcher("typed text is 10.08.2026") {
                it.config.getOrNull(SemanticsProperties.EditableText)?.text == "10.08.2026"
            }
        )
        composeTestRule.onNodeWithTag(
            "date-picker-typed-input"
        ).performTextReplacement("15.09.2026")
        composeTestRule.onNodeWithTag("date-picker-confirm").performClick()

        assertEquals("2026-09-15", picked)
    }

    @Test
    fun `typed entry follows a month-first format`() {
        open("us")

        composeTestRule.onNodeWithContentDescription("Type a date").performClick()
        composeTestRule.onNodeWithTag(
            "date-picker-typed-input"
        ).performTextReplacement("09/15/2026")
        composeTestRule.onNodeWithTag("date-picker-confirm").performClick()

        assertEquals("2026-09-15", picked)
    }

    @Test
    fun `an impossible typed day shows the format hint and blocks confirm`() {
        open("eu")

        composeTestRule.onNodeWithContentDescription("Type a date").performClick()
        composeTestRule.onNodeWithTag(
            "date-picker-typed-input"
        ).performTextReplacement("31.02.2026")

        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("date-picker-confirm").assertIsNotEnabled()
        assertNull(picked)
    }

    @Test
    fun `a typed day before the field's floor is out of range and blocks confirm`() {
        open("eu", selectableFrom = LocalDate.of(2026, 8, 20))

        composeTestRule.onNodeWithContentDescription("Type a date").performClick()
        composeTestRule.onNodeWithTag(
            "date-picker-typed-input"
        ).performTextReplacement("15.08.2026")

        composeTestRule.onNodeWithText(
            "That date is outside the allowed range."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("date-picker-confirm").assertIsNotEnabled()
    }

    @Test
    fun `switching back to the calendar keeps a typed day selected`() {
        open("eu")

        composeTestRule.onNodeWithContentDescription("Type a date").performClick()
        composeTestRule.onNodeWithTag(
            "date-picker-typed-input"
        ).performTextReplacement("15.09.2026")
        composeTestRule.onNodeWithContentDescription("Pick from the calendar").performClick()

        // The calendar's header now shows the typed day (the field behind keeps the stored one).
        composeTestRule.onNodeWithText("15.09.2026").assertIsDisplayed()
        composeTestRule.onNodeWithTag("date-picker-confirm").assertIsEnabled().performClick()

        assertEquals("2026-09-15", picked)
    }

    @Test
    fun `the week starts on Monday for a day-first format and Sunday for a month-first one`() {
        open("eu")
        assertEquals("Monday", firstWeekdayHeader())
    }

    @Test
    fun `a month-first format starts the week on Sunday`() {
        open("us")
        assertEquals("Sunday", firstWeekdayHeader())
    }

    private fun firstWeekdayHeader(): String {
        val names = setOf(
            "Monday",
            "Tuesday",
            "Wednesday",
            "Thursday",
            "Friday",
            "Saturday",
            "Sunday"
        )
        val headers = composeTestRule.onAllNodes(
            SemanticsMatcher("weekday header") {
                it.config.getOrNull(SemanticsProperties.ContentDescription)
                    ?.firstOrNull() in names
            },
            useUnmergedTree = true
        ).fetchSemanticsNodes()
        return headers.minBy { it.boundsInRoot.left }
            .config.getOrNull(SemanticsProperties.ContentDescription)!!.first()
    }
}
