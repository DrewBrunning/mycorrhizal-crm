package com.mycorrhizal.crm.feature.network

import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.semantics.SemanticsActions
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertHasClickAction
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasClickAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.unit.dp
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.GraphRepository
import com.mycorrhizal.crm.model.AppVersion
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.GraphChain
import com.mycorrhizal.crm.model.network.GraphChainStep
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.model.network.GraphEntityNode
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.LocalServerVersion
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
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
        deceased: Boolean = false
    ) = GraphChain(
        targetId = targetId,
        targetVCardUid = uid,
        targetName = name,
        depth = depth,
        steps = steps,
        healthScore = healthScore,
        healthBand = healthBand,
        deceased = deceased
    )

    private fun state(
        chains: List<GraphChain> = emptyList(),
        fromName: String = "Alice",
        circles: List<com.mycorrhizal.crm.domain.repository.CircleWithMembers> = emptyList(),
        selectedCircleId: String? = null
    ) = NetworkUiState(
        fromContactId = 1,
        fromVCardUid = "uid-1",
        fromName = fromName,
        depth = 2,
        circles = circles,
        selectedCircleId = selectedCircleId,
        allChains = chains
    )

    private fun setContent(
        uiState: NetworkUiState,
        onOpenContact: (Int) -> Unit = {},
        onDepthChange: (Int) -> Unit = {},
        onRelationApply: () -> Unit = {},
        onCircleSelect: (String?) -> Unit = {},
        showMenu: Boolean = false,
        darkTheme: Boolean = false
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
                    onErrorShown = {}
                )
            }
        }
    }

    @Test
    fun `chains render under their depth headers`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                    ),
                    chain(
                        30,
                        "t3",
                        "Eve",
                        depth = 1,
                        steps = listOf(GraphChainStep(30, "t3", "Eve", "sibling_of"))
                    )
                )
            )
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
            .onNodeWithText("No connections yet")
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
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                    ),
                    chain(
                        0,
                        "t2",
                        "Ghost",
                        depth = 2,
                        steps = listOf(GraphChainStep(0, "t2", "Ghost", "spouse_of"))
                    )
                )
            )
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Child")
            .assert(hasClickAction())
        composeTestRule
            .onNodeWithContentDescription("Ghost — Spouse")
            .assert(
                SemanticsMatcher(
                    "has no click action"
                ) { node -> !node.config.contains(SemanticsActions.OnClick) }
            )
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
                        healthBand = "moss"
                    )
                )
            )
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Child — Healthy")
            .assertIsDisplayed()
    }

    @Test
    fun `a row with no health score keeps the plain two-part description`() {
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                    )
                )
            )
        )

        composeTestRule.onNodeWithContentDescription("Carol — Child").assertIsDisplayed()
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
                        deceased = true
                    )
                )
            )
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Child — Deceased")
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
                        deceased = true
                    )
                )
            )
        )

        composeTestRule
            .onNodeWithContentDescription("Carol — Child — Deceased")
            .assertIsDisplayed()
    }

    @Test
    fun `tapping a row opens the target contact`() {
        var openedId: Int? = null
        setContent(
            uiState = state(
                chains = listOf(
                    chain(
                        10,
                        "t1",
                        "Carol",
                        depth = 1,
                        steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                    )
                )
            ),
            onOpenContact = { openedId = it }
        )

        composeTestRule.onNodeWithContentDescription("Carol — Child").performClick()

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
                    com.mycorrhizal.crm.domain.repository.CircleWithMembers(
                        "c1",
                        "Family",
                        setOf("t1")
                    )
                )
            ),
            onCircleSelect = { selected = it }
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
                    com.mycorrhizal.crm.domain.repository.CircleWithMembers(
                        "c1",
                        "Family",
                        setOf("t1")
                    )
                ),
                selectedCircleId = "c1"
            ),
            onCircleSelect = { selected = it }
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
                fromName = ""
            )
        )

        composeTestRule.onNodeWithText(
            "Choose a starting contact to explore their network."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Choose contact").assertIsDisplayed()
    }

    // Issue #1768 follow-up: a start contact whose name has not resolved
    // shows "Unknown contact", never its raw vCard UID.
    @Test
    fun `a start contact with no name shows unknown contact, not the uid`() {
        setContent(uiState = state(fromName = ""))

        composeTestRule.onNodeWithText("Unknown contact").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("uid-1").assertCountEquals(0)
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
                errorRes = com.mycorrhizal.crm.ui.R.string.network_error_no_vcard_uid
            )
        )

        composeTestRule.onNodeWithText("Contact has no VCard UID").assertIsDisplayed()
        composeTestRule.onNodeWithText("Choose contact").assertIsDisplayed()
    }

    // A failed start-contact fetch is the shared ErrorState (real reason + Retry), not the
    // "no VCard UID" message or the bare picker prompt.
    @Test
    fun `a failed start-contact fetch shows the localized reason with Retry`() {
        var refreshed = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = NetworkUiState(
                        fromContactId = 1,
                        error = com.mycorrhizal.crm.network.ApiError.Client(
                            409,
                            "raw-body-not-shown",
                            com.mycorrhizal.crm.model.network.ErrorReason(
                                com.mycorrhizal.crm.model.network.ErrorReasonCode.FEED_LIMIT_REACHED,
                                mapOf("max" to 50.0)
                            )
                        )
                    ),
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = {},
                    onDepthChange = {},
                    onRelationInputChange = {},
                    onRelationApply = {},
                    onCircleSelect = {},
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {},
                    onRefresh = { refreshed++ }
                )
            }
        }

        composeTestRule.onNodeWithText("feed limit reached (maximum 50 active feeds)")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("raw-body-not-shown").assertDoesNotExist()
        composeTestRule.onNodeWithText("Contact has no VCard UID").assertDoesNotExist()
        composeTestRule.onNodeWithText("Retry").performClick()
        assertEquals(1, refreshed)
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
                        contactSearchResults = listOf(
                            ContactSummary(id = 2, uid = "uid-2", fn = "Bob")
                        )
                    ),
                    onDismiss = {},
                    onSearch = { searched = it },
                    onSelect = { selected = it }
                )
            }
        }

        composeTestRule.onNodeWithTag("picker-search").performTextInput("bo")
        assertEquals("bo", searched)
        composeTestRule.onNodeWithText("Bob").performClick()
        assertEquals(2, selected?.id)
    }

    @Test
    fun `the picker has exactly one dismiss button and left-aligned results`() {
        var dismissed = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactPickerDialog(
                    uiState = NetworkUiState(
                        pickerOpen = true,
                        contactSearchResults = listOf(
                            ContactSummary(id = 2, uid = "uid-2", fn = "Bob")
                        )
                    ),
                    onDismiss = { dismissed++ },
                    onSearch = {},
                    onSelect = {}
                )
            }
        }
        composeTestRule.onAllNodesWithText("Cancel").assertCountEquals(1)
        val field = composeTestRule.onNodeWithTag("picker-search").fetchSemanticsNode()
        val row = composeTestRule.onNodeWithText("Bob").fetchSemanticsNode()
        // Left-aligned with the field above, not centred in the dialog.
        assertTrue(row.boundsInRoot.left - field.boundsInRoot.left < 24f)
        composeTestRule.onNodeWithTag("picker-result-2").assertHeightIsAtLeast(48.dp)
        composeTestRule.onNodeWithText("Cancel").performClick()
        assertEquals(1, dismissed)
    }

    @Test
    fun `the picker shows its empty-search state`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactPickerDialog(
                    uiState = NetworkUiState(
                        pickerOpen = true,
                        contactSearchQuery = "zzz",
                        contactSearchResults = emptyList()
                    ),
                    onDismiss = {},
                    onSearch = {},
                    onSelect = {}
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
                healthBand = "moss"
            )
        )
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

    private fun viewModel(connections: Result<GraphConnectionsResponse>): NetworkViewModel {
        val graphRepository = mockk<GraphRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
        coEvery { graphRepository.selfContactVCardUid() } returns Result.success("uid-self")
        coEvery { contactRepository.resolveByUid(listOf("uid-self")) } returns Result.success(
            mapOf("uid-self" to ContactSummary(id = 1, uid = "uid-self", firstname = "Alice"))
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
                        chain(
                            10,
                            "t1",
                            "Carol",
                            depth = 1,
                            steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                        )
                    )
                )
            )
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
            composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty()
        )
    }

    // Issue #2629: the entity overlay renders inside the list, so with the toggle on a failed
    // traversal does not replace it with the full-screen ErrorState.
    @Test
    fun `a failed traversal with the entity overlay on keeps the list instead of the error state`() {
        val graphRepository = mockk<GraphRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
        coEvery { graphRepository.selfContactVCardUid() } returns Result.success("uid-self")
        coEvery { contactRepository.resolveByUid(listOf("uid-self")) } returns Result.success(
            mapOf("uid-self" to ContactSummary(id = 1, uid = "uid-self", firstname = "Alice"))
        )
        coEvery { graphRepository.getConnections("uid-self", 2, null) } returns
            Result.failure(ApiError.Client(500, "down"))
        coEvery { graphRepository.entityOverlay("uid-self", 2) } returns
            Result.failure(ApiError.Client(500, "overlay-down"))
        val vm = NetworkViewModel(graphRepository, contactRepository, SavedStateHandle())
        composeTestRule.setContent {
            MycorrhizalTheme { NetworkScreen(viewModel = vm) }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()

        composeTestRule.runOnIdle { vm.setShowEntities(true) }
        composeTestRule.waitForIdle()

        // The overlay stays visible, but the traversal failure must not go
        // silent: its reason and Retry stay reachable inline.
        composeTestRule.onNodeWithText("down").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertIsDisplayed()
    }

    // Issue #2629: a failed traversal is the shared ErrorState (localized reason) with a Retry
    // that re-runs the load; the empty state must not show and Retry then brings the rows back.
    @Test
    fun `a failed traversal shows the localized reason with Retry that reloads the network`() {
        val graphRepository = mockk<GraphRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { graphRepository.circlesWithMembers() } returns Result.success(emptyList())
        coEvery { graphRepository.selfContactVCardUid() } returns Result.success("uid-self")
        coEvery { contactRepository.resolveByUid(listOf("uid-self")) } returns Result.success(
            mapOf("uid-self" to ContactSummary(id = 1, uid = "uid-self", firstname = "Alice"))
        )
        coEvery { graphRepository.getConnections("uid-self", 2, null) } returnsMany listOf(
            Result.failure(
                ApiError.Client(
                    409,
                    "raw-body-not-shown",
                    ErrorReason(ErrorReasonCode.FEED_LIMIT_REACHED, mapOf("max" to 50.0))
                )
            ),
            Result.success(
                GraphConnectionsResponse(
                    chains = listOf(
                        chain(
                            10,
                            "t1",
                            "Carol",
                            depth = 1,
                            steps = listOf(GraphChainStep(10, "t1", "Carol", "child_of"))
                        )
                    )
                )
            )
        )
        val vm = NetworkViewModel(graphRepository, contactRepository, SavedStateHandle())
        composeTestRule.setContent {
            MycorrhizalTheme { NetworkScreen(viewModel = vm) }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("feed limit reached (maximum 50 active feeds)")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("raw-body-not-shown").assertDoesNotExist()
        composeTestRule.onNodeWithText("No connections yet").assertDoesNotExist()

        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Carol").assertIsDisplayed()
        composeTestRule.onNodeWithText("Retry").assertDoesNotExist()
        coVerify(exactly = 2) { graphRepository.getConnections("uid-self", 2, null) }
    }

    // --- Issue #2302 / ADR 0043 decision 2: the opt-in, read-only entity overlay -------------

    @Test
    fun `the entity toggle is off by default and reports the user's choice`() {
        var toggled: Boolean? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = state(),
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = {},
                    onDepthChange = {},
                    onRelationInputChange = {},
                    onRelationApply = {},
                    onCircleSelect = {},
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {},
                    onToggleEntities = { toggled = it }
                )
            }
        }
        composeTestRule.onNodeWithTag("network-entities-toggle").assertIsOff()
        composeTestRule.onNodeWithTag("network-entities-toggle").performClick()
        assertEquals(true, toggled)
    }

    @Test
    fun `with the overlay on, entities show with their contacts and open on tap and never as chains`() {
        var opened: String? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = state(chains = listOf(chain(7, "t7", "Ada", 1, emptyList()))).copy(
                        showEntities = true,
                        entityOverlay = listOf(
                            EntityOverlayRow(
                                GraphEntityNode("e-o1", "organisation", "Acme"),
                                listOf("Alice", "Ada")
                            )
                        )
                    ),
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = {},
                    onDepthChange = {},
                    onRelationInputChange = {},
                    onRelationApply = {},
                    onCircleSelect = {},
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {},
                    onOpenEntity = { opened = it }
                )
            }
        }
        composeTestRule.onNodeWithTag("network-entities-toggle").assertIsOn()
        composeTestRule.onNodeWithTag("network-entity-e-o1").assertIsDisplayed()
        composeTestRule.onNodeWithText("Organisation · Alice, Ada").assertIsDisplayed()
        // Tapping opens the entity page (the graph node id is `e-<entityId>`).
        composeTestRule.onNodeWithTag("network-entity-e-o1")
            .assertHasClickAction()
            .assertHeightIsAtLeast(48.dp)
            .performClick()
        assertEquals("o1", opened)
    }

    private fun showOverlayRow(row: EntityOverlayRow) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = state(chains = listOf(chain(7, "t7", "Ada", 1, emptyList()))).copy(
                        showEntities = true,
                        entityOverlay = listOf(row)
                    ),
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = {},
                    onDepthChange = {},
                    onRelationInputChange = {},
                    onRelationApply = {},
                    onCircleSelect = {},
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {},
                    onOpenEntity = {}
                )
            }
        }
    }

    // #2946: the row subtitle names the contact's role ("Employee") when the link has one...
    @Test
    fun `an overlay row shows the link role when it has one`() {
        showOverlayRow(
            EntityOverlayRow(
                GraphEntityNode("e-o1", "organisation", "Acme"),
                listOf("Ada"),
                roles = listOf("employee")
            )
        )
        composeTestRule.onNodeWithText("Employee · Ada").assertIsDisplayed()
        composeTestRule.onNodeWithText("Organisation · Ada").assertDoesNotExist()
    }

    // ...and falls back to the entity kind only when the link has no role.
    @Test
    fun `an overlay row falls back to the kind when the link has no role`() {
        showOverlayRow(
            EntityOverlayRow(GraphEntityNode("e-o2", "organisation", "Globex"), listOf("Ada"))
        )
        composeTestRule.onNodeWithText("Organisation · Ada").assertIsDisplayed()
    }

    @Test
    fun `an empty overlay says nothing is linked`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NetworkScreenContent(
                    uiState = state(chains = listOf(chain(7, "t7", "Ada", 1, emptyList())))
                        .copy(showEntities = true),
                    showMenu = false,
                    onBack = {},
                    onMenuClick = {},
                    onOpenContact = {},
                    onDepthChange = {},
                    onRelationInputChange = {},
                    onRelationApply = {},
                    onCircleSelect = {},
                    onOpenPicker = {},
                    onClosePicker = {},
                    onSearchContacts = {},
                    onSelectFrom = {},
                    onErrorShown = {}
                )
            }
        }
        composeTestRule.onNodeWithText(
            "No organisations, places or pets are linked to these contacts."
        )
            .assertIsDisplayed()
    }

    // Issue #2527: the entity overlay needs `/entities` (floor v1.7.0).
    private fun overlayToggle(serverVersion: AppVersion?) {
        composeTestRule.setContent {
            CompositionLocalProvider(LocalServerVersion provides serverVersion) {
                MycorrhizalTheme {
                    NetworkScreenContent(
                        uiState = state(),
                        showMenu = false,
                        onBack = {},
                        onMenuClick = {},
                        onOpenContact = {},
                        onDepthChange = {},
                        onRelationInputChange = {},
                        onRelationApply = {},
                        onCircleSelect = {},
                        onOpenPicker = {},
                        onClosePicker = {},
                        onSearchContacts = {},
                        onSelectFrom = {},
                        onErrorShown = {},
                        onToggleEntities = {}
                    )
                }
            }
        }
    }

    @Test
    fun `the entity toggle is hidden on a v1_6_0 server`() {
        overlayToggle(AppVersion(1, 6, 0))
        composeTestRule.onNodeWithTag("network-entities-toggle").assertDoesNotExist()
    }

    @Test
    fun `the entity toggle shows on a v1_7_0 server`() {
        overlayToggle(AppVersion(1, 7, 0))
        composeTestRule.onNodeWithTag("network-entities-toggle").assertIsDisplayed()
    }
}
