package com.mycorrhizal.crm.feature.circles

import android.content.Context
import androidx.annotation.StringRes
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextReplacement
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.R
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.CompletableDeferred
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// The screen wires to hiltViewModel, which a plain Robolectric test cannot
// construct — so the pre-existing tests exercise the stateful pieces through
// the stateless ClassifyContent/DoneContent composables. The pull-to-refresh
// wrapper and the loading/empty/error/done branches are covered below by
// mounting the real top-level screen against a TriageViewModel backed by
// mocked repositories.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TriageScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun str(@StringRes res: Int, vararg args: Any): String =
        ApplicationProvider.getApplicationContext<Context>().getString(res, *args)

    private fun item(name: String, classification: TriageClassification = TriageClassification.CIRCLE, count: Int = 1) =
        TriageItem(original = name, name = name, classification = classification, contactCount = count)

    private fun screen(viewModel: TriageViewModel) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                TriageScreen(onBack = {}, viewModel = viewModel)
            }
        }
    }

    @Test
    fun `classify rows render the legacy name, count and classification chips`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = listOf(item("Friends", count = 3))),
                    onSetClassification = { _, _ -> },
                    onSetName = { _, _ -> },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Friends").assertIsDisplayed()
        composeTestRule.onNodeWithText("3 contacts").assertIsDisplayed()
        composeTestRule.onNodeWithText("Circle").assertIsDisplayed()
        composeTestRule.onNodeWithText("Tag").assertIsDisplayed()
        composeTestRule.onNodeWithText("Skip").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 circles, 0 tags, 0 skipped").assertIsDisplayed()
    }

    @Test
    fun `the apply button is disabled when everything is skipped`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = listOf(item("Obsolete", TriageClassification.SKIP))),
                    onSetClassification = { _, _ -> },
                    onSetName = { _, _ -> },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("0 circles, 0 tags, 1 skipped").assertIsDisplayed()
        composeTestRule.onNodeWithText("Apply").assertIsNotEnabled()
    }

    @Test
    fun `renaming a legacy string updates the item name`() {
        var items = listOf(item("Friends"))
        composeTestRule.setContent {
            MycorrhizalTheme {
                ClassifyContent(
                    state = TriageUiState(items = items),
                    onSetClassification = { _, _ -> },
                    onSetName = { index, name ->
                        items = items.mapIndexed { i, it -> if (i == index) it.copy(name = name) else it }
                    },
                    onApply = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Friends").performTextReplacement("Old friends")

        assertEquals("Old friends", items[0].name)
    }

    @Test
    fun `the done state reports how many circles and tags were created`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                DoneContent(
                    state = TriageUiState(done = true, appliedCircles = 1, appliedTags = 2),
                    onBack = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Triage complete").assertIsDisplayed()
        composeTestRule.onNodeWithText("Created 1 circle(s) and 2 tag(s).").assertIsDisplayed()
    }

    @Test
    fun `the top-level screen renders the loading skeleton on the first load`() {
        val contactRepository = mockk<ContactRepository>()
        val gate = CompletableDeferred<Result<List<String>>>()
        coEvery { contactRepository.listLegacyCircles() } coAnswers { gate.await() }

        screen(TriageViewModel(contactRepository, mockk(), mockk()))

        composeTestRule.onNodeWithContentDescription(str(R.string.a11y_state_loading)).assertIsDisplayed()
    }

    @Test
    fun `the top-level screen renders the empty state when there is nothing to clean up`() {
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.listLegacyCircles() } returns Result.success(emptyList())

        screen(TriageViewModel(contactRepository, mockk(), mockk()))

        composeTestRule.onNodeWithText(str(R.string.triage_empty)).assertIsDisplayed()
    }

    @Test
    fun `the top-level screen renders the error text when the load fails`() {
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.listLegacyCircles() } returns
            Result.failure(ApiError.Client(500, "boom"))

        screen(TriageViewModel(contactRepository, mockk(), mockk()))

        assertTrue(composeTestRule.onAllNodesWithText("boom").fetchSemanticsNodes().isNotEmpty())
    }

    @Test
    fun `the top-level screen renders the classification list when legacy strings exist`() {
        val contactRepository = mockk<ContactRepository>()
        coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends"))
        coEvery {
            contactRepository.listContacts(any(), any(), any(), any(), any(), any(), any())
        } returns Result.success(ContactsPage(contacts = emptyList(), nextCursor = null, limit = 500, sync = null))

        screen(TriageViewModel(contactRepository, mockk(), mockk()))

        composeTestRule.onNodeWithText("Friends").assertIsDisplayed()
    }

    @Test
    fun `the top-level screen renders the done state after applying`() {
        val contactRepository = mockk<ContactRepository>()
        val circleRepository = mockk<CircleRepository>()
        val tagRepository = mockk<TagRepository>()
        coEvery { contactRepository.listLegacyCircles() } returns Result.success(listOf("Friends"))
        coEvery {
            contactRepository.listContacts(any(), any(), any(), any(), any(), any(), any())
        } returns Result.success(ContactsPage(contacts = emptyList(), nextCursor = null, limit = 500, sync = null))
        coEvery { circleRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { circleRepository.create("Friends") } returns
            Result.success(Circle(id = "c1", name = "Friends"))

        val viewModel = TriageViewModel(contactRepository, circleRepository, tagRepository)
        screen(viewModel)
        composeTestRule.waitForIdle()

        viewModel.apply()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(str(R.string.triage_done_title)).assertIsDisplayed()
    }
}
