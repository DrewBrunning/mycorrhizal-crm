package com.mycorrhizal.crm.feature.contacts.entities

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.domain.repository.EntityRepository
import com.mycorrhizal.crm.model.network.Entity
import com.mycorrhizal.crm.model.network.EntityKind
import com.mycorrhizal.crm.model.network.EntityListResponse
import com.mycorrhizal.crm.model.network.EntityMergeResponse
import com.mycorrhizal.crm.model.network.EntitySuggestionEntity
import com.mycorrhizal.crm.model.network.EntitySuggestionPair
import com.mycorrhizal.crm.model.network.EntitySuggestionsResponse
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * The stateful [EntitiesScreen] wiring for the suggestion Merge action (#2520): the merge result
 * is announced once from the screen, and a pair that cannot be loaded shows its error on the
 * card. Both live outside the stateless `EntitiesContent` the other screen tests drive.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class EntitiesScreenMergeFlowTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<EntityRepository>()

    private val pair = EntitySuggestionPair(
        reason = "normalized_name",
        a = EntitySuggestionEntity(id = "id-Acme", kind = EntityKind.ORGANISATION, name = "Acme"),
        b = EntitySuggestionEntity(id = "id-ACME", kind = EntityKind.ORGANISATION, name = "ACME")
    )

    private fun full(name: String) =
        Entity(id = "id-$name", name = name, kind = EntityKind.ORGANISATION)

    private fun stubList() {
        coEvery { repository.list(any(), any(), any(), any()) } returns Result.success(
            EntityListResponse(entities = listOf(full("Acme"), full("ACME")), nextCursor = null)
        )
        coEvery { repository.suggestions(any()) } returns Result.success(
            EntitySuggestionsResponse(mode = "pairs", pairs = listOf(pair))
        )
    }

    private fun show() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntitiesScreen(
                    onBack = {},
                    onOpenEntity = {},
                    onCreateFromContacts = {},
                    viewModel = EntitiesViewModel(repository)
                )
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `confirming a merge from the screen announces it once`() {
        stubList()
        coEvery { repository.get("id-Acme") } returns Result.success(full("Acme"))
        coEvery { repository.get("id-ACME") } returns Result.success(full("ACME"))
        coEvery { repository.merge("id-ACME", any()) } returns Result.success(
            EntityMergeResponse(survivor = full("Acme"), loserId = "id-ACME", linksRepointed = 2)
        )
        show()

        composeTestRule.onNodeWithText("Find duplicates").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("suggestion-merge-${pair.a.id}").performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("entity-merge-confirm").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Merged ACME into Acme.").assertIsDisplayed()
        composeTestRule.onAllNodesWithTag("entity-merge-dialog").assertCountEquals(0)
    }

    @Test
    fun `a pair that cannot be loaded shows its error on the card and no dialog`() {
        stubList()
        coEvery { repository.get(any()) } returns
            Result.failure(java.io.IOException("offline"))
        show()

        composeTestRule.onNodeWithText("Find duplicates").performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("suggestion-merge-${pair.a.id}").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithTag("suggestion-merge-error").performScrollTo().assertIsDisplayed()
        composeTestRule.onAllNodesWithTag("entity-merge-dialog").assertCountEquals(0)
    }

    @Test
    fun `a merge-load error with no pair open is shown by the card`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                EntitiesContent(
                    state = EntitiesUiState(
                        entities = listOf(full("Acme")),
                        suggestions = listOf(pair),
                        mergeError = ApiError.Network(java.io.IOException("offline"))
                    ),
                    onSelectKind = {},
                    onQueryChange = {},
                    onOpenEntity = {},
                    onLoadMore = {},
                    onRetry = {},
                    onAdd = {},
                    onFindDuplicates = {},
                    onDismissSuggestion = {},
                    onMergeSuggestion = {},
                    onConfirmMerge = { _, _ -> },
                    onDismissMerge = {}
                )
            }
        }
        composeTestRule.onNodeWithTag("suggestion-merge-error").performScrollTo().assertIsDisplayed()
    }
}
