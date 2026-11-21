package com.mycorrhizal.crm.testing.a11y

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onRoot
import androidx.compose.ui.unit.dp
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Regression tests for [assertAccessibleSemantics] itself. A sweep that silently
 * stops asserting is worse than no sweep, so each invariant gets a minimal
 * failing tree and a minimal correct one.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AccessibilitySemanticsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun assertViolation(): AssertionError = assertThrows(AssertionError::class.java) {
        assertAccessibleSemantics(composeTestRule.onRoot().fetchSemanticsNode())
    }

    @Test
    fun `a labelled button passes`() {
        composeTestRule.setContent {
            Box(Modifier.size(48.dp).clickable {}) { Text("Save") }
        }

        assertAccessibleSemantics(composeTestRule.onRoot().fetchSemanticsNode())
    }

    @Test
    fun `a toggleable checkbox passes`() {
        composeTestRule.setContent {
            var checked by remember { mutableStateOf(true) }
            Box(
                Modifier
                    .size(48.dp)
                    .toggleable(value = checked, role = Role.Checkbox) { checked = it }
            ) {
                Text("Include")
            }
        }

        assertAccessibleSemantics(composeTestRule.onRoot().fetchSemanticsNode())
    }

    @Test
    fun `a checkbox role without a state is a violation`() {
        composeTestRule.setContent {
            Box(Modifier.size(48.dp).clickable(role = Role.Checkbox) {}) { Text("Include") }
        }

        val error = assertViolation()
        assertTrue(
            "expected a state-announcement violation, got: ${error.message}",
            error.message.orEmpty().contains("state", ignoreCase = true)
        )
    }

    @Test
    fun `a radio role without a state is a violation`() {
        composeTestRule.setContent {
            Box(Modifier.size(48.dp).clickable(role = Role.RadioButton) {}) { Text("Choose") }
        }

        val error = assertViolation()
        assertTrue(
            "expected a state-announcement violation, got: ${error.message}",
            error.message.orEmpty().contains("state", ignoreCase = true)
        )
    }

    @Test
    fun `a zero-size interactive node is a violation`() {
        composeTestRule.setContent {
            Box(Modifier.size(0.dp).clickable {}) { Text("Invisible") }
        }

        val error = assertViolation()
        assertTrue(
            "expected a zero-size violation, got: ${error.message}",
            error.message.orEmpty().contains("box", ignoreCase = true)
        )
    }
}
