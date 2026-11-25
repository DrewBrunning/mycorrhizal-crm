package com.mycorrhizal.crm.feature.occasions

import androidx.activity.ComponentActivity
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertHeightIsAtLeast
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.assertWidthIsAtLeast
import androidx.compose.ui.test.junit4.v2.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import androidx.compose.ui.unit.dp
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.OccasionObligationRepository
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.OccasionObligation
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.util.Locale
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

private const val SNACKBAR_MS = 11_000L
private const val UID = "11111111-1111-4111-8111-111111111111"

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactOccasionsScreenTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

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
        leadTimeDays: Int = 14,
        anchorMonth: Int? = 12,
        anchorDay: Int? = 25
    ) = OccasionObligation(
        id = id,
        entityId = UID,
        kind = kind,
        label = label,
        anchorMonth = anchorMonth,
        anchorDay = anchorDay,
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

    // #2920 parity with the web's #2914 fix: the language orders and punctuates
    // the anchor date; it used to be the English "<month> <day>" in every
    // language ("diciembre 25" in Spanish).
    @Test
    @Config(qualifiers = "es")
    fun `the anchor date follows the language's day-month order`() {
        render(listOf(ob()))
        composeTestRule.onNodeWithText("25 de diciembre").assertIsDisplayed()
    }

    @Test
    @Config(qualifiers = "fr")
    fun `the anchor date in French puts the day first`() {
        render(listOf(ob()))
        composeTestRule.onNodeWithText("25 décembre").assertIsDisplayed()
    }

    @Test
    fun `a legacy dateless row says it has no date instead of leaving a blank`() {
        render(listOf(ob(anchorMonth = null, anchorDay = null)))
        composeTestRule.onNodeWithText("No date — no reminder").assertIsDisplayed()
        composeTestRule.onNodeWithText("December 25").assertDoesNotExist()
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

    /** Opens the dropdown tagged [tag] and picks [text], scrolling the popup list to it. */
    private fun choose(tag: String, text: String) {
        composeTestRule.onNodeWithTag(tag).performScrollTo().performClick()
        composeTestRule.onNodeWithText(text).performScrollTo().performClick()
    }

    private fun pickMonthAndDay(month: String, day: String) {
        choose("occasion-ob-month", month)
        choose("occasion-ob-day", day)
    }

    private fun openDayList() {
        composeTestRule.onNodeWithTag("occasion-ob-day").performScrollTo().performClick()
    }

    @Test
    fun `creating an occasion submits the typed label and returns to the list`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.success(ob("n", "Birthday gift"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Birthday gift")
        pickMonthAndDay("June", "3")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        coVerify {
            repository.create(
                match {
                    it.entityId == UID && it.label == "Birthday gift" && it.kind == "card" &&
                        it.active && it.leadTimeDays == 0 &&
                        it.anchorMonth == 6 && it.anchorDay == 3
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
    fun `a fresh form shows no validation error and blank-label Create explains itself`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        // Nothing is flagged before the user has interacted or submitted.
        composeTestRule.onNodeWithText("Enter a label for this occasion.").assertDoesNotExist()
        composeTestRule.onNodeWithText("Create").assertIsEnabled()

        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Enter a label for this occasion.").assertIsDisplayed()
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `submitting without a date shows the localized required reason and keeps the input`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Edge")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("An occasion needs a month and a day so it can remind you")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("Edge").assertIsDisplayed()
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `the Day list follows the chosen month`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()

        choose("occasion-ob-month", "February")
        openDayList()
        composeTestRule.onNodeWithText("29").assertExists()
        composeTestRule.onNodeWithText("30").assertDoesNotExist()
        composeTestRule.onNodeWithText("31").assertDoesNotExist()
        composeTestRule.onNodeWithText("29").performScrollTo().performClick()

        choose("occasion-ob-month", "April")
        openDayList()
        composeTestRule.onNodeWithText("30").assertExists()
        composeTestRule.onNodeWithText("31").assertDoesNotExist()
        composeTestRule.onNodeWithText("30").performScrollTo().performClick()

        choose("occasion-ob-month", "December")
        openDayList()
        composeTestRule.onNodeWithText("31").assertExists()
    }

    @Test
    fun `the label error appears on the field once it is left empty, then clears on typing`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performClick()
        composeTestRule.onNodeWithText("Enter a label for this occasion.").assertDoesNotExist()

        // Leaving the field empty is the blur that reveals the error, on the field itself.
        composeTestRule.onNodeWithTag("occasion-ob-lead-time").performScrollTo().performClick()
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Enter a label for this occasion.").assertIsDisplayed()

        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Card")
        composeTestRule.onNodeWithText("Enter a label for this occasion.").assertDoesNotExist()
    }

    @Test
    fun `changing to a shorter month tells the user the day was cleared`() {
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        pickMonthAndDay("January", "31")
        composeTestRule.onNodeWithText("February has at most 29 days", substring = true)
            .assertDoesNotExist()

        choose("occasion-ob-month", "February")
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText(
            "February has at most 29 days, so the day was cleared. Choose a day again."
        ).assertExists()
        composeTestRule.onNodeWithTag("occasion-ob-day").assertTextContains("Not set")
    }

    @Test
    fun `a legacy dateless row can be retired by saving it inactive`() {
        render(listOf(ob(anchorMonth = null, anchorDay = null)))
        coEvery { repository.update(any(), any()) } returns Result.success(ob())
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
        composeTestRule.onNodeWithText("Active").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Save").performClick()
        composeTestRule.waitForIdle()

        coVerify {
            repository.update(
                "o1",
                match { !it.active && it.anchorMonth == null && it.anchorDay == null }
            )
        }
    }

    @Test
    fun `a legacy dateless row that stays active is refused on the date fields`() {
        render(listOf(ob(anchorMonth = null, anchorDay = null)))
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
        composeTestRule.onNodeWithText("Save").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("An occasion needs a month and a day so it can remind you")
            .assertExists()
        coVerify(exactly = 0) { repository.update(any(), any()) }
    }

    @Test
    fun `switching to a shorter month clears a day that no longer exists`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.success(ob("n", "Edge"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Edge")
        pickMonthAndDay("January", "31")

        choose("occasion-ob-month", "February")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        // The 31 was dropped with the month change, so it is "incomplete", never a Feb 31.
        composeTestRule.onNodeWithText("An occasion anchor date requires both a month and a day")
            .assertIsDisplayed()
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `each server anchor-date reason renders localized text and keeps the form input`() {
        val cases = listOf(
            Triple(
                ErrorReasonCode.OCCASION_ANCHOR_DATE_REQUIRED,
                "An occasion needs a month and a day so it can remind you",
                "occasion-ob-month"
            ),
            Triple(
                ErrorReasonCode.OCCASION_ANCHOR_DATE_INCOMPLETE,
                "An occasion anchor date requires both a month and a day",
                "occasion-ob-day"
            ),
            Triple(
                ErrorReasonCode.OCCASION_ANCHOR_DATE_INVALID,
                "That month and day do not exist together, for example February 30",
                "occasion-ob-day"
            )
        )
        render(emptyList())
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Edge")
        pickMonthAndDay("February", "29")
        for ((code, text, fieldTag) in cases) {
            coEvery { repository.create(any()) } returns Result.failure(
                ApiError.Client(400, "RAW SERVER TEXT", reason = ErrorReason(code, emptyMap()))
            )
            composeTestRule.onNodeWithText("Create").performClick()
            composeTestRule.waitForIdle()

            composeTestRule.onNodeWithText(text).assertIsDisplayed()
            // The message is the named field's own error, not a toast.
            composeTestRule.onNodeWithTag(fieldTag)
                .assert(SemanticsMatcher.keyIsDefined(SemanticsProperties.Error))
            composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
            composeTestRule.onNodeWithText("Add an occasion").assertIsDisplayed()
            composeTestRule.onNodeWithText("Edge").assertIsDisplayed()
            composeTestRule.onNodeWithTag("occasion-ob-month").assertTextContains("February")
            composeTestRule.onNodeWithTag("occasion-ob-day").assertTextContains("29")
            // Let the snackbar clear before the next reason.
            composeTestRule.mainClock.advanceTimeBy(SNACKBAR_MS)
        }
    }

    @Test
    fun `a rejected save keeps the form open and shows the error`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(422, "nope"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("x")
        pickMonthAndDay("March", "9")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("nope").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Add an occasion").assertIsDisplayed()
    }

    // #2943: a save failure is inline text in the form, kept until the user edits or retries; it
    // used to be a 4-second snackbar.
    @Test
    fun `a rejected save keeps its error inline after the snackbar time and clears on an edit`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(422, "nope"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("x")
        pickMonthAndDay("March", "9")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.mainClock.advanceTimeBy(10_000)
        composeTestRule.onNodeWithTag(
            "occasion-ob-save-error"
        ).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("nope").assertIsDisplayed()

        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("y")
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithTag("occasion-ob-save-error").assertDoesNotExist()
    }

    @Test
    fun `the row edit and delete actions name the obligation and are 48dp targets`() {
        render(listOf(ob()))

        composeTestRule.onNodeWithContentDescription("Edit Christmas card")
            .assertHeightIsAtLeast(48.dp).assertWidthIsAtLeast(48.dp)
        composeTestRule.onNodeWithContentDescription("Delete Christmas card")
            .assertHeightIsAtLeast(48.dp).assertWidthIsAtLeast(48.dp)
    }

    @Test
    fun `the edit action opens the form prefilled and saving replaces the row`() {
        render(listOf(ob()))
        coEvery { repository.update("o1", any()) } returns Result.success(ob())
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
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

    private fun kindField() = composeTestRule.onNodeWithTag("occasion-ob-kind")

    @Test
    fun `the Kind field shows a known kind as its label and still saves the stored token`() {
        render(listOf(ob(kind = "gift")))
        coEvery { repository.update("o1", any()) } returns Result.success(ob(kind = "gift"))
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
        kindField().assertTextContains("Gift")
        composeTestRule.onNodeWithText("Save").performClick()
        composeTestRule.waitForIdle()

        coVerify { repository.update("o1", match { it.kind == "gift" }) }
    }

    @Test
    fun `picking a suggestion chip stores its token and a typed custom kind is kept verbatim`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.success(ob("n", "x", kind = "invite"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Dinner")
        pickMonthAndDay("March", "9")
        composeTestRule.onNodeWithText("Invite").performClick()
        kindField().assertTextContains("Invite")
        composeTestRule.onNodeWithTag("occasion-ob-kind").performTextReplacement("Flowers")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        coVerify { repository.create(match { it.kind == "Flowers" }) }
    }

    @Test
    fun `typing a known label maps back to its token`() {
        render(emptyList())
        coEvery { repository.create(any()) } returns Result.success(ob("n", "x", kind = "gift"))
        composeTestRule.onNodeWithContentDescription("Add occasion").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextInput("Present")
        pickMonthAndDay("March", "9")
        composeTestRule.onNodeWithTag("occasion-ob-kind").performTextReplacement("Gift")
        composeTestRule.onNodeWithText("Create").performClick()
        composeTestRule.waitForIdle()

        coVerify { repository.create(match { it.kind == "gift" }) }
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

    @Test
    fun `a failed load is worded from its reason code, never the server text`() {
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana")))
        )
        coEvery { repository.listForContact(UID) } returns Result.failure(
            ApiError.Client(
                409,
                "RAW SERVER TEXT",
                reason = ErrorReason(ErrorReasonCode.ENTITY_NOT_FOUND, emptyMap())
            )
        )
        val viewModel = ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to 5))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactOccasionsScreen(onBack = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithText("Organisation, place or pet not found").assertIsDisplayed()
        composeTestRule.onNodeWithText("RAW SERVER TEXT").assertDoesNotExist()
    }

    @Test
    fun `system back from the form returns to the occasions list, not the contact page`() {
        var leftScreen = false
        coEvery { contacts.getContact(5) } returns Result.success(
            ContactRecordResponse(id = 5, card = Card(uid = UID, name = Name(full = "Dana")))
        )
        coEvery { repository.listForContact(UID) } returns Result.success(listOf(ob()))
        val viewModel = ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to 5))
        )
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactOccasionsScreen(onBack = { leftScreen = true }, viewModel = viewModel)
            }
        }
        composeTestRule.waitForIdle()
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
        composeTestRule.onNodeWithText("Edit occasion").assertIsDisplayed()

        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Edit occasion").assertDoesNotExist()
        composeTestRule.onNodeWithText("Christmas card").assertIsDisplayed()
        assertEquals(false, leftScreen)
    }

    @Test
    fun `system back from an edited form still asks before discarding`() {
        render(listOf(ob()))
        composeTestRule.onNodeWithContentDescription("Edit Christmas card").performClick()
        composeTestRule.onNodeWithTag("occasion-ob-label").performTextReplacement("Changed")
        composeTestRule.onNodeWithText("Changed").assertIsDisplayed()
        composeTestRule.waitForIdle()

        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Edit occasion").assertIsDisplayed()
    }

    // A failed contact fetch is the shared ErrorState (localized reason + Retry), not "no VCard UID".
    @Test
    fun `a failed contact fetch shows the localized reason with Retry`() {
        coEvery { contacts.getContact(5) } returnsMany listOf(
            Result.failure(
                ApiError.Client(
                    409,
                    "raw-body-not-shown",
                    ErrorReason(ErrorReasonCode.FEED_LIMIT_REACHED, mapOf("max" to 50.0))
                )
            ),
            Result.success(ContactRecordResponse(id = 5, card = Card(uid = UID)))
        )
        coEvery { repository.listForContact(UID) } returns Result.success(emptyList())
        val viewModel = ContactOccasionsViewModel(
            repository,
            contacts,
            SavedStateHandle(mapOf("contactId" to 5))
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactOccasionsScreen(onBack = {}, viewModel = viewModel) }
        }
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("feed limit reached (maximum 50 active feeds)")
            .assertIsDisplayed()
        composeTestRule.onNodeWithText("raw-body-not-shown").assertDoesNotExist()
        composeTestRule.onNodeWithText("Contact has no VCard UID").assertDoesNotExist()

        composeTestRule.onNodeWithText("Retry").performClick()
        composeTestRule.waitForIdle()

        composeTestRule.onNodeWithText("Retry").assertDoesNotExist()
        coVerify(exactly = 2) { contacts.getContact(5) }
    }
}
