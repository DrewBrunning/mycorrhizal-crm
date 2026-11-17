package com.mycorrhizal.crm

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #2649: Organisations, places & pets is a drawer entry (People), not only a Settings row. */
@OptIn(androidx.compose.material3.windowsizeclass.ExperimentalMaterial3WindowSizeClassApi::class)
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], application = android.app.Application::class, qualifiers = "w360dp-h800dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntitiesNavEntryTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun drawer(serverVersion: AppVersion?, onClick: (String) -> Unit = {}) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    DrawerContent(currentRoute = "home", onDestinationClick = onClick)
                }
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `the drawer lists the entities entry under People and opens its route`() {
        var clicked: String? = null
        drawer(AppVersion(1, 7, 0)) { clicked = it }
        val entry = navDestinationsIn(NavGroup.PEOPLE).map { it.route }
        assertTrue("entities" in entry)
        composeTestRule.onNodeWithTag("drawer-entities").performScrollTo().performClick()
        assertEquals("entities", clicked)
    }

    @Test
    fun `a server older than v1_7_0 does not show the entry`() {
        drawer(AppVersion(1, 6, 0))
        composeTestRule.onNodeWithTag("drawer-entities").assertDoesNotExist()
    }

    @Test
    fun `the entity routes select the entry, not Settings`() {
        val entities = navDestinations.first { it.route == "entities" }
        val settings = navDestinations.first { it.route == "settings" }
        for (route in listOf("entities", "entities/e1", "entities/from-contacts")) {
            assertTrue(isSelected(route, entities))
            assertFalse(isSelected(route, settings))
        }
    }

    @Test
    fun `the navigation rail keeps its primary set without the entry`() {
        assertFalse(railDestinations.any { it.route == "entities" })
    }
}
