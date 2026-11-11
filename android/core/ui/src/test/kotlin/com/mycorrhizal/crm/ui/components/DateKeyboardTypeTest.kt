package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.text.input.KeyboardType
import com.mycorrhizal.crm.model.util.DateFormat
import org.junit.Assert.assertEquals
import org.junit.Test

/** Issue #2378: dot/hyphen date formats get the number keyboard; `/` formats keep text. */
class DateKeyboardTypeTest {

    private val expected = mapOf(
        DateFormat.EU to KeyboardType.Number,
        DateFormat.EU_MMM to KeyboardType.Number,
        DateFormat.EU_MMMM to KeyboardType.Number,
        DateFormat.EU_HYPHEN to KeyboardType.Number,
        DateFormat.ISO to KeyboardType.Number,
        DateFormat.US to KeyboardType.Text,
        DateFormat.US_MMM to KeyboardType.Text,
        DateFormat.US_MMMM to KeyboardType.Text,
        DateFormat.CA to KeyboardType.Text
    )

    @Test
    fun `the table covers every format id the app can supply`() {
        assertEquals(DateFormat.ALL_FORMATS.toSet(), expected.keys)
    }

    @Test
    fun `every supported format picks the keyboard its separator needs`() {
        DateFormat.ALL_FORMATS.forEach { format ->
            assertEquals(format, expected.getValue(format), dateKeyboardType(format))
        }
    }

    @Test
    fun `an unknown format id follows the eu fallback and gets the number keyboard`() {
        assertEquals(KeyboardType.Number, dateKeyboardType("something-new"))
    }
}
