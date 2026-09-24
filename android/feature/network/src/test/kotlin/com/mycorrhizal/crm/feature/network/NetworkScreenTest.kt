package com.mycorrhizal.crm.feature.network

import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.model.network.GraphChainStep
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
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
        // Issue #383 (ADR-0023): optional so every existing call site (none
        // of which cares about the score) is unaffected.
        healthScore: Int? = null,
        healthBand: String? = null,
        // Issue #1193: optional, same reasoning -- defaults to false so
        // every existing call site is unaffected.
        deceased: Boolean = false,
    ) = GraphChain(
        targetId = targetId,
        targetVCardUid = uid,
        targetName = name,
        depth = depth,
        steps = steps,
        healthScore = healthScore,
        healthBand = healthBand,
        deceased = deceased,
    )

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
        showMenu: Boolean = false,
        darkTheme: Boolean = false,
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = darkTheme) {
                NetworkScreenContent(
                    uiState = uiState,
                    showMenu = showMenu,
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
    fun `chains render under their depth headers`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                    chain(30, "t3", "Eve", depth = 1, steps = listOf(GraphChainStep(30, "t3", "Eve", "sibling_of"))),
                ),
            ),
        )

        // The depth headers + their rows render (the exact depth->chain mapping
        // is asserted at the ViewModel level — `groupedChains[1]` etc. — since
        // LazyColumn only composes the visible window in the test viewport).
        composeTestRule.onNodeWithText("Direct").assertIsDisplayed()
        composeTestRule.onNodeWithText("Carol").assertIsDisplayed()
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
    fun `a row with a health score spells the band out in words in its content description`() {
        // Issue #383 (ADR-0023): color alone must never carry the band —
        // the merged row description always spells it out, same rule the
        // dot's color follows (mirrors CadenceScreen's icon-tint precedent).
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of")),
                        healthScore = 72,
                        healthBand = "moss",
                    ),
                ),
            ),
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Carol (child of) — Healthy")
            .assertIsDisplayed()
    }

    @Test
    fun `a row with no health score keeps the plain two-part description`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                ),
            ),
        )

        composeTestRule.onNodeWithContentDescription("Carol — Carol (child of)").assertIsDisplayed()
    }

    // --- Issue #1193: a deceased target overrides the health-band indicator ---

    @Test
    fun `a deceased target's content description says Deceased, not its health band`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of")),
                        healthScore = 72,
                        healthBand = "moss",
                        deceased = true,
                    ),
                ),
            ),
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Carol (child of) — Deceased")
            .assertIsDisplayed()
    }

    @Test
    fun `a deceased target with no health band still gets the Deceased description`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of")),
                        deceased = true,
                    ),
                ),
            ),
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Carol (child of) — Deceased")
            .assertIsDisplayed()
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

        assertTrue(applied)
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

    @Test
    fun `a hard start error renders above the picker affordance`() {
        // A starting contact with no VCard UID must show the localized error
        // (not silently fall back to the prompt) while still allowing a
        // different start contact to be chosen.
        setContent(
            uiState = NetworkUiState(
                fromContactId = 1,
                fromVCardUid = "",
                fromName = "",
                errorRes = com.mycorrhizal.crm.ui.R.string.network_error_no_vcard_uid,
            ),
        )

        composeTestRule.onNodeWithText("Contact has no VCard UID").assertIsDisplayed()
        composeTestRule.onNodeWithText("Choose contact").assertIsDisplayed()
    }

    @Test
    fun `typing in the picker searches and selecting a result invokes onSelectFrom`() {
        var searched: String? = null
        var selected: ContactSummary? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactPickerDialog(
                    uiState = NetworkUiState(
                        pickerOpen = true,
                        contactSearchResults = listOf(ContactSummary(id = 2, uid = "uid-2", fn = "Bob")),
                    ),
                    onDismiss = {},
                    onSearch = { searched = it },
                    onSelect = { selected = it },
                )
            }
        }

        composeTestRule.onNodeWithTag("picker-search").performTextInput("bo")
        assertEquals("bo", searched)
        composeTestRule.onNodeWithText("Bob").performClick()
        assertEquals(2, selected?.id)
    }

    @Test
    fun `the picker shows its empty-search state`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactPickerDialog(
                    uiState = NetworkUiState(
                        pickerOpen = true,
                        contactSearchQuery = "zzz",
                        contactSearchResults = emptyList(),
                    ),
                    onDismiss = {},
                    onSearch = {},
                    onSelect = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Type to search for a contact.").assertIsDisplayed()
    }

    // --- Issue #214: Compose semantics a11y sweep (the axe-core analog) -----

    private fun populatedState() = state(
        chains = listOf(
            chain(
                10,
                "t1",
                "Carol",
                depth = 1,
                steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of")),
                // Issue #383: exercise the health-score dot in the a11y sweep too.
                healthScore = 72,
                healthBand = "moss",
            ),
        ),
    )

    @Test
    fun `network screen has no accessibility violations (light)`() {
        setContent(populatedState(), showMenu = true, darkTheme = false)

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `network screen has no accessibility violations (dark)`() {
        setContent(populatedState(), showMenu = true, darkTheme = true)

        composeTestRule.assertAccessibleSemantics()
    }

    // --- Top-level NetworkScreen against a real ViewModel: the
    // RefreshableContent onRefresh wrapper (line 91) and the inline error
    // branch only execute when the real screen wires the VM's callbacks.

    private fun viewModel(
        connections: Result<GraphConnectionsResponse>,
    ): NetworkViewModel {
        val graphRepository = mockk<GraphRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
        coEvery { graphRepository.selfContactVCardUid() } returns Result.success("uid-self")
        coEvery { contactRepository.resolveByUid(listOf("uid-self")) } returns Result.success(
            mapOf("uid-self" to ContactSummary(id = 1, uid = "uid-self", firstname = "Alice")),
        )
        coEvery { graphRepository.getConnections("uid-self", 2, null) } returns connections
        return NetworkViewModel(graphRepository, contactRepository, SavedStateHandle())
    }

    @Test
    fun `top-level screen renders the view model's chains`() {
        val vm = viewModel(
            Result.success(
                GraphConnectionsResponse(
                    chains = listOf(
                        chain(10, "t1", "Carol", depth = 1, steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))),
                    ),
                ),
            ),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { NetworkScreen(viewModel = vm) }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Carol").assertIsDisplayed()
    }

    @Test
    fun `top-level screen renders the inline error when the traversal fails`() {
        val vm = viewModel(Result.failure(ApiError.Client(500, "boom")))
        composeTestRule.setContent {
            MycorrhizalTheme { NetworkScreen(viewModel = vm) }
        }
        composeTestRule.waitForIdle()

        assertTrue(
            composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty(),
        )
    }
}
