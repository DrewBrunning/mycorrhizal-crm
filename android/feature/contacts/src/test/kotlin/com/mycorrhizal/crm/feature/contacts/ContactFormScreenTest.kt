package com.mycorrhizal.crm.feature.contacts

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertCountEquals
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.getBoundsInRoot
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
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
class ContactFormScreenTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private fun setContent(
        state: ContactFormState = ContactFormState(),
        onSave: () -> Unit = {},
        onBack: () -> Unit = {},
        onGivenNameChange: (String) -> Unit = {},
        onCircleToggle: (String) -> Unit = {},
        onTagToggle: (String) -> Unit = {},
        onCardNotesChange: (List<com.mycorrhizal.crm.model.network.CardNote>) -> Unit = {},
        onGenderChange: (String) -> Unit = {},
        onPreferredLanguagesChange: (
            List<com.mycorrhizal.crm.model.network.LanguagePref>
        ) -> Unit = {
        },
        onPronounsChange: (List<com.mycorrhizal.crm.model.network.Pronouns>) -> Unit = {},
        onGrammaticalGendersChange: (
            List<com.mycorrhizal.crm.model.network.GrammaticalGender>
        ) -> Unit = {
        },
        onKeywordsChange: (List<String>) -> Unit = {},
        onCardKindChange: (String) -> Unit = {},
        onAmbientChange: (Boolean) -> Unit = {}
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = onBack,
                    saveLabel = if (state.isEdit) "Save changes" else "Create contact",
                    isSaving = state.isSaving,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() },
                    isDirty = state.isDirty,
                    entityLabel = "contact"
                ) { padding ->
                    ContactFormContent(
                        state = state,
                        onGivenNameChange = onGivenNameChange,
                        onSurnameChange = {},
                        onNicknameChange = {},
                        onEmailsChange = {},
                        onPhonesChange = {},
                        onAddressesChange = {},
                        onImppChange = {},
                        onSocialChange = {},
                        onOtherServicesChange = {},
                        onLinksChange = {},
                        onBirthdayChange = {},
                        onCardNotesChange = onCardNotesChange,
                        onCardKindChange = onCardKindChange,
                        onAmbientChange = onAmbientChange,
                        onGenderChange = onGenderChange,
                        onPreferredLanguagesChange = onPreferredLanguagesChange,
                        onPronounsChange = onPronounsChange,
                        onGrammaticalGendersChange = onGrammaticalGendersChange,
                        onKeywordsChange = onKeywordsChange,
                        onCircleToggle = onCircleToggle,
                        onTagToggle = onTagToggle,
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }
    }

    // --- Issue #1734: ambient toggle ---

    @Test
    fun `the ambient switch shows its label and supporting text, off when the flag is absent`() {
        setContent(state = ContactFormState(ambient = null))

        composeTestRule.onNodeWithText(
            "Ambient contact — I see them all the time"
        ).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Counts as in regular contact without logging activities")
            .performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-ambient-toggle").performScrollTo().assertIsOff()
    }

    @Test
    fun `the ambient switch is on when the stored flag is true`() {
        setContent(state = ContactFormState(ambient = true))

        composeTestRule.onNodeWithTag("contact-ambient-toggle").performScrollTo().assertIsOn()
    }

    @Test
    fun `tapping the ambient switch reports the new value`() {
        var reported: Boolean? = null
        setContent(state = ContactFormState(ambient = true), onAmbientChange = { reported = it })

        composeTestRule.onNodeWithTag("contact-ambient-toggle").performScrollTo().performClick()
        assertEquals(false, reported)
    }

    @Test
    fun `renders the form fields`() {
        setContent(
            state = ContactFormState(
                addresses = listOf(
                    com.mycorrhizal.crm.model.network.Address(
                        components = listOf(
                            com.mycorrhizal.crm.model.network.AddressComponent(
                                kind = "name",
                                value = "1 Main St"
                            )
                        )
                    )
                ),
                // Issue #832: every field toggled on, so this test still proves every
                // section CAN render — gating itself is covered by dedicated tests below.
                enabledFields = com.mycorrhizal.crm.model.network.ContactFieldKey.entries.toSet()
            )
        )
        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Surname").assertIsDisplayed()
        composeTestRule.onNodeWithText("Prefix").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-kind-field").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithTag("card-kind-field").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Language").performScrollTo().assertIsDisplayed()
        // M7: the previously read-only/invisible field groups now have editors.
        composeTestRule.onNodeWithText("Address").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Street").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Instant messaging").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Social profiles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("How we met").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Additional contact information"
        ).performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Notes").performScrollTo().assertIsDisplayed()
        // Issue #832: fields with no prior Android UI.
        composeTestRule.onNodeWithText("Gender").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Speak to as").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Keywords").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Preferred languages").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No circles yet").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("No tags yet").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Create contact").assertIsDisplayed()
        // Issue #1728: these groups moved to their own contact-page drill-downs.
        composeTestRule.onNodeWithText("Organization").assertDoesNotExist()
        composeTestRule.onNodeWithText("Job titles").assertDoesNotExist()
        composeTestRule.onNodeWithText("Personal information").assertDoesNotExist()
        composeTestRule.onNodeWithText("Work information").assertDoesNotExist()
        composeTestRule.onNodeWithText("Anniversaries").assertDoesNotExist()
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
        composeTestRule.onNodeWithText("Save changes").assertIsDisplayed()
    }

    @Test
    fun `save button invokes the callback`() {
        var saved = false
        setContent(onSave = { saved = true })
        composeTestRule.onNodeWithText("Create contact").performClick()
        assertEquals(true, saved)
    }

    // Issue #1404: Save/Create is pinned in the Scaffold's bottomBar, not the last
    // child of the scroll column, so it never needs scrolling to.
    @Test
    @Config(qualifiers = "w360dp-h800dp")
    fun `save button is displayed without scrolling and stays displayed at the top and bottom of the form`() {
        setContent()
        val form = composeTestRule.onNode(hasScrollAction())

        composeTestRule.onNodeWithText("Create contact").assertIsDisplayed()

        form.performScrollToNode(hasText("No tags yet"))
        composeTestRule.onNodeWithText("Create contact").assertIsDisplayed()
        // The last field is not hidden behind the pinned bar.
        val lastFieldBottom = composeTestRule.onNodeWithText("No tags yet").getBoundsInRoot().bottom
        val barTop = composeTestRule.onNodeWithText("Create contact").getBoundsInRoot().top
        assertTrue(
            "last field ($lastFieldBottom) overlaps the save bar ($barTop)",
            lastFieldBottom <= barTop
        )

        form.performScrollToNode(hasText("Given name"))
        composeTestRule.onNodeWithText("Create contact").assertIsDisplayed()
    }

    @Test
    fun `save button is disabled while saving and announces the saving state`() {
        setContent(state = ContactFormState(isSaving = true))
        composeTestRule.onNodeWithText("Create contact")
            .assertIsNotEnabled()
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.StateDescription, "Saving"))
    }

    @Test
    fun `a selected circle renders as a removable chip`() {
        var toggled: String? = null
        setContent(
            state = ContactFormState(
                circles = listOf("friends"),
                allCircles = listOf(Circle(id = "c1", name = "friends"))
            ),
            onCircleToggle = { toggled = it }
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
                allTags = listOf(Tag(id = "t1", name = "close"))
            ),
            onTagToggle = { toggled = it }
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
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.KEYWORDS
            ),
            onKeywordsChange = { keywords = it }
        )
        composeTestRule.onNodeWithText("hiking").performScrollTo().performClick()
        assertEquals(emptyList<String>(), keywords)
    }

    @Test
    fun `editing an existing card note forwards the updated list`() {
        var notes: List<com.mycorrhizal.crm.model.network.CardNote>? = null
        setContent(
            state = ContactFormState(
                cardNotes = listOf(
                    com.mycorrhizal.crm.model.network.CardNote(note = "met at conf")
                ),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.CARD_NOTES
            ),
            onCardNotesChange = { notes = it }
        )
        composeTestRule.onNodeWithText("met at conf").performScrollTo().performTextInput("!")
        assertEquals("!met at conf", notes?.single()?.note)
    }

    @Test
    fun `preferred languages editor shows the loaded language value`() {
        setContent(
            state = ContactFormState(
                preferredLanguages = listOf(
                    com.mycorrhizal.crm.model.network.LanguagePref(language = "en")
                ),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.PREFERRED_LANGUAGES
            )
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
    fun `non-birth anniversaries are no longer rendered by the edit form`() {
        setContent(
            state = ContactFormState(
                anniversaries = listOf(
                    com.mycorrhizal.crm.model.network.Anniversary(
                        kind = "wedding",
                        date = com.mycorrhizal.crm.model.network.AnniversaryDate(
                            partial = com.mycorrhizal.crm.model.network.PartialDate(
                                year = 2020,
                                month = 6,
                                day = 1
                            )
                        )
                    )
                ),
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.ANNIVERSARIES
            )
        )
        // Issue #1728: the birthday picker stays in the identity form; every
        // other anniversary moved to the Anniversaries drill-down.
        composeTestRule.onNodeWithText("Birthday").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Anniversaries").assertDoesNotExist()
        composeTestRule.onNodeWithText("01.06.2020").assertDoesNotExist()
    }

    // --- Issue #832: gating by the enabled-fields toggle set ---

    @Test
    fun `a field absent from the enabled set does not render`() {
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS - ContactFieldKey.NICKNAME
            )
        )
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
    fun `the human-animal kind dropdown is never gated either`() {
        // crm.kind has no ContactFieldKey on web — same always-shown rule as
        // givenName/surname. A prior pass wired ContactFieldKey.CARD_KIND to this
        // dropdown by mistake; that key now correctly gates a separate field (below).
        setContent(state = ContactFormState(enabledFields = emptySet()))
        composeTestRule.onNodeWithTag("contact-kind-field").assertIsDisplayed()
    }

    @Test
    fun `the real cardKind dropdown is gated and distinct from the kind dropdown`() {
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.CARD_KIND
            )
        )
        composeTestRule.onNodeWithTag("contact-kind-field").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithTag("card-kind-field").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the cardKind dropdown does not render when its key is disabled`() {
        setContent(state = ContactFormState(enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS))
        composeTestRule.onNodeWithTag("card-kind-field").assertDoesNotExist()
    }

    @Test
    fun `selecting a cardKind option forwards its token`() {
        var cardKind: String? = null
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.CARD_KIND
            ),
            onCardKindChange = { cardKind = it }
        )
        composeTestRule.onNodeWithTag("card-kind-field").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Organization").performClick()
        assertEquals("org", cardKind)
    }

    @Test
    fun `the online services section header is absent when all three of its fields are disabled`() {
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS -
                    ContactFieldKey.IMPP_ADDRESSES - ContactFieldKey.SOCIAL_PROFILES -
                    ContactFieldKey.OTHER_ONLINE_SERVICES
            )
        )
        composeTestRule.onNodeWithText("Online services").assertDoesNotExist()
    }

    @Test
    fun `the online services section header renders when only one of its three fields is enabled`() {
        setContent(
            state = ContactFormState(
                enabledFields = DEFAULT_ENABLED_CONTACT_FIELDS + ContactFieldKey.SOCIAL_PROFILES
            )
        )
        composeTestRule.onNodeWithText("Online services").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Social profiles").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Instant messaging").assertDoesNotExist()
    }

    // Issue #1665 / ADR 0008: a 412 on save keeps the form and offers keep-editing / load-latest.
    private fun formViewModel(state: ContactFormState): ContactFormViewModel {
        val viewModel = io.mockk.mockk<ContactFormViewModel>(relaxed = true)
        io.mockk.every { viewModel.uiState } returns kotlinx.coroutines.flow.MutableStateFlow(state)
        io.mockk.every { viewModel.events } returns
            kotlinx.coroutines.flow.MutableStateFlow<ContactFormEvent?>(
                null
            )
        return viewModel
    }

    @Test
    fun `a save conflict shows the edit prompt and routes both buttons to the view model`() {
        val viewModel = formViewModel(ContactFormState(contactId = 5, conflict = true))

        composeTestRule.setContent {
            MycorrhizalTheme { ContactFormScreen(onSaved = {}, onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Your edit was not saved.",
            substring = true
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Load latest version").performClick()
        io.mockk.verify(exactly = 1) { viewModel.onConflictLoadLatest() }
        composeTestRule.onNodeWithText("Keep editing").performClick()
        io.mockk.verify(exactly = 1) { viewModel.onConflictKeepEditing() }
    }

    @Test
    fun `no conflict prompt on the form without a conflict`() {
        val viewModel = formViewModel(ContactFormState(contactId = 5))

        composeTestRule.setContent {
            MycorrhizalTheme { ContactFormScreen(onSaved = {}, onBack = {}, viewModel = viewModel) }
        }

        composeTestRule.onNodeWithText("Changed elsewhere").assertDoesNotExist()
    }

    // --- Issue #1751: unsaved-changes guard ---

    private fun pristineState(): ContactFormState =
        ContactFormState().let { it.copy(baseline = it.snapshotForBaseline()) }

    @Test
    fun `an untouched contact form leaves from the up arrow without asking`() {
        var back = false
        setContent(state = pristineState(), onBack = { back = true })

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        assertTrue(back)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `typing then system back asks before discarding, and keep editing keeps the input`() {
        var back = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                var state by remember { mutableStateOf(pristineState()) }
                FormScaffold(
                    title = "Edit contact",
                    onBack = { back = true },
                    saveLabel = "Save changes",
                    isSaving = false,
                    onSave = {},
                    snackbarHostState = remember { SnackbarHostState() },
                    isDirty = state.isDirty,
                    entityLabel = "contact"
                ) { padding ->
                    ContactFormContent(
                        state = state,
                        onGivenNameChange = { state = state.copy(givenName = it) },
                        onSurnameChange = {},
                        onNicknameChange = { state = state.copy(nickname = it) },
                        onEmailsChange = {},
                        onPhonesChange = {},
                        onAddressesChange = {},
                        onImppChange = {},
                        onSocialChange = {},
                        onOtherServicesChange = {},
                        onLinksChange = {},
                        onBirthdayChange = {},
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("Given name").performTextInput("Caz")
        composeTestRule.onNodeWithText("Caz").assertIsDisplayed()
        composeTestRule.waitForIdle()
        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Your edits to this contact haven't been saved."
        ).assertIsDisplayed()
        assertEquals(false, back)

        composeTestRule.onNodeWithText("Keep editing").performClick()

        composeTestRule.onNodeWithText("Caz").assertIsDisplayed()
        assertEquals(false, back)
    }

    @Test
    fun `discard leaves the contact form`() {
        var back = false
        composeTestRule.setContent {
            MycorrhizalTheme {
                var state by remember { mutableStateOf(pristineState()) }
                FormScaffold(
                    title = "Edit contact",
                    onBack = { back = true },
                    saveLabel = "Save changes",
                    isSaving = false,
                    onSave = {},
                    snackbarHostState = remember { SnackbarHostState() },
                    isDirty = state.isDirty,
                    entityLabel = "contact"
                ) { padding ->
                    ContactFormContent(
                        state = state,
                        onGivenNameChange = { state = state.copy(givenName = it) },
                        onSurnameChange = {},
                        onNicknameChange = { state = state.copy(nickname = it) },
                        onEmailsChange = {},
                        onPhonesChange = {},
                        onAddressesChange = {},
                        onImppChange = {},
                        onSocialChange = {},
                        onOtherServicesChange = {},
                        onLinksChange = {},
                        onBirthdayChange = {},
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }

        composeTestRule.onNodeWithText("Given name").performTextInput("Caz")
        composeTestRule.onNodeWithText("Caz").assertIsDisplayed()
        composeTestRule.waitForIdle()
        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }
        composeTestRule.onNodeWithText("Discard").performClick()

        assertTrue(back)
    }

    // --- Issue #1772: landscape / half-height compact window ---

    @Test
    @Config(qualifiers = "w640dp-h360dp")
    fun `landscape puts save in the top bar and keeps the form field on screen`() {
        var saved = false
        setContent(onSave = { saved = true })

        composeTestRule.onNodeWithText("Given name").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create contact").assertIsDisplayed().performClick()

        assertTrue(saved)
    }
}
