package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: ContactFormState = ContactFormState(),
        onSave: () -> Unit = {},
        onGivenNameChange: (String) -> Unit = {},
        onCircleToggle: (String) -> Unit = {},
        onTagToggle: (String) -> Unit = {},
        onCardNotesChange: (List<com.mycorrhizal.crm.model.network.CardNote>) -> Unit = {},
        onGenderChange: (String) -> Unit = {},
        onPreferredLanguagesChange: (List<com.mycorrhizal.crm.model.network.LanguagePref>) -> Unit = {},
        onPronounsChange: (List<com.mycorrhizal.crm.model.network.Pronouns>) -> Unit = {},
        onGrammaticalGendersChange: (List<com.mycorrhizal.crm.model.network.GrammaticalGender>) -> Unit = {},
        onKeywordsChange: (List<String>) -> Unit = {},
        onAnniversariesChange: (List<com.mycorrhizal.crm.model.network.Anniversary>) -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFormContent(
                    state = state,
                    onGivenNameChange = onGivenNameChange,
                    onSurnameChange = {},
                    onNicknameChange = {},
                    onEmailsChange = {},
                    onPhonesChange = {},
                    onAddressesChange = {},
                    onTitlesChange = {},
                    onImppChange = {},
                    onSocialChange = {},
                    onOtherServicesChange = {},
                    onLinksChange = {},
                    onPersonalInfoChange = {},
                    onBirthdayChange = {},
                    onCardNotesChange = onCardNotesChange,
                    onGenderChange = onGenderChange,
                    onPreferredLanguagesChange = onPreferredLanguagesChange,
                    onPronounsChange = onPronounsChange,
                    onGrammaticalGendersChange = onGrammaticalGendersChange,
                    onKeywordsChange = onKeywordsChange,
                    onAnniversariesChange = onAnniversariesChange,
                    onCircleToggle = onCircleToggle,
                    onTagToggle = onTagToggle,
                    onSave = onSave,
                )
            }
        }
    }

    @Test
    fun `renders the form fields`() {
        setContent(
            state = ContactFormState(
                addresses = listOf(
                    com.mycorrhizal.crm.model.network.Address(
                        components = listOf(
                            com.mycorrhizal.crm.model.network.AddressComponent(kind = "name", value = "1 Main St"),
                        ),
                    ),
                ),
                // Issue #832: every field toggled on, so this test still proves every
                // section CAN render — gating itself is covered by dedicated tests below.
                enabledFields = com.mycorrhizal.crm.model.network.ContactFieldKey.entries.toSet(),
            ),
        )
        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Surname").assertIsDisplayed()
        composeTestRule.onNodeWithText("Prefix").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Kind").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Language").performScrollTo().assertIsDisplayed()
        // M7: the previously read-only/invisible field groups now have editors.
        composeTestRule.onNodeWithText("Address").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Street").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Job titles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Instant Messaging").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Social profiles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Personal information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("How we met").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Work information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Additional contact information").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes").performScrollTo().assertIsDisplayed()
        // Issue #832: fields with no prior Android UI.
        composeTestRule.onNodeWithText("Gender").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Speak to as").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Keywords").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Preferred languages").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Anniversaries").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No circles yet").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No tags yet").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create contact").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `typing a given name forwards the change`() {
        var name: String? = null
        setContent(onGivenNameChange = { name = it })
        composeTestRule.onNodeWithText("Given name").performTextInput("Carol")
        assertEquals("Carol", name)
    }

    @Test
    fun `nickname field is rendered exactly once`() {
        // Regression test for #1121: the form used to render two identical
        // nickname OutlinedTextFields bound to the same state and callback.
        setContent()
        composeTestRule.onAllNodesWithText("Nickname").assertCountEquals(1)
    }

    @Test
    fun `edit mode shows the save label`() {
        setContent(state = ContactFormState(contactId = 5))
        composeTestRule.onNodeWithText("Save changes").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `save button invokes the callback`() {
        var saved = false
        setContent(onSave = { saved = true })
        composeTestRule.onNodeWithText("Create contact").performScrollTo().performClick()
        assertEquals(true, saved)
    }

    @Test
    fun `a selected circle renders as a removable chip`() {
        var toggled: String? = null
        setContent(
            state = ContactFormState(
                circles = listOf("friends"),
                allCircles = listOf(Circle(id = "c1", name = "friends")),
            ),
            onCircleToggle = { toggled = it },
        )
        composeTestRule.onNodeWithText("friends").performScrollTo().performClick()
        assertEquals("friends", toggled)
    }

    @Test
    fun `a selected tag renders as a removable chip`() {
        var toggled: String? = null
        setContent(
            state = ContactFormState(
                tags = listOf("close"),
                allTags = listOf(Tag(id = "t1", name = "close")),
            ),
            onTagToggle = { toggled = it },
        )
        composeTestRule.onNodeWithText("close").performScrollTo().performClick()
        assertEquals("close", toggled)
    }

    // --- Issue #832: fields with no prior Android UI ---

    @Test
    fun `typing a gender forwards the change`() {
        var gender: String? = null
        setContent(onGenderChange = { gender = it })
        composeTestRule.onNodeWithText("Gender").performScrollTo().performTextInput("they/them")
        assertEquals("they/them", gender)
    }

    @Test
    fun `an existing keyword renders as a removable chip`() {
        var keywords: List<String>? = null
        setContent(
            state = ContactFormState(
                keywords = listOf("hiking"),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.KEYWORDS,
            ),
            onKeywordsChange = { keywords = it },
        )
        composeTestRule.onNodeWithText("hiking").performScrollTo().performClick()
        assertEquals(emptyList<String>(), keywords)
    }

    @Test
    fun `editing an existing card note forwards the updated list`() {
        var notes: List<com.mycorrhizal.crm.model.network.CardNote>? = null
        setContent(
            state = ContactFormState(
                cardNotes = listOf(com.mycorrhizal.crm.model.network.CardNote(note = "met at conf")),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.CARD_NOTES,
            ),
            onCardNotesChange = { notes = it },
        )
        composeTestRule.onNodeWithText("met at conf").performScrollTo().performTextInput("!")
        assertEquals("!met at conf", notes?.single()?.note)
    }

    @Test
    fun `preferred languages editor shows the loaded language value`() {
        setContent(
            state = ContactFormState(
                preferredLanguages = listOf(com.mycorrhizal.crm.model.network.LanguagePref(language = "en")),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.PREFERRED_LANGUAGES,
            ),
        )
        composeTestRule.onNodeWithText("Preferred languages").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("en").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `pronouns and grammatical gender editors are both rendered under speak to as`() {
        setContent()
        composeTestRule.onNodeWithText("Speak to as").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Pronouns").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Grammatical gender").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `anniversaries editor is rendered separately from the birthday field`() {
        setContent(
            state = ContactFormState(
                anniversaries = listOf(
                    com.mycorrhizal.crm.model.network.Anniversary(
                        kind = "wedding",
                        date = com.mycorrhizal.crm.model.network.AnniversaryDate(
                            partial = com.mycorrhizal.crm.model.network.PartialDate(year = 2020, month = 6, day = 1),
                        ),
                    ),
                ),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.ANNIVERSARIES,
            ),
        )
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Anniversaries").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("2020-06-01").performScrollTo().assertIsDisplayed()
    }

    // --- Issue #832: gating by the enabled-fields toggle set ---

    @Test
    fun `a field absent from the enabled set does not render`() {
        setContent(state = ContactFormState(enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS - ContactFieldKey.NICKNAME))
        composeTestRule.onNodeWithText("Nickname").assertDoesNotExist()
    }

    @Test
    fun `a field present in the enabled set renders`() {
        setContent(state = ContactFormState(enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS))
        composeTestRule.onNodeWithText("Nickname").assertIsDisplayed()
    }

    @Test
    fun `given name and surname are never gated`() {
        // Web parity: firstname/lastname have no ContactFieldKey and are always shown,
        // even with every other field disabled.
        setContent(state = ContactFormState(enabledFields = emptySet()))
        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Surname").assertIsDisplayed()
    }

    @Test
    fun `the online services section header is absent when all three of its fields are disabled`() {
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS -
                    ContactFieldKey.IMPP_ADDRESSES - ContactFieldKey.SOCIAL_PROFILES - ContactFieldKey.OTHER_ONLINE_SERVICES,
            ),
        )
        composeTestRule.onNodeWithText("Online services").assertDoesNotExist()
    }

    @Test
    fun `the online services section header renders when only one of its three fields is enabled`() {
        setContent(
            state = ContactFormState(enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.SOCIAL_PROFILES),
        )
        composeTestRule.onNodeWithText("Online services").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Social profiles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Instant Messaging").assertDoesNotExist()
    }
}
