package com.mycorrhizal.crm.feature.occasions

import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.util.Locale
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val UID = "11111111-1111-4111-8111-111111111111"

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactOccasionsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val repository = mockk<OccasionObligationRepository>()
    private val contacts = mockk<ContactRepository>()
    private val originalLocale = Locale.getDefault()

    @Before
    fun pinLocale() {
        Locale.setDefault(Locale.UK)
    }

    @After
    fun restoreLocale() {
        Locale.setDefault(originalLocale)
    }

    private fun ob(
        id: String = "o1",
        label: String = "Christmas card",
        kind: String = "card",
        active: Boolean = true,
        sensitivity: String = "normal",
        notes: String? = null,
        leadTimeDays: Int = 14
    ) = OccasionObligation(
        id = id,
        entityId = UID,
        kind = kind,
        label = label,
        anchorMonth = 12,
        anchorDay = 25,
        leadTimeDays = leadTimeDays,
        active = active,
        sensitivity = sensitivity,
        notes = notes
    )

    private fun render(items: List<OccasionObligation>): ContactOccasionsViewModel {
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana")))
        )
        coEvery { repository.listForContact(UID) } returns Result.success(items)
        val viewModel = ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to 5))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactOccasionsScreen(onBack = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()
        return viewModel
    }

    @Test
    fun `a row shows the kind, the localized anchor date, lead time and label`() {
        render(listOf(ob(notes = "Send to the new address")))
        composeTestRule.onNodeWithText("Christmas card").assertIsDisplayed()
        composeTestRule.onNodeWithText("Card").assertIsDisplayed()
        composeTestRule.onNodeWithText("December 25").assertIsDisplayed()
        composeTestRule.onNodeWithText("14 days lead time").assertIsDisplayed()
        composeTestRule.onNodeWithText("Send to the new address").assertIsDisplayed()
    }

    @Test
    fun `an inactive private row is badged and a zero lead time shows nothing`() {
        render(listOf(ob(active = false, sensitivity = "private", leadTimeDays = 0)))
        composeTestRule.onNodeWithText("Inactive").assertIsDisplayed()
        composeTestRule.onNodeWithText("Private").assertIsDisplayed()
        composeTestRule.onNodeWithText("0 days lead time").assertDoesNotExist()
    }

    @Test
    fun `an unknown kind renders its raw label and a singular lead time is not pluralised`() {
        render(listOf(ob(kind = "bbq", leadTimeDays = 1)))
        composeTestRule.onNodeWithText("bbq").assertIsDisplayed()
        composeTestRule.onNodeWithText("1 day lead time").assertIsDisplayed()
    }

    @Test
    fun `no obligations shows the empty state with the create action`() {
        render(emptyList())
        composeTestRule.onNodeWithText("No standing occasions yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add one for a recurring card, gift, or invite.")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Add occasion").performClick()
        composeTestRule.onNodeWithText("Add an occasion").assertIsDisplayed()
    }

    @Test
    fun `a failed load shows the localized error and no rows`() {
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana")))
        )
        coEvery { repository.listForContact(UID) } returns
            Result.failure(ApiError.Client(500, "boom"))
        val viewModel = ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to 5))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactOccasionsScreen(onBack = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("boom").assertIsDisplayed()
    }

    @Test
    fun `creating an occasion submits the typed label and returns to the list`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.success(ob("n", "Birthday gift"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Birthday gift")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        coVerify {
            repository.create(
                match {
                    it.entityId == UID && it.label == "Birthday gift" && it.kind == "card" &&
                        it.active && it.leadTimeDays == 0 && it.anchorMonth == null
                }
            )
        }
        // The form closed: the list is showing again.
        composeTestRule.onNodeWithText("Add an occasion").assertDoesNotExist()
    }

    @Test
    fun `the Active row exposes its on or off state as a switch and toggles on tap`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        val active = composeTestRule.onNodeWithText("Active").performScrollTo()
        active.assert(SemanticsMatcher.expectValue(SemanticsProperties.Role, Role.Switch))
        active.assertIsOn()

        active.performClick()
        active.assertIsOff()
        active.performClick()
        active.assertIsOn()
    }

    @Test
    fun `save stays disabled until a label is typed`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("x")
        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
    }

    @Test
    fun `a rejected save keeps the form open and shows the error`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(422, "nope"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("x")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("nope").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add an occasion").assertIsDisplayed()
    }

    @Test
    fun `the edit action opens the form prefilled and saving replaces the row`() {
        render(listOf(ob()))
        coEvery { repository.update("o1", any()) } returns Result.success(ob())
        composeTestRule.onNodeWithContentDescription("Edit").performClick()
        composeTestRule.onNodeWithText("Edit occasion").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "occasion-ob-label"
        ).performTextReplacement("Christmas card 2")
        composeTestRule.onNodeWithText("Save").performClick()
        composeTestRule.waitForIdle()

        coVerify {
            repository.update(
                "o1",
                match { it.label == "Christmas card 2" && it.anchorMonth == 12 && it.anchorDay == 25 }
            )
        }
    }

    @Test
    fun `delete asks for confirmation then deletes`() {
        render(listOf(ob()))
        coEvery { repository.delete("o1") } returns Result.success(Unit)
        composeTestRule.onNodeWithContentDescription("Delete Christmas card").performClick()
        composeTestRule.onNodeWithText("Delete occasion").assertIsDisplayed()
        coVerify(exactly = 0) { repository.delete(any()) }
        composeTestRule.onNodeWithText("Delete").performClick()
        composeTestRule.waitForIdle()
        coVerify(exactly = 1) { repository.delete("o1") }
    }

    @Test
    fun `cancelling the delete confirmation deletes nothing`() {
        render(listOf(ob()))
        composeTestRule.onNodeWithContentDescription("Delete Christmas card").performClick()
        composeTestRule.onNodeWithText("Cancel").performClick()
        coVerify(exactly = 0) { repository.delete(any()) }
    }
}
