package com.mycorrhizal.crm.ui.components

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.util.DateFieldMode
import com.mycorrhizal.crm.ui.LocalDateFormat
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import java.time.OffsetDateTime
import java.util.TimeZone
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1726: the shared date field renders, opens the picker and clears. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1200dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class DateFieldTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `shows the stored day in the user's format and confirms it unchanged`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CompositionLocalProvider(LocalDateFormat provides "us") {
                    DateField(
                        value = "2026-08-10",
                        onValueChange = { picked = it },
                        label = "Date",
                        mode = DateFieldMode.DATE_ONLY,
                        testTag = "date"
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("08/10/2026").assertIsDisplayed()
        composeTestRule.onNodeWithTag("date").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals("2026-08-10", picked)
    }

    @Test
    fun `a yearless partial opens with year unknown checked and round-trips`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                DateField(
                    value = "--02-29",
                    onValueChange = { picked = it },
                    label = "Birthday",
                    mode = DateFieldMode.PARTIAL,
                    testTag = "birthday"
                )
            }
        }

        composeTestRule.onNodeWithText("29.02.").assertIsDisplayed()
        composeTestRule.onNodeWithTag("birthday").performClick()
        composeTestRule.onNodeWithTag("date-field-year-unknown").assertIsOn()
        composeTestRule.onNodeWithText("Confirm").performClick()

        // 29 Feb stays selectable because the picker opens on the leap-year
        // placeholder, and the year is dropped on the way out.
        assertEquals("--02-29", picked)
    }

    @Test
    fun `allowClear exposes a clear action that empties the value`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                DateField(
                    value = "2026-08-10",
                    onValueChange = { picked = it },
                    label = "From",
                    mode = DateFieldMode.DATE_ONLY,
                    allowClear = true
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Clear date").performClick()
        assertEquals("", picked)
    }

    @Test
    fun `an explicit onClear callback is used instead of writing an empty value`() {
        var cleared = false
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                DateField(
                    value = "2026-08-10",
                    onValueChange = { picked = it },
                    label = "From",
                    mode = DateFieldMode.DATE_ONLY,
                    allowClear = true,
                    onClear = { cleared = true }
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("Clear date").performClick()

        assertEquals(true, cleared)
        assertNull(picked)
    }

    @Test
    fun `a date-time field adds a time in the same flow and emits an instant`() {
        var picked: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                DateField(
                    value = "2026-08-10",
                    onValueChange = { picked = it },
                    label = "Date",
                    mode = DateFieldMode.DATE_TIME,
                    testTag = "date-time"
                )
            }
        }

        composeTestRule.onNodeWithTag("date-time").performClick()
        composeTestRule.onNodeWithText("Add time").performClick()
        composeTestRule.onNodeWithText("Time").assertIsDisplayed()
        composeTestRule.onNodeWithTag("time-picker-confirm").performClick()
        composeTestRule.onNodeWithTag("date-picker-confirm").performClick()

        val instant = OffsetDateTime.parse(picked)
        assertEquals(2026, instant.year)
        assertEquals(8, instant.monthValue)
        assertEquals(10, instant.dayOfMonth)
        // The picker defaults to 12:00; the emitted value is a full RFC 3339
        // instant (seconds included) with the device's offset.
        assertEquals(12, instant.hour)
        assertEquals(0, instant.minute)
        assertEquals(0, instant.second)
    }

    @Test
    fun `removing an existing time keeps the date-only shape`() {
        var picked: String? = null
        // The stored instant is shown in the device zone. Pin a zone where that
        // day differs from the UTC day (12:00Z is 02:00 on the 11th at UTC+14),
        // so the expected value is a literal: deriving it with the same
        // ZoneId.systemDefault() conversion the code uses could never fail.
        val savedZone = TimeZone.getDefault()
        TimeZone.setDefault(TimeZone.getTimeZone("Pacific/Kiritimati"))
        try {
            removeTimeFromStoredInstant { picked = it }
        } finally {
            TimeZone.setDefault(savedZone)
        }
        assertEquals("2026-08-11T00:00:00Z", picked)
    }

    private fun removeTimeFromStoredInstant(onPicked: (String?) -> Unit) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                DateField(
                    value = "2026-08-10T12:00:00Z",
                    onValueChange = onPicked,
                    label = "Date",
                    mode = DateFieldMode.DATE_TIME,
                    testTag = "date-time"
                )
            }
        }

        composeTestRule.onNodeWithTag("date-time").performClick()
        composeTestRule.onNodeWithTag("date-picker-time").performClick()
        composeTestRule.onNodeWithTag("time-picker-remove").performClick()
        composeTestRule.onNodeWithTag("date-picker-confirm").performClick()
    }
}
