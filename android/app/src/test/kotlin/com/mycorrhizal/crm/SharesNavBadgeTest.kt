package com.mycorrhizal.crm

import android.app.Application
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.ui.LocalPendingShareGrantCount
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2293 / ADR 0042 decision 11: the pending-standing-share badge on the Shares entry. */
@OptIn(androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = Application::class, qualifiers = "w360dp-h800dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class SharesNavBadgeTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun drawer(pending: Int) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalPendingShareGrantCount provides pending) {
                MycorrhizalTheme {
                    DrawerContent(currentRoute = "home", onDestinationClick = {})
                }
            }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("drawer-shares").performScrollTo()
    }

    @Test
    fun `a pending count badges the shares entry with a spoken description`() {
        drawer(pending = 3)
        composeTestRule.onNodeWithContentDescription("3 shares waiting for you").assertExists()
    }

    @Test
    fun `a single pending share uses the singular`() {
        drawer(pending = 1)
        composeTestRule.onNodeWithContentDescription("1 share waiting for you").assertExists()
    }

    @Test
    fun `no pending shares means no badge`() {
        drawer(pending = 0)
        composeTestRule.onNodeWithContentDescription("0 shares waiting for you").assertDoesNotExist()
    }

    @Test
    fun `the standing-share routes keep the shares entry selected`() {
        val shares = navDestinations.first { it.route == "shares" }
        assertTrue(isSelected("shares", shares))
        assertTrue(isSelected("shares/with-me", shares))
        assertTrue(isSelected("shares/by-me", shares))
        assertTrue(isSelected("shares/grants/{grantId}?sections={sections}", shares))
        assertFalse(isSelected("share-picker", shares))
    }
}
