package com.mycorrhizal.crm.ui.components

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsFocused
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1922: screen entry must move focus to the title, not leave it on the
 * top bar's Back/Menu button, so TalkBack announces which screen (or contact)
 * opened. The title also has to be a heading for TalkBack's heading navigation.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ScreenTitleTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `the screen title takes focus on entry`() {
        composeTestRule.setContent { MycorrhizalTheme { ScreenTitle("Contact detail") } }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Contact detail").assertIsFocused()
    }

    @Test
    fun `the screen title carries heading semantics`() {
        composeTestRule.setContent { MycorrhizalTheme { ScreenTitle("Contact detail") } }
        composeTestRule.waitForIdle()

        composeTestRule
            .onNodeWithText("Contact detail")
            .assertIsDisplayed()
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }
}
