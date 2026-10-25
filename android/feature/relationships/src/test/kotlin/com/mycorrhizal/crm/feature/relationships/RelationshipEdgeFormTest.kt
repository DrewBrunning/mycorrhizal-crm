package com.mycorrhizal.crm.feature.relationships

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.hasContentDescription
import androidx.compose.ui.test.junit4.StateRestorationTester
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToIndex
import androidx.compose.ui.test.performTextReplacement
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #1773 (maintainer decision 2026-10-20): the relationship create/edit form is a
 * full-screen form, not an AlertDialog. On the real screen: New/Edit replace the list, Back
 * returns without saving, Save creates/updates and returns, the open form and its input
 * survive a configuration change, the list keeps its scroll position, the pickers are
 * labelled single nodes, and the contact search still works.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RelationshipEdgeFormTest {
    @get:Rule
    val composeTestRule = createComposeRule()

    private val isDropdown = SemanticsMatcher.expectValue(
        SemanticsProperties.Role,
        Role.DropdownList
    )

    private val edgeRepository = mockk<RelationshipEdgeRepository>()
    private val contactRepository = mockk<ContactRepository>()

    private fun edge(id: String, other: String = "u-other") = RelationshipEdge(
        id = id,
        sourceId = other,
        targetId = "u-viewed",
        type = RelationshipEdgeTypes.FRIEND_OF,
        status = RelationshipEdgeStatuses.CONFIRMED
    )

    private fun viewModel(
        edges: List<RelationshipEdge> = emptyList(),
        resolve: Map<String, ContactSummary> = emptyMap()
    ): RelationshipsViewModel {
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = "u-viewed"))
        )
        coEvery {
            edgeRepository.listForContact(
                "u-viewed",
                null,
                null
            )
        } returns Result.success(edges)
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(resolve)
        return RelationshipsViewModel(
            edgeRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 5))
        )
    }

    private fun show(vm: RelationshipsViewModel) = composeTestRule.setContent {
        MycorrhizalTheme {
            RelationshipsScreen(
                onBack = {},
                onNavigateToContact = {},
                viewModel = vm
            )
        }
    }

    private fun stubAliceSearch() {
        coEvery {
            contactRepository.listContacts(
                search = "Ali",
                limit = 25
            )
        } returns Result.success(
            ContactsPage(
                contacts = listOf(
                    ContactSummary(id = 9, uid = "u-alice", firstname = "Alice", lastname = "Smith")
                ),
                nextCursor = "",
                limit = 25,
                sync = null
            )
        )
    }

    private fun searchAndPickAlice() {
        composeTestRule.onNodeWithContentDescription("New relationship").performClick()
        composeTestRule.onNodeWithText("Link to contact").performClick()
        composeTestRule.onNodeWithText("Choose a contact to continue").assertIsDisplayed()
        composeTestRule.onNodeWithTag("relationship-contact-search").performTextReplacement("Ali")
        composeTestRule.waitUntil(5_000) {
            composeTestRule.onAllNodesWithText("Alice Smith").fetchSemanticsNodes().isNotEmpty()
        }
        composeTestRule.onNodeWithText("Alice Smith").performClick()
    }

    @Test
    fun `new relationship replaces the list, back discards, save creates and returns`() {
        coEvery { edgeRepository.create(any()) } returns Result.success(edge("e-new"))
        show(viewModel())

        composeTestRule.onNodeWithContentDescription("New relationship").performClick()
        composeTestRule.onNodeWithContentDescription("New relationship").assertDoesNotExist()
        composeTestRule.onNodeWithText("Enter a name to continue").assertIsDisplayed()

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithTag("relationship-type").assertDoesNotExist()
        composeTestRule.onNodeWithContentDescription("New relationship").assertIsDisplayed()
        coVerify(exactly = 0) { edgeRepository.create(any()) }

        composeTestRule.onNodeWithContentDescription("New relationship").performClick()
        composeTestRule.onNodeWithText(
            "Other contact name",
            substring = true
        ).performTextReplacement("Casey")
        composeTestRule.onNodeWithText("Create").performClick()

        val input = slot<RelationshipEdgeInput>()
        coVerify(exactly = 1) { edgeRepository.create(capture(input)) }
        // The viewed contact is the target; the type describes the OTHER party (the source).
        assertEquals("u-viewed", input.captured.targetId)
        assertEquals(RelationshipEdgeTypes.FRIEND_OF, input.captured.type)
        assertEquals("Casey", input.captured.sourceThin?.name)
        composeTestRule.onNodeWithTag("relationship-type").assertDoesNotExist()
    }

    @Test
    fun `editing an edge opens the form pre-filled with the other party and save updates`() {
        coEvery { edgeRepository.update("e1", any()) } returns Result.success(edge("e1"))
        show(
            viewModel(
                edges = listOf(edge("e1")),
                resolve = mapOf(
                    "u-other" to ContactSummary(id = 9, uid = "u-other", firstname = "Alice")
                )
            )
        )

        composeTestRule.onNodeWithContentDescription("Edit Alice").performClick()
        composeTestRule.onNodeWithText("Edit relationship").assertIsDisplayed()
        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "relationship-type"
        ).assert(hasContentDescription("Type, Friend"))
        composeTestRule.onNodeWithText("Save").performClick()

        coVerify(exactly = 1) { edgeRepository.update("e1", any()) }
        composeTestRule.onNodeWithContentDescription("Edit Alice").assertIsDisplayed()
    }

    @Test
    fun `type and sensitivity are labelled single drop-down nodes`() {
        show(viewModel())
        composeTestRule.onNodeWithContentDescription("New relationship").performClick()

        composeTestRule.onNodeWithTag("relationship-type")
            .assert(hasContentDescription("Type, Friend")).assert(isDropdown)
        composeTestRule.onNodeWithTag("relationship-sensitivity")
            .assert(hasContentDescription("Sensitivity, Normal")).assert(isDropdown)

        composeTestRule.onNodeWithTag("relationship-sensitivity").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Private").performClick()
        composeTestRule.onNodeWithTag("relationship-sensitivity")
            .assert(hasContentDescription("Sensitivity, Private"))
    }

    @Test
    fun `contact search finds a contact, selecting it enables create and links the source`() {
        stubAliceSearch()
        coEvery {
            edgeRepository.create(
                any()
            )
        } returns Result.success(edge("e-new", other = "u-alice"))
        show(viewModel())

        searchAndPickAlice()
        composeTestRule.onNodeWithText("Create").performClick()

        val input = slot<RelationshipEdgeInput>()
        coVerify(exactly = 1) { edgeRepository.create(capture(input)) }
        assertEquals("u-alice", input.captured.sourceId)
    }

    @Test
    fun `an open relationship form and its typed name survive a configuration change`() {
        val vm = viewModel()
        val restoration = StateRestorationTester(composeTestRule)
        restoration.setContent {
            MycorrhizalTheme {
                RelationshipsScreen(
                    onBack = {},
                    onNavigateToContact = {},
                    viewModel = vm
                )
            }
        }

        composeTestRule.onNodeWithContentDescription("New relationship").performClick()
        composeTestRule.onNodeWithText(
            "Other contact name",
            substring = true
        ).performTextReplacement("Casey")
        restoration.emulateSavedInstanceStateRestore()

        composeTestRule.onNodeWithTag("relationship-type").assertIsDisplayed()
        composeTestRule.onNodeWithText("Casey").assertIsDisplayed()
        coVerify(exactly = 0) { edgeRepository.create(any()) }
    }

    @Test
    fun `a selected linked contact survives a configuration change`() {
        stubAliceSearch()
        val vm = viewModel()
        val restoration = StateRestorationTester(composeTestRule)
        restoration.setContent {
            MycorrhizalTheme {
                RelationshipsScreen(
                    onBack = {},
                    onNavigateToContact = {},
                    viewModel = vm
                )
            }
        }
        searchAndPickAlice()

        restoration.emulateSavedInstanceStateRestore()

        // Still selected (not just listed as a search result again): Create is enabled.
        composeTestRule.onNodeWithText("Create").assertIsEnabled()
        composeTestRule.onNodeWithText("Alice Smith").assertIsDisplayed()
    }

    @Test
    fun `back from an edit form returns to the same scroll position in the list`() {
        val edges = (0 until 40).map { edge("e$it", other = "u$it") }
        val resolve = (0 until 40).associate {
            "u$it" to ContactSummary(id = 100 + it, uid = "u$it", firstname = "Person$it")
        }
        show(viewModel(edges = edges, resolve = resolve))

        composeTestRule.onNodeWithTag("relationship-list").performScrollToIndex(30)
        composeTestRule.onNodeWithContentDescription("Edit Person30").performClick()
        composeTestRule.onNodeWithTag("relationship-type").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Back").performClick()

        composeTestRule.onNodeWithText("Person30").assertIsDisplayed()
        composeTestRule.onNodeWithText("Person0").assertDoesNotExist()
    }

    // Issue #1751: unsaved-changes guard.

    @Test
    fun `a dirty relationship form asks before discarding and keep editing keeps the name`() {
        show(viewModel())
        composeTestRule.onNodeWithContentDescription("New relationship").performClick()
        composeTestRule.onNodeWithText(
            "Other contact name",
            substring = true
        ).performTextReplacement("Casey")

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Your edits to this relationship haven't been saved."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("relationship-type").assertIsDisplayed()

        composeTestRule.onNodeWithText("Keep editing").performClick()
        composeTestRule.onNodeWithText("Casey").assertIsDisplayed()

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithText("Discard").performClick()
        composeTestRule.onNodeWithTag("relationship-type").assertDoesNotExist()
    }
}
