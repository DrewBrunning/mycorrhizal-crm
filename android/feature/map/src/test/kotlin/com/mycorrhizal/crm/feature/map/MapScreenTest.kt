package com.mycorrhizal.crm.feature.map

import androidx.compose.material3.Text
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.LatLng
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MapScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val ada = MapPoint(1, "Ada Lovelace", "a1", "10 Downing St, London", LatLng(51.5, -0.12))
    private val grace = MapPoint(2, "Grace Hopper", "b1", "", LatLng(38.9, -77.1))

    private fun content(
        state: MapUiState,
        opened: MutableList<Int> = mutableListOf(),
        selected: MutableList<String?> = mutableListOf(),
        listToggles: MutableList<Boolean> = mutableListOf(),
        onMenuClick: (() -> Unit)? = {},
        onRetry: () -> Unit = {},
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                MapScreenContent(
                    uiState = state,
                    onMenuClick = onMenuClick,
                    onOpenContact = { opened += it },
                    onSelect = { selected += it },
                    onShowListChange = { listToggles += it },
                    onRetry = onRetry,
                    // The native renderer can't run under Robolectric; the stub
                    // exposes what the real canvas is handed.
                    mapCanvas = { styleUrl, points, selectedKey, modifier: Modifier ->
                        androidx.compose.foundation.layout.Box(modifier) {
                            Text("canvas:$styleUrl:${points.size}:${selectedKey.orEmpty()}")
                        }
                    },
                )
            }
        }
    }

    private fun loaded(
        points: List<MapPoint> = listOf(ada, grace),
        selectedKey: String? = null,
        showList: Boolean = false,
        truncated: Boolean = false,
    ) = MapUiState(
        isLoading = false,
        styleUrl = "https://tiles.example/s",
        points = points,
        selectedKey = selectedKey,
        showList = showList,
        truncated = truncated,
    )

    @Test
    fun `hands the style url, points and selection to the canvas`() {
        content(loaded(selectedKey = "1/a1"))

        composeTestRule.onNodeWithText("canvas:https://tiles.example/s:2:1/a1").assertIsDisplayed()
        composeTestRule.onNodeWithTag(MAP_CANVAS_TAG).assertIsDisplayed()
    }

    @Test
    fun `shows a selected-contact card that opens the contact`() {
        val opened = mutableListOf<Int>()
        content(loaded(selectedKey = "1/a1"), opened = opened)

        composeTestRule.onNodeWithTag("map-selected-card").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ada Lovelace").assertIsDisplayed()
        composeTestRule.onNodeWithText("10 Downing St, London").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open contact").performClick()
        assertEquals(listOf(1), opened)
    }

    @Test
    fun `a selected point without an address label omits the label line`() {
        content(loaded(selectedKey = "2/b1"))

        composeTestRule.onNodeWithText("Grace Hopper").assertIsDisplayed()
        composeTestRule.onNodeWithText("Open contact").assertIsDisplayed()
    }

    @Test
    fun `no card is shown until something is selected`() {
        content(loaded())

        composeTestRule.onNodeWithTag("map-selected-card").assertDoesNotExist()
    }

    @Test
    fun `shows the loading skeleton while loading`() {
        content(MapUiState(isLoading = true))

        composeTestRule.onNodeWithTag(MAP_CANVAS_TAG).assertDoesNotExist()
        composeTestRule.onNodeWithText("Map").assertIsDisplayed()
    }

    @Test
    fun `shows the error with a working retry`() {
        var retried = 0
        content(MapUiState(isLoading = false, error = "No connection"), onRetry = { retried++ })

        composeTestRule.onNodeWithText("No connection").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").performClick()
        assertEquals(1, retried)
        composeTestRule.onNodeWithTag(MAP_CANVAS_TAG).assertDoesNotExist()
    }

    @Test
    fun `explains an empty map and offers no list toggle`() {
        content(loaded(points = emptyList()))

        composeTestRule.onNodeWithText(
            "No contacts have coordinates yet. Add coordinates to an address on a contact to see it here.",
        ).assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Show as list").assertDoesNotExist()
    }

    @Test
    fun `warns when the server truncated the list`() {
        content(loaded(truncated = true))

        composeTestRule.onNodeWithText(
            "Showing the first 2 addresses only; the map is limited to this many points.",
        ).assertIsDisplayed()
    }

    @Test
    fun `no truncation warning for a complete list`() {
        content(loaded())

        composeTestRule.onNodeWithText("Showing the first", substring = true).assertDoesNotExist()
    }

    @Test
    fun `the toggle switches to the accessible list and back`() {
        val toggles = mutableListOf<Boolean>()
        content(loaded(), listToggles = toggles)
        composeTestRule.onNodeWithContentDescription("Show as list").performClick()
        assertEquals(listOf(true), toggles)
    }

    @Test
    fun `the list view lists every point and opens a contact on tap`() {
        val opened = mutableListOf<Int>()
        content(loaded(showList = true), opened = opened)

        composeTestRule.onNodeWithTag("map-point-list").assertIsDisplayed()
        composeTestRule.onNodeWithTag(MAP_CANVAS_TAG).assertDoesNotExist()
        composeTestRule.onNodeWithText("Ada Lovelace").assertIsDisplayed()
        composeTestRule.onNodeWithText("10 Downing St, London").assertIsDisplayed()
        composeTestRule.onNodeWithText("Grace Hopper").performClick()
        assertEquals(listOf(2), opened)
        composeTestRule.onNodeWithContentDescription("Show map").assertIsDisplayed()
    }

    @Test
    fun `the menu button is present only when a menu handler is given`() {
        var menu = 0
        content(loaded(), onMenuClick = { menu++ })
        composeTestRule.onNodeWithContentDescription("Menu").performClick()
        assertEquals(1, menu)
    }

    @Test
    fun `no hamburger when the drawer is permanent`() {
        content(loaded(), onMenuClick = null)

        composeTestRule.onNodeWithContentDescription("Menu").assertDoesNotExist()
    }

    @Test
    fun `passes the accessibility sweep (light)`() {
        content(loaded(selectedKey = "1/a1"))
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `passes the accessibility sweep (dark)`() {
        content(loaded(selectedKey = "1/a1"), darkTheme = true)
        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `the list view passes the accessibility sweep`() {
        content(loaded(showList = true))
        composeTestRule.assertAccessibleSemantics()
    }
}
