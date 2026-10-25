package com.mycorrhizal.crm.ui.components

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.semantics.getOrNull
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
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
 * Issue #1760: the one empty state every list screen uses. Pins the title /
 * body / action layout, that the action is a real focusable button, and that
 * the text is announced (polite live region) when it replaces the loading
 * skeleton.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EmptyStateTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `title only renders no body and no button`() {
        composeTestRule.setContent {
            MycorrhizalTheme { EmptyState(title = "No notes yet") }
        }
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
        composeTestRule.onAllNodes(hasClickAction()).assertCountEquals(0)
    }

    @Test
    fun `title body and action all render and the action fires once`() {
        var clicks = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                EmptyState(
                    title = "No notes yet",
                    body = "Notes capture what you want to remember.",
                    icon = Icons.Outlined.Add,
                    actionLabel = "New note",
                    onAction = { clicks += 1 }
                )
            }
        }
        composeTestRule.onNodeWithText("No notes yet").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Notes capture what you want to remember."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("New note").assertIsDisplayed().performClick()
        assertEquals(1, clicks)
    }

    @Test
    fun `an action label without a handler renders no button`() {
        composeTestRule.setContent {
            MycorrhizalTheme { EmptyState(title = "No notes yet", actionLabel = "New note") }
        }
        composeTestRule.onAllNodes(hasClickAction()).assertCountEquals(0)
    }

    @Test
    fun `title and body are one polite live region and the button is a separate focusable control`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EmptyState(
                    title = "No notes yet",
                    body = "Body text",
                    actionLabel = "New note",
                    onAction = {}
                )
            }
        }
        val region = SemanticsMatcher("polite live region") {
            it.config.getOrNull(SemanticsProperties.LiveRegion) == LiveRegionMode.Polite
        }
        val node = composeTestRule.onNode(region).fetchSemanticsNode()
        val spoken = node.config.getOrNull(
            SemanticsProperties.Text
        ).orEmpty().joinToString(" ") { it.text }
        assertTrue("live region speaks the title: $spoken", "No notes yet" in spoken)
        assertTrue("live region speaks the body: $spoken", "Body text" in spoken)
        // The action is outside the live region, so it keeps its own focus stop.
        composeTestRule.onAllNodes(hasClickAction() and !region).assertCountEquals(1)
    }
}
