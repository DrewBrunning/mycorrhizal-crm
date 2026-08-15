package com.mycorrhizal.crm.feature.network

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.model.network.GraphChainStep
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w400dp-h1400dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NetworkScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun chain(
        targetId: Int,
        uid: String,
        name: String,
        depth: Int,
        steps: List<GraphChainStep>,
    ) = GraphChain(targetId = targetId, targetVCardUid = uid, targetName = name, depth = depth, steps = steps)

    private fun state(
        chains: List<GraphChain> = emptyList(),
        fromName: String = "Alice",
        circles: List<com.mycorrhizal.crm.domain.repository.CircleWithMembers> = emptyList(),
        selectedCircleId: String? = null,
    ) = NetworkUiState(
        fromContactId = 1,
        fromVCardUid = "uid-1",
        fromName = fromName,
        depth = 2,
        circles = circles,
        selectedCircleId = selectedCircleId,
        allChains = chains,
    )

    private fun setContent(
        uiState: NetworkUiState,
        onOpenContact: (Int) -> Unit = {},
        onDepthChange: (Int) -> Unit = {},
        onRelationApply: () -> Unit = {},
        onCircleSelect: (String?) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = uiState,
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = onOpenContact,
                    onDepthChange = onDepthChange,
                    onRelationInputChange = {},
                    onRelationApply = onRelationApply,
                    onCircleSelect = onCircleSelect,
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {},
                )
            }
        }
    }

    @Test
    fun `chains are grouped under their depth headers`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                    chain(20, "t2", "Dave", depth = 2, steps = listOf(GraphChainStep(20, "t2", "Dave", "spouse_of"))),
                    chain(30, "t3", "Eve", depth = 1, steps = listOf(GraphChainStep(30, "t3", "Eve", "sibling_of"))),
                ),
            ),
        )

        composeTestRule.onNodeWithText("Direct").assertIsDisplayed()
        composeTestRule.onNodeWithText("2 hops away").assertIsDisplayed()
        composeTestRule.onNodeWithText("Carol").assertIsDisplayed()
        composeTestRule.onNodeWithText("Dave").assertIsDisplayed()
        composeTestRule.onNodeWithText("Eve").assertIsDisplayed()
    }

    @Test
    fun `an empty graph shows the empty state`() {
        setContent(uiState = state(chains = emptyList()))

        composeTestRule
            .onNodeWithText("No connections found. Add relationships to build a network here.")
            .assertIsDisplayed()
    }

    @Test
    fun `every row exposes a content description and is tappable`() {
        // M14's accessibility requirement: each row is one merged, announced,
        // focusable item (the reason the list replaced a canvas). A row whose
        // target_id is 0 (soft-deleted intermediate) renders but is NOT
        // tappable — that distinction is asserted here too.
        setContent(
            uiState = state(
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                    chain(0, "t2", "Ghost", depth = 2, steps = listOf(GraphChainStep(0, "t2", "Ghost", "spouse_of"))),
                ),
            ),
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Carol (child of)")
            .assert(hasClickAction())
        composeTestRule
            .onNodeWithContentDescription("Ghost — Ghost (spouse of)")
            .assert(SemanticsMatcher("has no click action") { node -> !node.config.contains(SemanticsActions.OnClick) })
    }

    @Test
    fun `tapping a row opens the target contact`() {
        var openedId: Int? = null
        setContent(
            uiState = state(
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                ),
            ),
            onOpenContact = { openedId = it },
        )

        composeTestRule.onNodeWithContentDescription("Carol — Carol (child of)").performClick()

        assertEquals(10, openedId)
    }

    @Test
    fun `selecting a depth chip triggers onDepthChange`() {
        var changedDepth: Int? = null
        setContent(uiState = state(), onDepthChange = { changedDepth = it })

        composeTestRule.onNodeWithTag("depth-3").performClick()

        assertEquals(3, changedDepth)
    }

    @Test
    fun `applying the relation filter triggers onRelationApply`() {
        var applied = false
        setContent(uiState = state(), onRelationApply = { applied = true })

        composeTestRule.onNodeWithTag("relation-apply").performClick()

        assertEquals(true, applied)
    }

    @Test
    fun `selecting a circle from the filter triggers onCircleSelect`() {
        var selected: String? = "unset"
        setContent(
            uiState = state(
                circles = listOf(
                    com.mycorrhizal.crm.domain.repository.CircleWithMembers("c1", "Family", setOf("t1")),
                ),
            ),
            onCircleSelect = { selected = it },
        )

        composeTestRule.onNodeWithTag("circle-filter").performClick()
        composeTestRule.onNodeWithText("Family").performClick()

        assertEquals("c1", selected)
    }

    @Test
    fun `clearing the circle filter to all circles triggers onCircleSelect null`() {
        var selected: String? = "unset"
        setContent(
            uiState = state(
                circles = listOf(
                    com.mycorrhizal.crm.domain.repository.CircleWithMembers("c1", "Family", setOf("t1")),
                ),
                selectedCircleId = "c1",
            ),
            onCircleSelect = { selected = it },
        )

        composeTestRule.onNodeWithTag("circle-filter").performClick()
        composeTestRule.onNodeWithText("All circles").performClick()

        assertNull(selected)
    }

    @Test
    fun `a missing from contact prompts the picker instead of the list`() {
        setContent(
            uiState = NetworkUiState(
                fromContactId = null,
                fromVCardUid = "",
                fromName = "",
            ),
        )

        composeTestRule.onNodeWithText("Choose a starting contact to explore their network.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Choose contact").assertIsDisplayed()
    }
}
