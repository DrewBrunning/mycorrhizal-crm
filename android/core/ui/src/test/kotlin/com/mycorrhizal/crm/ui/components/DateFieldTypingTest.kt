package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performImeAction
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.time.LocalDate
import java.util.TimeZone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2207: typing a date into the shared Android date field. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1200dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DateFieldTypingTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val emitted = mutableListOf<String>()
    private var cleared = 0

    private fun setField(
        initial: String,
        mode: DateFieldMode = DateFieldMode.DATE_ONLY,
        format: String = "eu",
        selectableFrom: LocalDate? = null,
        useOnClear: Boolean = false
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides format) {
                    var current by remember { mutableStateOf(initial) }
                    DateField(
                        value = current,
                        onValueChange = {
                            current = it
                            emitted += it
                        },
                        label = "Date",
                        mode = mode,
                        selectableFrom = selectableFrom,
                        onClear = if (useOnClear) {
                            {
                                current = ""
                                cleared++
                            }
                        } else {
                            null
                        },
                        testTag = "date"
                    )
                }
            }
        }
    }

    private fun type(text: String) {
        composeTestRule.onNodeWithTag("date").performTextReplacement(text)
    }

    private fun done() {
        composeTestRule.onNodeWithTag("date").performImeAction()
    }

    private fun assertTypedEmits(format: String, text: String, expected: String) {
        setField(initial = "", format = format)
        type(text)
        assertEquals(listOf(expected), emitted)
    }

    @Test
    fun `typing a valid eu date reports it immediately`() =
        assertTypedEmits("eu", "10.08.2026", "2026-08-10")

    @Test
    fun `typing a valid us date reports it`() = assertTypedEmits("us", "08/10/2026", "2026-08-10")

    @Test
    fun `typing a valid iso date reports it`() = assertTypedEmits("iso", "2026-08-10", "2026-08-10")

    @Test
    fun `typing a valid ca date reports it`() = assertTypedEmits("ca", "10/08/2026", "2026-08-10")

    @Test
    fun `typing a valid eu-hyphen date reports it`() =
        assertTypedEmits("eu-hyphen", "10-08-2026", "2026-08-10")

    @Test
    fun `a month-name format is typed numerically`() =
        assertTypedEmits("us-mmmm", "08/10/2026", "2026-08-10")

    @Test
    fun `a pasted ISO date is accepted whatever the format`() =
        assertTypedEmits("us", "2026-08-10", "2026-08-10")

    @Test
    fun `a far-past date can be typed`() = assertTypedEmits("eu", "07.03.1950", "1950-03-07")

    @Test
    fun `a half-typed date reports nothing`() {
        setField(initial = "2026-08-10")
        type("10.08.20")
        assertTrue(emitted.isEmpty())
    }

    @Test
    fun `an invalid date shows an error, keeps its text and leaves the value alone`() {
        setField(initial = "2026-08-10")
        type("31.02.2026")
        done()

        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("date").assertTextContains("31.02.2026")
        assertTrue(emitted.isEmpty())
    }

    @Test
    fun `the error names the user's format`() {
        setField(initial = "", format = "us")
        type("13/45/2026")
        done()
        composeTestRule.onNodeWithText("Enter a valid date as MM/DD/YYYY.").assertIsDisplayed()
    }

    @Test
    fun `the error clears when the text is corrected`() {
        setField(initial = "2026-08-10")
        type("31.02.2026")
        done()
        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertIsDisplayed()

        type("28.02.2026")
        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertDoesNotExist()
        assertEquals(listOf("2026-02-28"), emitted)
    }

    @Test
    fun `a date before the lower bound shows the range error`() {
        val floor = LocalDate.of(2026, 8, 10)
        setField(initial = "2026-08-10", selectableFrom = floor)
        type("09.08.2026")
        done()

        composeTestRule.onNodeWithText(
            "That date is outside the allowed range."
        ).assertIsDisplayed()
        assertTrue(emitted.isEmpty())

        type("11.08.2026")
        assertEquals(listOf("2026-08-11"), emitted)
    }

    @Test
    fun `a valid entry is normalised to the display format on Done`() {
        setField(initial = "2026-08-10", format = "us-mmm")
        type("08/11/2026")
        done()
        composeTestRule.onNodeWithTag("date").assertTextContains("Aug 11, 2026")
    }

    @Test
    fun `focusing shows the numeric form of a month-name format`() {
        setField(initial = "2026-08-10", format = "eu-mmmm")
        composeTestRule.onNodeWithTag("date").assertTextContains("10 August, 2026")
        composeTestRule.onNodeWithTag("date").performClick()
        composeTestRule.onNodeWithTag("date").assertTextContains("10.08.2026")
    }

    @Test
    fun `emptying the field clears the value`() {
        setField(initial = "2026-08-10")
        type("")
        assertEquals(listOf(""), emitted)
    }

    @Test
    fun `emptying the field uses onClear when given`() {
        setField(initial = "2026-08-10", useOnClear = true)
        type("")
        assertEquals(1, cleared)
        assertTrue(emitted.isEmpty())
    }

    @Test
    fun `tapping the text does not open the picker but the calendar icon does`() {
        setField(initial = "2026-08-10")
        composeTestRule.onNodeWithTag("date").performClick()
        composeTestRule.onNodeWithTag("date-picker-confirm").assertDoesNotExist()

        composeTestRule.onNodeWithTag("date-picker-open").performClick()
        composeTestRule.onNodeWithTag("date-picker-confirm").assertIsDisplayed()
    }

    @Test
    fun `a picker choice replaces half-typed text and its error`() {
        setField(initial = "2026-08-10")
        type("10.0")
        done()
        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertIsDisplayed()

        composeTestRule.onNodeWithTag("date-picker-open").performClick()
        composeTestRule.onNodeWithTag("date-picker-confirm").performClick()

        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertDoesNotExist()
        composeTestRule.onNodeWithTag("date").assertTextContains("10.08.2026")
        assertEquals(listOf("2026-08-10"), emitted)
    }

    @Test
    fun `a reminder-style field emits UTC midnight for a typed day`() {
        setField(initial = "", mode = DateFieldMode.DATE_UTC_MIDNIGHT)
        type("10.08.2026")
        assertEquals(listOf("2026-08-10T00:00:00Z"), emitted)
    }

    @Test
    fun `a date-time field keeps its time when the date is typed`() {
        val saved = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone("UTC"))
        try {
            setField(initial = "2026-08-10T12:30:00Z", mode = DateFieldMode.DATE_TIME)
            type("11.08.2026")
            assertEquals(listOf("2026-08-11T12:30:00Z"), emitted)
        } finally {
            TimeZone.setDefault(saved)
        }
    }

    @Test
    fun `a date-time field without a time emits the date-only shape`() {
        setField(initial = "", mode = DateFieldMode.DATE_TIME_OPTIONAL)
        type("11.08.2026")
        assertEquals(listOf("2026-08-11"), emitted)
    }

    @Test
    fun `partial mode accepts a full date`() {
        setField(initial = "", mode = DateFieldMode.PARTIAL)
        type("15.06.1984")
        assertEquals(listOf("1984-06-15"), emitted)
    }

    @Test
    fun `partial mode treats day and month only as year unknown`() {
        setField(initial = "", mode = DateFieldMode.PARTIAL)
        type("15.06.")
        assertEquals(listOf("--06-15"), emitted)
    }

    @Test
    fun `partial mode year-less entry follows the format's order`() {
        setField(initial = "", mode = DateFieldMode.PARTIAL, format = "us")
        type("06/15")
        assertEquals(listOf("--06-15"), emitted)
    }

    @Test
    fun `a stored year-less birthday is edited as day and month`() {
        setField(initial = "--02-29", mode = DateFieldMode.PARTIAL)
        composeTestRule.onNodeWithTag("date").performClick()
        composeTestRule.onNodeWithTag("date").assertTextContains("29.02.")
        type("30.04.")
        assertEquals(listOf("--04-30"), emitted)
    }

    @Test
    fun `partial mode rejects an impossible year-less day`() {
        setField(initial = "", mode = DateFieldMode.PARTIAL)
        type("30.02.")
        done()
        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY or DD.MM..")
            .assertIsDisplayed()
        assertTrue(emitted.isEmpty())
    }

    @Test
    fun `date mode rejects a year-less entry`() {
        setField(initial = "", mode = DateFieldMode.DATE_ONLY)
        type("15.06.")
        done()
        composeTestRule.onNodeWithText("Enter a valid date as DD.MM.YYYY.").assertIsDisplayed()
        assertTrue(emitted.isEmpty())
    }
}
