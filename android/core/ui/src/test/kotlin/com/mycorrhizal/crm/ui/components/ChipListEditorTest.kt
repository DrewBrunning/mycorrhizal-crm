package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1628: behaviour of the bare-string chip editor (#832). Ports web's
 * `KeywordsEditor.tsx`: a chip per entry with a delete affordance, plus a
 * field/button to append. The commit guard (`trimmed !in items`) and the
 * index-based removal are the two bits worth pinning.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ChipListEditorTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setEditor(initial: List<String>, onChange: (List<String>) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ChipListEditor(items = initial, onChange = onChange, label = "Keywords")
            }
        }
    }

    @Test
    fun `committing a padded draft trims it and appends once`() {
        val calls = mutableListOf<List<String>>()
        setEditor(emptyList(), onChange = { calls += it })

        composeTestRule.onNodeWithText("Add").performTextInput("  foo  ")
        composeTestRule.onNodeWithContentDescription("Add").performClick()

        // The raw draft is "  foo  "; the committed item is the trimmed "foo".
        assertEquals(listOf(listOf("foo")), calls)
    }

    @Test
    fun `committing a blank draft makes no change call`() {
        val calls = mutableListOf<List<String>>()
        setEditor(listOf("foo"), onChange = { calls += it })

        composeTestRule.onNodeWithText("Add").performTextInput("   ")
        composeTestRule.onNodeWithContentDescription("Add").performClick()

        assertTrue(calls.isEmpty())
    }

    @Test
    fun `committing a duplicate makes no change call`() {
        val calls = mutableListOf<List<String>>()
        setEditor(listOf("foo"), onChange = { calls += it })

        composeTestRule.onNodeWithText("Add").performTextInput("foo")
        composeTestRule.onNodeWithContentDescription("Add").performClick()

        // The `trimmed !in items` guard rejects the duplicate.
        assertTrue(calls.isEmpty())
    }

    @Test
    fun `removing the middle chip drops exactly index one`() {
        val calls = mutableListOf<List<String>>()
        setEditor(listOf("alpha", "beta", "gamma"), onChange = { calls += it })

        composeTestRule.onAllNodesWithContentDescription("Remove")[1].performClick()

        assertEquals(listOf(listOf("alpha", "gamma")), calls)
    }
}
