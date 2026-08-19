package com.mycorrhizal.crm.feature.cadence

import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotDisplayed
import androidx.compose.ui.test.assertIsToggleable
import androidx.compose.ui.test.hasSetTextAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.domain.repository.CadencePolicyRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.SessionState
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.flow.flowOf
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CadenceScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val uid = "11111111-1111-1111-1111-111111111111"

    private fun policy(
        interval: Int = 30,
        qualifyingTypes: List<String> = emptyList(),
        health: CadenceHealth? = null,
    ) = CadencePolicy(
        id = "p1",
        entityId = uid,
        targetIntervalDays = interval,
        qualifyingTypes = qualifyingTypes,
        health = health,
    )

    private fun setPanelContent(policy: CadencePolicy, dateFormat: String = "eu") {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadencePanelContent(policy = policy, dateFormat = dateFormat, onEdit = {}, onDelete = {})
            }
        }
    }

    /** Renders the full [CadenceScreen] against a fake ViewModel (mocked repos). */
    private fun setScreen(policy: CadencePolicy? = null) {
        val policyRepository = mockk<CadencePolicyRepository>()
        val contactRepository = mockk<ContactRepository>()
        val authRepository = mockk<AuthRepository>()
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
        coEvery { policyRepository.listForContact(uid) } returns Result.success(
            if (policy == null) emptyList() else listOf(policy),
        )
        coEvery { authRepository.observeSession() } returns flowOf(SessionState())
        val viewModel = CadenceViewModel(
            policyRepository,
            contactRepository,
            authRepository,
            SavedStateHandle(mapOf("contactId" to 5)),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
    }

    @Test
    fun `the delete confirmation dialog is not shown until Delete is tapped`() {
        // Regression pin: this dialog used to be gated with `pendingDelete?.let { … }`
        // on a Boolean, which renders the dialog unconditionally — the screen was
        // permanently covered by the delete prompt.
        setScreen(policy())
        composeTestRule.onNodeWithText("Delete cadence").assertIsNotDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()
        composeTestRule.onNodeWithText("Delete cadence").assertIsDisplayed()
    }

    @Test
    fun `confirming the delete confirmation removes the policy and shows the empty state`() {
        val policyRepository = mockk<CadencePolicyRepository>()
        val contactRepository = mockk<ContactRepository>()
        val authRepository = mockk<AuthRepository>()
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
        coEvery { policyRepository.listForContact(uid) } returns Result.success(listOf(policy()))
        coEvery { policyRepository.delete("p1") } returns Result.success(Unit)
        coEvery { authRepository.observeSession() } returns flowOf(SessionState())
        val viewModel = CadenceViewModel(
            policyRepository,
            contactRepository,
            authRepository,
            SavedStateHandle(mapOf("contactId" to 5)),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Delete").performClick()
        // Two "Delete" nodes now exist (the panel button and the dialog's confirm) —
        // the confirm is the last one.
        composeTestRule.onAllNodesWithText("Delete")[1].performClick()
        composeTestRule.waitForIdle()

        coVerify { policyRepository.delete("p1") }
        composeTestRule.onNodeWithText("No cadence set for this contact.").assertIsDisplayed()
    }

    @Test
    fun `the add fab shows only in the empty state`() {
        setScreen(policy = null)
        composeTestRule.onNodeWithContentDescription("Add Cadence").assertIsDisplayed()
    }

    @Test
    fun `the add fab is hidden once a policy exists`() {
        setScreen(policy())
        composeTestRule.onNodeWithContentDescription("Add Cadence").assertIsNotDisplayed()
    }

    @Test
    fun `a failed create returns to the empty state with the error toasted`() {
        val policyRepository = mockk<CadencePolicyRepository>()
        val contactRepository = mockk<ContactRepository>()
        val authRepository = mockk<AuthRepository>()
        coEvery { contactRepository.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = uid, name = Name(full = "Dana White"))),
        )
        coEvery { policyRepository.listForContact(uid) } returns Result.success(emptyList())
        coEvery { policyRepository.create(any()) } returns Result.failure(
            com.mycorrhizal.crm.network.ApiError.Client(409, "A cadence already exists"),
        )
        coEvery { authRepository.observeSession() } returns flowOf(SessionState())
        val viewModel = CadenceViewModel(
            policyRepository,
            contactRepository,
            authRepository,
            SavedStateHandle(mapOf("contactId" to 5)),
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceScreen(onBack = {}, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithContentDescription("Add Cadence").performClick()
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        // The empty state is restored so the user can retry — a transient write
        // failure must not leave the no-policy body replaced by dead error text.
        composeTestRule.onNodeWithText("No cadence set for this contact.").assertIsDisplayed()
        composeTestRule.onNodeWithText("A cadence already exists").assertIsDisplayed()
    }

    @Test
    fun `panel shows the interval chip`() {
        setPanelContent(policy(interval = 45))
        composeTestRule.onNodeWithText("every 45 days").assertIsDisplayed()
    }

    @Test
    fun `panel shows the server's overdue verdict even when the dates look fine`() {
        // The M12 "health is read, never recomputed" pin: next_due is comfortably
        // in the future, which local dates would call "on track", but the server's
        // overdue_by is 3 — the UI must show the server's 3-days-overdue verdict.
        setPanelContent(
            policy(
                health = CadenceHealth(
                    hasQualifyingInteraction = true,
                    lastInteraction = "2026-08-01T10:00:00Z",
                    nextDue = "2026-12-01T00:00:00Z",
                    overdueBy = 3,
                ),
            ),
        )
        composeTestRule.onNodeWithText("3 days overdue").assertIsDisplayed()
    }

    @Test
    fun `panel shows on track when not overdue but has interactions`() {
        setPanelContent(
            policy(
                health = CadenceHealth(
                    hasQualifyingInteraction = true,
                    lastInteraction = "2026-07-01T10:00:00Z",
                    nextDue = "2026-07-31T00:00:00Z",
                    overdueBy = 0,
                ),
            ),
        )
        composeTestRule.onNodeWithText("On track").assertIsDisplayed()
    }

    @Test
    fun `panel shows the no-interactions-yet hint before the first qualifying interaction`() {
        setPanelContent(
            policy(health = CadenceHealth(hasQualifyingInteraction = false, overdueBy = 0)),
        )
        composeTestRule.onNodeWithText("No qualifying interactions yet — cadence starts once you record one.")
            .assertIsDisplayed()
    }

    @Test
    fun `panel shows the qualifying type chips`() {
        setPanelContent(policy(qualifyingTypes = listOf("call", "visit")))
        composeTestRule.onNodeWithText("Call").assertIsDisplayed()
        composeTestRule.onNodeWithText("Visit").assertIsDisplayed()
    }

    @Test
    fun `panel renders next-due in UTC regardless of the device zone`() {
        // 2026-09-10T01:00:00Z is 2026-09-09 18:00 in Los Angeles — a
        // device-zone renderer would show "Next due: 9 September 2026". The
        // dates are server-computed calendar values; render them in UTC the
        // way web and M11's prep view do (a real bug this pins).
        val original = java.util.TimeZone.getDefault()
        java.util.TimeZone.setDefault(java.util.TimeZone.getTimeZone("America/Los_Angeles"))
        try {
            setPanelContent(
                policy(
                    health = CadenceHealth(
                        hasQualifyingInteraction = true,
                        lastInteraction = "2026-07-01T10:00:00Z",
                        nextDue = "2026-09-10T01:00:00Z",
                        overdueBy = 0,
                    ),
                ),
            )
            composeTestRule.onNodeWithText("Next due: 10 September 2026").assertIsDisplayed()
        } finally {
            java.util.TimeZone.setDefault(original)
        }
    }

    @Test
    fun `panel renders next-due and last-interaction in the user's date format`() {
        setPanelContent(
            policy(
                health = CadenceHealth(
                    hasQualifyingInteraction = true,
                    lastInteraction = "2026-07-01T10:00:00Z",
                    nextDue = "2026-07-31T00:00:00Z",
                    overdueBy = 0,
                ),
            ),
            dateFormat = "iso",
        )
        composeTestRule.onNodeWithText("Next due: 2026-07-31").assertIsDisplayed()
        composeTestRule.onNodeWithText("Last interaction: 2026-07-01").assertIsDisplayed()
    }

    @Test
    fun `an unparseable health date is not rendered as an empty caption`() {
        setPanelContent(
            policy(
                health = CadenceHealth(
                    hasQualifyingInteraction = true,
                    lastInteraction = "2026-07-01T10:00:00Z",
                    nextDue = "not-a-date",
                    overdueBy = 0,
                ),
            ),
        )
        // The valid caption still renders; the unparseable one is dropped rather
        // than showing "Next due: " with an empty value.
        composeTestRule.onNodeWithText("Last interaction: 1 July 2026").assertIsDisplayed()
        composeTestRule.onNodeWithText("Next due: ").assertIsNotDisplayed()
    }

    @Test
    fun `create dialog defaults to 30 days and an empty selection which is preserved on save`() {
        var savedInterval: Int? = null
        var savedTypes: List<String>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(
                    policy = null,
                    onConfirm = { interval, types ->
                        savedInterval = interval
                        savedTypes = types
                    },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Create").performClick()
        assertEquals(30, savedInterval)
        assertEquals(emptyList<String>(), savedTypes)
    }

    @Test
    fun `create dialog sends only the checked qualifying types`() {
        var savedTypes: List<String>? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(
                    policy = null,
                    onConfirm = { _, types -> savedTypes = types },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("Call").performClick()
        // Later checkboxes sit below the dialog's fold in Robolectric's small
        // window — scroll them into view so the click actually lands.
        composeTestRule.onNodeWithText("Gift").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Create").performClick()
        assertEquals(listOf("call", "gift"), savedTypes)
    }

    @Test
    fun `dialog rejects a non-positive interval without confirming`() {
        var confirmed = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(
                    policy = null,
                    onConfirm = { _, _ -> confirmed = true },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNode(hasSetTextAction()).performTextReplacement("0")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.onNodeWithText("Enter a positive number of days.").assertIsDisplayed()
        assertFalse(confirmed)
    }

    @Test
    fun `edit dialog pre-fills the policy's interval and types`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(
                    policy = policy(interval = 90, qualifyingTypes = listOf("visit")),
                    onConfirm = { _, _ -> },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNodeWithText("90").assertIsDisplayed()
        composeTestRule.onNodeWithText("Visit").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `edit dialog save sends the edited interval`() {
        var savedInterval: Int? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(
                    policy = policy(interval = 90, qualifyingTypes = listOf("visit")),
                    onConfirm = { interval, _ -> savedInterval = interval },
                    onDismiss = {},
                )
            }
        }
        composeTestRule.onNode(hasSetTextAction()).performTextReplacement("120")
        composeTestRule.onNodeWithText("Save").performClick()
        assertEquals(120, savedInterval)
    }

    @Test
    fun `qualifying type checkbox is named by its label`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(policy = null, onConfirm = { _, _ -> }, onDismiss = {})
            }
        }
        // #199: Modifier.toggleable on the row merges the type label into the
        // checkbox's own accessible name -- previously an unnamed Checkbox
        // sat next to a label Text with its own separate, unassociated
        // .clickable duplicating the same toggle.
        composeTestRule.onNodeWithText("Call").assertIsToggleable()
    }

    @Test
    fun `qualifying types label is a heading`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                CadenceDialog(policy = null, onConfirm = { _, _ -> }, onDismiss = {})
            }
        }
        // #208: section labels carried no heading semantics, so TalkBack's
        // heading navigation found nothing inside this dialog.
        composeTestRule.onNodeWithText("Qualifying interactions")
            .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Heading))
    }
}
