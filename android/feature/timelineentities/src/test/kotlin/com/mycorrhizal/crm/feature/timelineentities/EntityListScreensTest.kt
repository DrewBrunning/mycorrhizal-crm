package com.mycorrhizal.crm.feature.timelineentities

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M17 (99-M17-android-entity-scaffold-edit-delete-confirm.md). Two layers,
// matching the module's existing test split (ViewModel logic tested
// separately in TimelineEntitiesViewModelTest, with no ViewModel/coroutines
// here):
//
//   1. EntityListScaffoldTest -- the shared delete-confirmation + tap-to-edit
//      mechanics, entity-agnostic by construction (one scaffold, so one set
//      of tests covers all four callers structurally).
//   2. One test class per entity dialog -- edit mode's pre-fill and the
//      confirmed values reaching onConfirm are genuinely entity-specific
//      (different fields per type), so this is the layer the ticket's
//      "parameterize across all four entity types" requirement lands on.

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(ExperimentalMaterial3Api::class)
class EntityListScaffoldTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        items: List<EntityItem>,
        onItemClick: (String) -> Unit = {},
        onDelete: (String) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntityListScaffold(
                    title = "Life events",
                    addLabel = "New life event",
                    uiState = EntityListUiState(items = items),
                    onAdd = {},
                    onItemClick = onItemClick,
                    onDelete = onDelete,
                    onErrorShown = {},
                    onBack = {},
                    dialog = {},
                )
            }
        }
    }

    @Test
    fun `tapping a row invokes onItemClick`() {
        var clickedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onItemClick = { clickedId = it },
        )
        composeTestRule.onNodeWithText("Moved to Madison").performClick()
        assertEquals("e1", clickedId)
    }

    @Test
    fun `delete asks first -- tapping delete shows a confirmation and does not call onDelete`() {
        var deletedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete").performClick()

        composeTestRule.onNodeWithText("Delete item?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete “Moved to Madison”? This cannot be undone.").assertIsDisplayed()
        // The confirmation is up; the repository call has NOT happened --
        // this is the whole point of the ticket's requirement. A test that
        // only asserted "delete calls onDelete" would pass against the old,
        // unsafe, unconfirmed behavior too.
        assertNull(deletedId)
    }

    @Test
    fun `cancel is inert -- dismissing the confirmation issues no call and leaves the item present`() {
        var deletedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()

        assertNull(deletedId)
        composeTestRule.onNodeWithText("Moved to Madison").assertIsDisplayed()
    }

    @Test
    fun `confirming the dialog calls onDelete with the right id`() {
        var deletedId: String? = null
        setContent(
            items = listOf(EntityItem(id = "e1", label = "Moved to Madison")),
            onDelete = { deletedId = it },
        )
        composeTestRule.onNodeWithContentDescription("Delete").performClick()
        // The dialog's confirm TextButton's text ("Delete") is the only node
        // with that exact text once the dialog is up -- the row's own delete
        // affordance is an icon with a contentDescription, not a text node.
        composeTestRule.onNodeWithText("Delete").performClick()

        assertEquals("e1", deletedId)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class LifeEventDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `add mode starts blank and reports the typed values`() {
        var confirmedType: String? = null
        var confirmedDescription: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                LifeEventDialog(
                    initial = null,
                    onConfirm = { type, description -> confirmedType = type; confirmedDescription = description },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("New life event").assertIsDisplayed()
        composeTestRule.onNodeWithText("Type").performTextReplacement("moved")
        composeTestRule.onNodeWithText("Description").performTextReplacement("Moved to Madison")
        composeTestRule.onNodeWithText("Create").performClick()

        assertEquals("moved", confirmedType)
        assertEquals("Moved to Madison", confirmedDescription)
    }

    @Test
    fun `edit mode pre-fills from the loaded entity and saving reports the edited values`() {
        var confirmedType: String? = null
        var confirmedDescription: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                LifeEventDialog(
                    initial = LifeEvent(id = "e1", entityId = "uid", type = "moved", description = "Moved to Madison"),
                    onConfirm = { type, description -> confirmedType = type; confirmedDescription = description },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit life event").assertIsDisplayed()
        composeTestRule.onNodeWithText("moved").assertIsDisplayed()
        composeTestRule.onNodeWithText("Moved to Madison").assertIsDisplayed()

        composeTestRule.onNodeWithText("Moved to Madison").performTextReplacement("Moved to Chicago")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("moved", confirmedType)
        assertEquals("Moved to Chicago", confirmedDescription)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class GiftDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `edit mode pre-fills from the loaded entity and saving reports the edited value`() {
        var confirmedDescription: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                GiftDialog(
                    initial = Gift(id = "g1", entityId = "uid", description = "Socks"),
                    onConfirm = { description -> confirmedDescription = description },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit gift").assertIsDisplayed()
        composeTestRule.onNodeWithText("Socks").assertIsDisplayed()

        composeTestRule.onNodeWithText("Socks").performTextReplacement("Wool socks")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("Wool socks", confirmedDescription)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class PreferenceDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `edit mode pre-fills from the loaded entity and saving reports the edited values`() {
        var confirmedCategory: String? = null
        var confirmedValue: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                PreferenceDialog(
                    initial = Preference(id = "p1", entityId = "uid", category = "food", value = "peanuts"),
                    onConfirm = { category, value -> confirmedCategory = category; confirmedValue = value },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit preference").assertIsDisplayed()
        composeTestRule.onNodeWithText("food").assertIsDisplayed()
        composeTestRule.onNodeWithText("peanuts").assertIsDisplayed()

        composeTestRule.onNodeWithText("peanuts").performTextReplacement("tree nuts")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("food", confirmedCategory)
        assertEquals("tree nuts", confirmedValue)
    }
}

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AgendaDialogTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `edit mode pre-fills from the loaded entity and saving reports the edited value`() {
        var confirmedContent: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                AgendaDialog(
                    initial = ConversationAgenda(id = "a1", entityId = "uid", content = "Ask about the move"),
                    onConfirm = { content -> confirmedContent = content },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Edit item").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ask about the move").assertIsDisplayed()

        composeTestRule.onNodeWithText("Ask about the move").performTextReplacement("Ask about the new place")
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("Ask about the new place", confirmedContent)
    }
}
