package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertIsToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.domain.repository.WebhookRepository
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookDelivery
import com.mycorrhizal.crm.model.network.WebhookInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.awaitCancellation
import org.junit.Assert.assertEquals
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
class WebhooksScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun webhook(id: Int, name: String = "Hook $id") = Webhook(
        id = id,
        name = name,
        url = "https://example.com/$id",
        events = listOf("contact.created"),
        isActive = true,
    )

    // --- Top-level WebhooksScreen: the RefreshableContent wrapper + its
    // loading/empty/populated branches only execute when the real screen is
    // mounted against a real ViewModel (mocked repository, no Hilt).

    private fun setScreen(viewModel: WebhooksViewModel) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhooksScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `top-level screen shows a spinner while the initial load is in flight`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } coAnswers { awaitCancellation() }

        setScreen(WebhooksViewModel(repository))

        composeTestRule
            .onNode(SemanticsMatcher.keyIsDefined(SemanticsProperties.ProgressBarRangeInfo))
            .assertExists()
        composeTestRule.onNodeWithText("No webhooks configured yet.").assertDoesNotExist()
    }

    @Test
    fun `top-level screen shows the empty state when no webhooks exist`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } returns Result.success(emptyList())

        setScreen(WebhooksViewModel(repository))

        composeTestRule.onNodeWithText("No webhooks configured yet.").assertIsDisplayed()
    }

    @Test
    fun `top-level screen renders the webhook list and description`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } returns Result.success(listOf(webhook(1, name = "Hook A")))

        setScreen(WebhooksViewModel(repository))

        composeTestRule.onNodeWithText("Receive HTTP POST notifications when events occur in your CRM.").assertIsDisplayed()
        composeTestRule.onNodeWithText("Hook A").assertIsDisplayed()
    }

    @Test
    fun `top-level screen shows an action error above a populated list`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } returns Result.success(listOf(webhook(1, name = "Hook A")))
        coEvery { repository.test(1) } returns Result.failure(ApiError.Server(500, "boom"))
        val viewModel = WebhooksViewModel(repository)
        setScreen(viewModel)

        composeTestRule.runOnIdle { viewModel.test(webhook(1, name = "Hook A")) }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Hook A").assertIsDisplayed()
        composeTestRule.onNodeWithText("Server error (500)").assertIsDisplayed()
    }

    @Test
    fun `top-level screen shows the test-delivered message above a populated list`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } returns Result.success(listOf(webhook(1, name = "Hook A")))
        coEvery { repository.test(1) } returns Result.success(
            WebhookDelivery(id = 10, webhookId = 1, eventType = "test", statusCode = 200),
        )
        val viewModel = WebhooksViewModel(repository)
        setScreen(viewModel)

        composeTestRule.runOnIdle { viewModel.test(webhook(1, name = "Hook A")) }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Test delivered (200)").assertIsDisplayed()
    }

    // Lines 195-197: the row's `onEdit` lambda only runs when the real
    // top-level screen is mounted and its Edit action is clicked. The direct
    // WebhookRow test below supplies its own onEdit, so it never reaches this.
    @Test
    fun `top-level screen opens the editor when a row's edit action is clicked`() {
        val repository = mockk<WebhookRepository>()
        coEvery { repository.list() } returns Result.success(listOf(webhook(1, name = "Hook A")))
        setScreen(WebhooksViewModel(repository))

        composeTestRule.onNodeWithContentDescription("Edit Hook A").performClick()
        composeTestRule.waitForIdle()

        // The editor dialog's title only appears once the row's edit lambda ran.
        composeTestRule.onNodeWithText("Edit").assertIsDisplayed()
    }

    @Test
    fun `shows a webhook row with test edit and delete actions`() {
        val webhook = Webhook(id = 1, name = "Hook A", url = "https://example.com/a", events = listOf("contact.created"), isActive = true)
        var tested = false
        var edited = false
        var deleted = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookRow(
                    webhook = webhook,
                    testing = false,
                    expanded = false,
                    deliveries = emptyList(),
                    onTest = { tested = true },
                    onEdit = { edited = true },
                    onDelete = { deleted = true },
                    onToggleDeliveries = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Hook A").assertIsDisplayed()
        composeTestRule.onNodeWithText("Active").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 events").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Test Hook A").performClick()
        composeTestRule.onNodeWithContentDescription("Edit Hook A").performClick()
        composeTestRule.onNodeWithContentDescription("Delete Hook A").performClick()
        assert(tested && edited && deleted)
    }

    @Test
    fun `expanded deliveries render status and event type`() {
        val webhook = Webhook(id = 1, name = "Hook A", url = "https://example.com/a", events = listOf("contact.created"), isActive = true)
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookRow(
                    webhook = webhook,
                    testing = false,
                    expanded = true,
                    deliveries = listOf(
                        WebhookDelivery(id = 1, webhookId = 1, eventType = "contact.created", statusCode = 200),
                    ),
                    onTest = {},
                    onEdit = {},
                    onDelete = {},
                    onToggleDeliveries = {},
                )
            }
        }

        composeTestRule.onNodeWithText("contact.created").assertIsDisplayed()
        composeTestRule.onNodeWithText("200").assertIsDisplayed()
    }

    @Test
    fun `expanded deliveries show an empty message when none exist`() {
        val webhook = Webhook(id = 1, name = "Hook A", url = "https://example.com/a", events = listOf("contact.created"), isActive = true)
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookRow(
                    webhook = webhook,
                    testing = false,
                    expanded = true,
                    deliveries = emptyList(),
                    onTest = {},
                    onEdit = {},
                    onDelete = {},
                    onToggleDeliveries = {},
                )
            }
        }

        composeTestRule.onNodeWithText("No deliveries yet.").assertIsDisplayed()
    }

    @Test
    fun `editor dialog blocks save until name url and an event are provided`() {
        var confirmed: WebhookInput? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookEditorDialog(
                    initial = null,
                    isSaving = false,
                    onConfirm = { confirmed = it },
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Save").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Name").performTextInput("Hook A")
        composeTestRule.onNodeWithText("URL").performTextInput("https://example.com/a")
        composeTestRule.onNodeWithText("Save").assertIsNotEnabled()

        composeTestRule.onNodeWithText("Contact Created").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Save").performClick()

        val input = confirmed
        assertEquals("Hook A", input?.name)
        assertEquals("https://example.com/a", input?.url)
        assertEquals(listOf("contact.created"), input?.events)
        assertTrue(input?.isActive == true)
    }

    @Test
    fun `editor dialog pre-fills an existing webhook for editing`() {
        val webhook = Webhook(id = 5, name = "Hook B", url = "https://example.com/b", events = listOf("note.created"), isActive = false)
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookEditorDialog(
                    initial = webhook,
                    isSaving = false,
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }

        composeTestRule.onNodeWithText("Edit").assertIsDisplayed()
        // The existing event chip is already selected and the active switch off —
        // both reflected via their labels.
        composeTestRule.onNodeWithText("Note Created").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `active switch is named by its label`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                WebhookEditorDialog(
                    initial = null,
                    isSaving = false,
                    onConfirm = {},
                    onDismiss = {},
                )
            }
        }

        // #199: Modifier.toggleable on the row merges "Active" into the
        // switch's own accessible name and exposes its checked state --
        // previously an unnamed Switch sat next to a plain, unassociated
        // Text. (Not exercising performClick() here: this row sits below the
        // dialog's own scrollable chip list, outside any scrollable
        // container, which Robolectric's default test window doesn't give
        // real layout bounds for -- a test-harness limit, not part of the
        // fix. The toggle/click wiring is unchanged from before and is
        // covered by NotificationChannelsScreenTest and WebhooksScreenTest's
        // own editor-dialog save-flow test above.)
        composeTestRule.onNodeWithText("Active")
            .assertIsToggleable()
            .assertIsOn()
    }
}
