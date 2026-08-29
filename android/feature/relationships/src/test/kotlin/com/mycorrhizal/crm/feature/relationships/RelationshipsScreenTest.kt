package com.mycorrhizal.crm.feature.relationships

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.lifecycle.SavedStateHandle
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.RelationshipEdgeRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeStatuses
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.testing.a11y.assertAccessibleSemantics
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issue #682: the relationships screen — confirmed + suggested edge rows, the
 * accept/delete/reject flows, and the create dialog — had no test coverage
 * (the module also lacked the `:core:testing` a11y helpers). Mounts the real
 * screen against a [RelationshipsViewModel] backed by mocked repositories.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class RelationshipsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun setScreen(
        edges: List<RelationshipEdge> = emptyList(),
        resolve: Map<String, ContactSummary> = emptyMap(),
        onNavigateToContact: (Int) -> Unit = {},
    ) {
        val edgeRepository = mockk<RelationshipEdgeRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = "u-viewed")),
        )
        coEvery { edgeRepository.listForContact("u-viewed", null, null) } returns Result.success(edges)
        if (resolve.isNotEmpty()) {
            coEvery { contactRepository.resolveByUid(any()) } returns Result.success(resolve)
        }
        val vm = RelationshipsViewModel(
            edgeRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 5)),
        )
        composeTestRule.setContent {
            MycorrhizalTheme(darkTheme = false) {
                RelationshipsScreen(onBack = {}, onNavigateToContact = onNavigateToContact, viewModel = vm)
            }
        }
    }

    private fun confirmedEdge(id: String, source: String, target: String, type: String) = RelationshipEdge(
        id = id, sourceId = source, targetId = target, type = type, status = RelationshipEdgeStatuses.CONFIRMED,
    )

    @Test
    fun `renders confirmed edges with the other party's resolved name`() {
        setScreen(
            edges = listOf(
                confirmedEdge("e1", "u-other", "u-viewed", RelationshipEdgeTypes.FRIEND_OF),
            ),
            resolve = mapOf("u-other" to ContactSummary(id = 9, uid = "u-other", firstname = "Alice")),
        )

        composeTestRule.onNodeWithText("Alice").assertIsDisplayed()
        composeTestRule.onNodeWithText("friend of").assertIsDisplayed()
    }

    @Test
    fun `renders the suggested section with accept and reject actions`() {
        setScreen(
            edges = listOf(
                confirmedEdge("e1", "u-other", "u-viewed", RelationshipEdgeTypes.FRIEND_OF),
                RelationshipEdge(
                    id = "e2",
                    sourceId = "u-viewed",
                    targetId = "u-bob",
                    type = RelationshipEdgeTypes.SPOUSE_OF,
                    status = RelationshipEdgeStatuses.SUGGESTED,
                ),
            ),
            resolve = mapOf(
                "u-other" to ContactSummary(id = 9, uid = "u-other", firstname = "Alice"),
                "u-bob" to ContactSummary(id = 10, uid = "u-bob", firstname = "Bob"),
            ),
        )

        // The suggested section header and the suggested-row status label both
        // read "Suggested" — two nodes; the point is both appear.
        composeTestRule.onAllNodesWithText(str(R.string.relationships_suggested_section))
            .assertCountEquals(2)
        composeTestRule.onNodeWithText("Bob").assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.relationships_accept))
            .assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Reject Bob").assertIsDisplayed()
    }

    @Test
    fun `an empty relationship list shows the empty state`() {
        setScreen()

        composeTestRule.onNodeWithText(str(R.string.relationships_empty))
            .assertIsDisplayed()
    }

    @Test
    fun `relationships screen has no accessibility violations`() {
        setScreen(
            edges = listOf(confirmedEdge("e1", "u-other", "u-viewed", RelationshipEdgeTypes.FRIEND_OF)),
            resolve = mapOf("u-other" to ContactSummary(id = 9, uid = "u-other", firstname = "Alice")),
        )

        composeTestRule.assertAccessibleSemantics()
    }

    @Test
    fun `deleting a confirmed edge confirms then calls the repository`() {
        val edgeRepository = mockk<RelationshipEdgeRepository>()
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = "u-viewed")),
        )
        coEvery { edgeRepository.listForContact("u-viewed", null, null) } returns Result.success(
            listOf(confirmedEdge("e1", "u-other", "u-viewed", RelationshipEdgeTypes.FRIEND_OF)),
        )
        coEvery { contactRepository.resolveByUid(any()) } returns Result.success(
            mapOf("u-other" to ContactSummary(id = 9, uid = "u-other", firstname = "Alice")),
        )
        coEvery { edgeRepository.delete("e1") } returns Result.success(Unit)
        val vm = RelationshipsViewModel(
            edgeRepository,
            contactRepository,
            SavedStateHandle(mapOf("contactId" to 5)),
        )
        composeTestRule.setContent {
            MycorrhizalTheme { RelationshipsScreen(onBack = {}, onNavigateToContact = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithContentDescription("Delete Alice").performClick()
        composeTestRule.onNodeWithText(str(R.string.relationships_delete_title))
            .assertIsDisplayed()
        composeTestRule.onNodeWithText(str(R.string.action_delete)).performClick()

        coVerify(exactly = 1) { edgeRepository.delete("e1") }
    }

    @Test
    fun `the fab opens the create relationship dialog`() {
        setScreen()

        composeTestRule.onNodeWithContentDescription(
            str(R.string.relationships_new),
        ).performClick()

        composeTestRule.onNodeWithText(str(R.string.relationships_new))
            .assertIsDisplayed()
    }
}
