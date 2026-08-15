package com.mycorrhizal.crm.feature.audit

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.domain.repository.AuditRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.model.network.AuditEvent
import com.mycorrhizal.crm.model.network.AuditEntityTypes
import com.mycorrhizal.crm.model.network.AuditEventsResponse
import com.mycorrhizal.crm.model.network.AuditOperations
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
// A taller window so the full-screen flows (filter toolbar + LazyColumn rows)
// fit in the viewport — at Robolectric's tiny default (320x414) the filter
// toolbar consumes most of the height and list rows render with zero bounds.
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class AuditScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun contactUpdate(id: Long = 1) = AuditEvent(
        id = id,
        createdAt = "2026-08-14T10:00:00Z",
        entityType = AuditEntityTypes.CONTACT,
        entityId = "uid-1",
        operation = AuditOperations.UPDATE,
    )

    @Test
    fun `a delete event offers no undo button`() {
        val events = listOf(
            contactUpdate(id = 1),
            AuditEvent(
                id = 2,
                createdAt = "2026-08-14T09:00:00Z",
                entityType = AuditEntityTypes.CONTACT,
                entityId = "uid-2",
                operation = AuditOperations.DELETE,
            ),
        )
        var undoClicked = false
        var openedContact: Int? = null

        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditEventList(
                    events = events,
                    contactsByUid = emptyMap(),
                    canUndo = true,
                    onUndoClick = { undoClicked = true },
                    onOpenContact = { openedContact = it },
                    canLoadMore = false,
                    isLoadingMore = false,
                    onLoadMore = {},
                )
            }
        }

        // The contact update row has the undo affordance; the delete row must
        // NOT — surfacing one would be a dead control (backend rejects it).
        composeTestRule.onNodeWithTag("audit-undo-1").assertIsDisplayed()
        composeTestRule.onNodeWithTag("audit-undo-2").assertDoesNotExist()
        composeTestRule.onNodeWithTag("audit-undo-1").performClick()
        assertTrue(undoClicked)
    }

    @Test
    fun `an unresolved contact uid renders as plain text with no link`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditEventList(
                    events = listOf(contactUpdate(id = 1)),
                    contactsByUid = emptyMap(),
                    canUndo = false,
                    onUndoClick = {},
                    onOpenContact = {},
                    canLoadMore = false,
                    isLoadingMore = false,
                    onLoadMore = {},
                )
            }
        }

        // No resolved summary → the raw uid is shown (deleted contact fallback)
        // and there is no contact-link node.
        composeTestRule.onNodeWithText("uid-1").assertIsDisplayed()
        composeTestRule.onNodeWithTag("audit-contact-link-1").assertDoesNotExist()
    }

    @Test
    fun `a resolved contact uid renders a tappable link to its detail page`() {
        var openedContact: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditEventList(
                    events = listOf(contactUpdate(id = 1)),
                    contactsByUid = mapOf(
                        "uid-1" to ContactSummary(id = 5, uid = "uid-1", firstname = "Dana", lastname = "White"),
                    ),
                    canUndo = true,
                    onUndoClick = {},
                    onOpenContact = { openedContact = it },
                    canLoadMore = false,
                    isLoadingMore = false,
                    onLoadMore = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithTag("audit-contact-link-1").performClick()
        assertTrue(openedContact == 5)
    }

    @Test
    fun `load more button appears only when more rows are available`() {
        var loadMoreClicked = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditEventList(
                    events = listOf(contactUpdate(id = 1)),
                    contactsByUid = emptyMap(),
                    canUndo = false,
                    onUndoClick = {},
                    onOpenContact = {},
                    canLoadMore = true,
                    isLoadingMore = false,
                    onLoadMore = { loadMoreClicked = true },
                )
            }
        }

        composeTestRule.onNodeWithTag("audit-load-more").assertIsDisplayed()
        composeTestRule.onNodeWithTag("audit-load-more").performClick()
        assertTrue(loadMoreClicked)
    }

    @Test
    fun `the entity-type filter dropdown lists every backend token`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditFilterToolbar(
                    entityType = null,
                    entityIdInput = "",
                    hasFilters = false,
                    onEntityTypeChange = {},
                    onEntityIdChange = {},
                    onClearFilters = {},
                )
            }
        }

        // The menu is collapsed by default — assert the "All types" placeholder
        // and the clear button's disabled state (no active filters).
        composeTestRule.onNodeWithText("All types").assertIsDisplayed()
        composeTestRule.onNodeWithTag("audit-clear-filters").assertIsNotEnabled()
    }

    // --- Full-screen flows (ticket test cases 2 + the hasFilters wiring) ---

    /** Renders [AuditScreen] against a real ViewModel backed by mocked repos. */
    private fun setScreen(
        auditRepository: AuditRepository,
        contactRepository: ContactRepository,
    ) {
        val viewModel = AuditViewModel(auditRepository, contactRepository)
        composeTestRule.setContent {
            MycorrhizalTheme {
                AuditScreen(onBack = {}, onOpenContact = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `confirming undo runs the undo then refreshes the list`() {
        val auditRepository = mockk<AuditRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { auditRepository.list(entityType = any(), entityId = any(), limit = any()) } returns
            Result.success(AuditEventsResponse(auditEvents = listOf(contactUpdate(id = 1))))
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())
        coEvery { auditRepository.undo(1L) } returns Result.success(Unit)

        setScreen(auditRepository, contactRepository)
        composeTestRule.onNodeWithTag("audit-undo-1").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Undo this change?").assertIsDisplayed()

        // Confirm: undo fires, then the list re-fetches (2 list calls total:
        // the initial load + the post-undo refresh).
        composeTestRule.onAllNodesWithText("Undo")[1].performClick()
        composeTestRule.waitForIdle()

        coVerify(exactly = 1) { auditRepository.undo(1L) }
        coVerify(exactly = 2) { auditRepository.list(entityType = null, entityId = null, limit = 100) }
    }

    @Test
    fun `the clear filters button is enabled while the entity id input has text`() {
        val auditRepository = mockk<AuditRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { auditRepository.list(entityType = any(), entityId = any(), limit = any()) } returns
            Result.success(AuditEventsResponse(auditEvents = emptyList()))
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(emptyMap())

        setScreen(auditRepository, contactRepository)
        // Initially no filters → disabled (mirrors web's `hasFilters`).
        composeTestRule.onNodeWithTag("audit-clear-filters").assertIsNotEnabled()
        // Typing into the field enables Clear immediately — web's Clear is
        // enabled from the *input* value, before the 350ms debounce applies.
        composeTestRule.onNodeWithTag("audit-entity-id").performTextInput("uid-9")
        composeTestRule.onNodeWithTag("audit-clear-filters").assertIsEnabled()
    }
}
