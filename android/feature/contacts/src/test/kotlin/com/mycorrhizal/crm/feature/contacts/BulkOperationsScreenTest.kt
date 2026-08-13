package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.network.BulkActions
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import org.junit.runner.RunWith

// M9 item 2: BulkOperationsViewModel.run() already accepted circleId/tagId with zero UI call
// sites — these tests are the "reachable" proof that the picker + confirm flow actually reaches
// them, mirroring ContactListScreenTest's split-content pattern. The action row is a
// horizontalScroll'd Row of seven buttons, so the later ones (tag actions) sit off the initial
// viewport — performScrollTo() before clicking, same as ContactListScreenTest's collapsed
// search section, or performClick() silently taps a coordinate outside the composed window and
// nothing happens.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class BulkOperationsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val twoContacts = listOf(
        ContactSummary(id = 1, uid = "uid-1", fn = "Dana", firstname = "Dana"),
        ContactSummary(id = 2, uid = "uid-2", fn = "Carol", firstname = "Carol"),
    )

    private fun setContent(
        uiState: BulkUiState,
        onToggle: (Int) -> Unit = {},
        onRun: (String, String?, String?) -> Unit = { _, _, _ -> },
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                BulkOperationsScreenContent(
                    uiState = uiState,
                    onToggle = onToggle,
                    onRun = onRun,
                )
            }
        }
    }

    @Test
    fun `add to circle opens a picker listing the loaded circles`() {
        setContent(
            BulkUiState(
                contacts = twoContacts,
                selected = setOf(1),
                circles = listOf(Circle(id = "c-1", name = "Book club"), Circle(id = "c-2", name = "Family")),
            ),
        )

        composeTestRule.onNodeWithTag("bulk-add-circle").performScrollTo().performClick()

        composeTestRule.onNodeWithText("Choose a circle").assertIsDisplayed()
        composeTestRule.onNodeWithText("Book club").assertIsDisplayed()
        composeTestRule.onNodeWithText("Family").assertIsDisplayed()
    }

    @Test
    fun `picking a circle then confirming runs add_circle with that circle id`() {
        var ranAction: String? = null
        var ranCircleId: String? = null
        var ranTagId: String? = null
        setContent(
            BulkUiState(
                contacts = twoContacts,
                selected = setOf(1, 2),
                circles = listOf(Circle(id = "c-1", name = "Book club")),
            ),
            onRun = { action, circleId, tagId ->
                ranAction = action
                ranCircleId = circleId
                ranTagId = tagId
            },
        )

        composeTestRule.onNodeWithTag("bulk-add-circle").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Book club").performClick()
        // Selecting a circle closes the picker and opens the existing confirm dialog.
        composeTestRule.onNodeWithText("Confirm bulk action?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals(BulkActions.ADD_CIRCLE, ranAction)
        assertEquals("c-1", ranCircleId)
        assertEquals(null, ranTagId)
    }

    @Test
    fun `picking a tag then confirming runs add_tag with that tag id`() {
        var ranAction: String? = null
        var ranTagId: String? = null
        setContent(
            BulkUiState(
                contacts = twoContacts,
                selected = setOf(1),
                tags = listOf(Tag(id = "t-1", name = "VIP")),
            ),
            onRun = { action, _, tagId ->
                ranAction = action
                ranTagId = tagId
            },
        )

        composeTestRule.onNodeWithTag("bulk-add-tag").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Choose a tag").assertIsDisplayed()
        composeTestRule.onNodeWithText("VIP").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals(BulkActions.ADD_TAG, ranAction)
        assertEquals("t-1", ranTagId)
    }

    @Test
    fun `remove tag opens the same picker but runs remove_tag`() {
        var ranAction: String? = null
        setContent(
            BulkUiState(
                contacts = twoContacts,
                selected = setOf(1),
                tags = listOf(Tag(id = "t-1", name = "VIP")),
            ),
            onRun = { action, _, _ -> ranAction = action },
        )

        composeTestRule.onNodeWithTag("bulk-remove-tag").performScrollTo().performClick()
        composeTestRule.onNodeWithText("VIP").performClick()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals(BulkActions.REMOVE_TAG, ranAction)
    }

    @Test
    fun `an empty circle list shows the no-circles message instead of a blank picker`() {
        setContent(BulkUiState(contacts = twoContacts, circles = emptyList()))

        composeTestRule.onNodeWithTag("bulk-add-circle").performScrollTo().performClick()

        composeTestRule.onNodeWithText("No circles yet").assertIsDisplayed()
    }

    @Test
    fun `an empty tag list shows the no-tags message instead of a blank picker`() {
        setContent(BulkUiState(contacts = twoContacts, tags = emptyList()))

        composeTestRule.onNodeWithTag("bulk-add-tag").performScrollTo().performClick()

        composeTestRule.onNodeWithText("No tags yet").assertIsDisplayed()
    }

    @Test
    fun `plain archive still skips the picker and goes straight to confirm`() {
        var ranAction: String? = null
        setContent(
            BulkUiState(contacts = twoContacts, selected = setOf(1)),
            onRun = { action, _, _ -> ranAction = action },
        )

        composeTestRule.onNodeWithTag("bulk-archive").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Confirm bulk action?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Confirm").performClick()

        assertEquals(BulkActions.ARCHIVE, ranAction)
    }
}
