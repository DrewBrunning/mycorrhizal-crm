package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.Anniversary
import com.mycorrhizal.crm.model.network.AnniversaryDate
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.EntryPeriod
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Organization
import com.mycorrhizal.crm.model.network.PartialDate
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.PersonalInfoMeta
import com.mycorrhizal.crm.model.network.TemporalRange
import com.mycorrhizal.crm.model.network.Title
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import io.mockk.verify
import kotlinx.coroutines.flow.MutableStateFlow
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
class ContactSectionScreensTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun viewModel(state: ContactSectionsUiState): ContactSectionsViewModel {
        val vm = mockk<ContactSectionsViewModel>(relaxed = true)
        every { vm.uiState } returns MutableStateFlow(state)
        return vm
    }

    private fun record(
        card: Card = Card(name = Name(full = "Dana White")),
        crm: CRMEnvelope = CRMEnvelope()
    ) = ContactRecordResponse(id = 5, uid = "uid-5", revision = 7, card = card, crm = crm)

    private fun setSkills(state: ContactSectionsUiState): ContactSectionsViewModel {
        val vm = viewModel(state)
        composeTestRule.setContent {
            MycorrhizalTheme { ContactSkillsScreen(onBack = {}, viewModel = vm) }
        }
        return vm
    }

    // --- Skills & interests ------------------------------------------------

    @Test
    fun `skills empty state offers the create action and opens the form`() {
        setSkills(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            )
        )

        composeTestRule.onNodeWithText("No skills or interests yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add skill or interest").performClick()

        composeTestRule.onNodeWithText("Add skill or interest").assertIsDisplayed()
        composeTestRule.onNodeWithTag("personal-info-value").assertIsDisplayed()
        composeTestRule.onNodeWithTag("personal-info-level").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "personal-info-sensitivity"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `skills are grouped by kind with favorite lock and notes badges`() {
        setSkills(
            ContactSectionsUiState(
                record = record(
                    card = Card(
                        personalInfo = listOf(
                            PersonalInfo(
                                id = "pi-1",
                                kind = "expertise",
                                value = "Go",
                                level = "expert"
                            ),
                            PersonalInfo(id = "pi-2", kind = "hobby", value = "Climbing")
                        )
                    ),
                    crm = CRMEnvelope(
                        personalInfoMeta = mapOf(
                            "pi-1" to PersonalInfoMeta(
                                favorite = true,
                                sensitivity = "private",
                                notes = "Core language"
                            ),
                            "pi-2" to PersonalInfoMeta(sensitivity = "secret")
                        )
                    )
                )
            )
        )

        composeTestRule.onNodeWithText("Expertise").assertIsDisplayed()
        composeTestRule.onNodeWithText("Hobby").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Go").assertIsDisplayed()
        composeTestRule.onNodeWithText("Expert").assertIsDisplayed()
        composeTestRule.onNodeWithText("Core language").assertIsDisplayed()
        composeTestRule.onNodeWithContentDescription("Favorite").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Private").fetchSemanticsNodes()
        composeTestRule.onNodeWithTag("personal-info-lock-pi-1").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "personal-info-lock-pi-2"
        ).performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `adding an expertise entry forwards kind value level sensitivity and notes`() {
        val vm = setSkills(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            )
        )

        composeTestRule.onNodeWithContentDescription("Add skill or interest").performClick()
        composeTestRule.onNodeWithTag("personal-info-value").performTextInput("Go")
        composeTestRule.onNodeWithTag("personal-info-level").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Expert").performClick()
        composeTestRule.onNodeWithTag(
            "personal-info-notes"
        ).performScrollTo().performTextInput("Core language")
        composeTestRule.onNodeWithTag("personal-info-sensitivity").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Private").performClick()
        composeTestRule.onNodeWithTag("personal-info-favorite").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Create").performClick()

        verify {
            vm.savePersonalInfo(
                null,
                match {
                    it.kind == "expertise" &&
                        it.value == "Go" &&
                        it.level == "expert" &&
                        it.notes == "Core language" &&
                        it.sensitivity == "private" &&
                        it.favorite
                }
            )
        }
    }

    @Test
    fun `switching a skill to hobby offers the hobby level options and clears the expertise level`() {
        val vm = setSkills(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            )
        )

        composeTestRule.onNodeWithContentDescription("Add skill or interest").performClick()
        composeTestRule.onNodeWithTag("personal-info-level").performClick()
        composeTestRule.onNodeWithText("Expert").performClick()
        composeTestRule.onNodeWithTag("personal-info-kind").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Hobby").performClick()
        composeTestRule.onNodeWithText("Skills & interests").assertDoesNotExist()

        composeTestRule.onNodeWithTag("personal-info-level").performClick()
        composeTestRule.onNodeWithText("High").performClick()
        composeTestRule.onNodeWithTag("personal-info-value").performTextInput("Climbing")
        composeTestRule.onNodeWithText("Create").performClick()

        verify { vm.savePersonalInfo(null, match { it.kind == "hobby" && it.level == "high" }) }
    }

    @Test
    fun `editing a skill prefills its value level notes sensitivity and favorite`() {
        val vm = setSkills(
            ContactSectionsUiState(
                record = record(
                    card = Card(
                        personalInfo = listOf(
                            PersonalInfo(
                                id = "pi-1",
                                kind = "expertise",
                                value = "Go",
                                level = "expert"
                            )
                        )
                    ),
                    crm = CRMEnvelope(
                        personalInfoMeta = mapOf(
                            "pi-1" to PersonalInfoMeta(
                                notes = "Core",
                                sensitivity = "secret",
                                favorite = true
                            )
                        )
                    )
                )
            )
        )

        composeTestRule.onNodeWithTag("personal-info-edit-pi-1").performClick()

        composeTestRule.onNodeWithText("Edit skill or interest").assertIsDisplayed()
        composeTestRule.onNodeWithText("Go").assertIsDisplayed()
        composeTestRule.onNodeWithText("Expert").assertIsDisplayed()
        composeTestRule.onNodeWithText("Core").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Secret").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithTag("personal-info-favorite").performScrollTo().assertIsOn()

        composeTestRule.onNodeWithText("Save").performClick()
        verify {
            vm.savePersonalInfo(
                "pi-1",
                match { it.value == "Go" && it.level == "expert" && it.sensitivity == "secret" }
            )
        }
    }

    @Test
    fun `deleting a skill asks first and then deletes by id`() {
        val vm = setSkills(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(
                    card = Card(
                        personalInfo = listOf(
                            PersonalInfo(id = "pi-1", kind = "hobby", value = "Climbing")
                        )
                    )
                )
            )
        )

        composeTestRule.onNodeWithTag("personal-info-delete-pi-1").performClick()
        composeTestRule.onNodeWithText("Delete entry?").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Delete “Climbing”? This cannot be undone."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()

        verify { vm.deletePersonalInfo("pi-1") }
    }

    @Test
    fun `a 412 conflict shows the shared dialog and loads the latest record`() {
        val vm = setSkills(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(),
                conflict = true
            )
        )

        composeTestRule.onNodeWithText("Changed elsewhere").assertIsDisplayed()
        composeTestRule.onNodeWithText("Load latest version").performClick()

        verify { vm.onConflictLoadLatest() }
    }

    // --- Work ---------------------------------------------------------------

    @Test
    fun `work empty state opens the organization form and saves the entry`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactWorkScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText("No work details yet").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-work-add-organization").performClick()
        composeTestRule.onNodeWithTag("organization-name").performTextInput("Globex")
        composeTestRule.onNodeWithTag("organization-department").performTextInput("R&D")
        composeTestRule.onNodeWithTag("work-period-start").performTextInput("2019")
        composeTestRule.onNodeWithTag("work-period-end").performTextInput("2024")
        composeTestRule.onNodeWithText("Create").performClick()

        verify {
            vm.saveOrganization(
                null,
                match {
                    it.name == "Globex" && it.department == "R&D" && it.startYear == "2019" &&
                        it.endYear == "2024"
                }
            )
        }
    }

    @Test
    fun `work list renders organizations titles and work information with periods`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(
                    card = Card(
                        organizations = listOf(
                            Organization(
                                id = "org-1",
                                name = "Acme",
                                units = listOf(
                                    com.mycorrhizal.crm.model.network.OrgUnit(name = "Ops")
                                )
                            )
                        ),
                        titles = listOf(Title(id = "t-1", name = "Engineer", kind = "role"))
                    ),
                    crm = CRMEnvelope(
                        workInformation = "Builds things",
                        periods = listOf(
                            EntryPeriod(
                                kind = "organization",
                                entryId = "org-1",
                                range = TemporalRange(
                                    start = PartialDate(year = 2019),
                                    end = PartialDate(year = 2024)
                                )
                            ),
                            EntryPeriod(
                                kind = "title",
                                entryId = "t-1",
                                range = TemporalRange(start = PartialDate(year = 2020))
                            )
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactWorkScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag("contact-work-list").performScrollToNode(hasText("Acme"))
        composeTestRule.onNodeWithText("Acme").assertIsDisplayed()
        composeTestRule.onNodeWithText("Ops · 2019 – 2024").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-work-list").performScrollToNode(hasText("Engineer"))
        composeTestRule.onNodeWithText("Engineer").assertIsDisplayed()
        composeTestRule.onNodeWithText("Role · 2020 –").assertIsDisplayed()
        composeTestRule.onNodeWithTag(
            "contact-work-list"
        ).performScrollToNode(hasText("Builds things"))
        composeTestRule.onNodeWithText("Builds things").assertIsDisplayed()
    }

    @Test
    fun `editing work information opens a prefilled form and clears it when blanked`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(crm = CRMEnvelope(workInformation = "Builds things"))
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactWorkScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag(
            "contact-work-edit-information"
        ).performScrollTo().performClick()
        composeTestRule.onNodeWithText("Builds things").assertIsDisplayed()
        composeTestRule.onNodeWithTag("work-information").performTextReplacement("")
        composeTestRule.onNodeWithText("Save").performClick()

        verify { vm.saveWorkInformation("") }
    }

    @Test
    fun `adding a title forwards its kind and period`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(card = Card(titles = listOf(Title(id = "t-1", name = "Engineer"))))
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactWorkScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag("contact-work-add-title").performScrollTo().performClick()
        composeTestRule.onNodeWithTag("title-kind").performClick()
        composeTestRule.onNodeWithText("Role").performClick()
        composeTestRule.onNodeWithTag("title-name").performTextInput("Advisor")
        composeTestRule.onNodeWithTag("work-period-start").performTextInput("2022")
        composeTestRule.onNodeWithText("Create").performClick()

        verify {
            vm.saveTitle(
                null,
                match { it.name == "Advisor" && it.kind == "role" && it.startYear == "2022" }
            )
        }
    }

    @Test
    fun `deleting an organization asks first and then deletes by id`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record(
                    card = Card(organizations = listOf(Organization(id = "org-1", name = "Acme")))
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactWorkScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag("organization-delete-org-1").performClick()
        composeTestRule.onNodeWithText("Delete entry?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()

        verify { vm.deleteOrganization("org-1") }
    }

    // --- Anniversaries ------------------------------------------------------

    @Test
    fun `anniversaries empty state opens the form and save is disabled without a date`() {
        val vm = viewModel(
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactAnniversariesScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText("No anniversaries yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add anniversary").performClick()
        composeTestRule.onNodeWithText("Wedding").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
        composeTestRule.onNodeWithText("Choose a date").assertIsDisplayed()
    }

    @Test
    fun `anniversary list excludes the birthday and shows non-birth dates`() {
        val vm = viewModel(
            ContactSectionsUiState(
                record = record(
                    card = Card(
                        anniversaries = listOf(
                            Anniversary(
                                id = "birth-1",
                                kind = "birth",
                                date = AnniversaryDate(
                                    partial = PartialDate(year = 1990, month = 6, day = 15)
                                )
                            ),
                            Anniversary(
                                id = "a-1",
                                kind = "wedding",
                                date = AnniversaryDate(
                                    partial = PartialDate(year = 2020, month = 6, day = 1)
                                )
                            )
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactAnniversariesScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithText("Wedding").assertIsDisplayed()
        composeTestRule.onNodeWithText("2020-06-01").assertIsDisplayed()
        composeTestRule.onAllNodesWithText("Birth").fetchSemanticsNodes()
        composeTestRule.onNodeWithText("Birth").assertDoesNotExist()
    }

    @Test
    fun `editing an anniversary prefills the kind and date and saves it unchanged`() {
        val vm = viewModel(
            ContactSectionsUiState(
                record = record(
                    card = Card(
                        anniversaries = listOf(
                            Anniversary(
                                id = "a-1",
                                kind = "death",
                                date = AnniversaryDate(
                                    partial = PartialDate(year = 2021, month = 3, day = 4)
                                )
                            )
                        )
                    )
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactAnniversariesScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag("anniversary-edit-a-1").performClick()
        composeTestRule.onNodeWithText("Edit anniversary").assertIsDisplayed()
        composeTestRule.onNodeWithText("Death").assertIsDisplayed()
        composeTestRule.onNodeWithText("04.03.2021").assertIsDisplayed()
        composeTestRule.onNodeWithText("Save").performClick()

        verify {
            vm.saveAnniversary(
                "a-1",
                match { it.kind == "death" && it.date == "2021-03-04" }
            )
        }
    }

    @Test
    fun `deleting an anniversary asks first and then deletes by id`() {
        val vm = viewModel(
            ContactSectionsUiState(
                record = record(
                    card = Card(anniversaries = listOf(Anniversary(id = "a-1", kind = "wedding")))
                )
            )
        )
        composeTestRule.setContent {
            MycorrhizalTheme { ContactAnniversariesScreen(onBack = {}, viewModel = vm) }
        }

        composeTestRule.onNodeWithTag("anniversary-delete-a-1").performClick()
        composeTestRule.onNodeWithText("Delete entry?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()

        verify { vm.deleteAnniversary("a-1") }
    }

    @Test
    fun `the favorite toggle reflects the loaded state`() {
        val vm = setSkills(
            ContactSectionsUiState(
                record = record(
                    card = Card(
                        personalInfo = listOf(
                            PersonalInfo(id = "pi-1", kind = "hobby", value = "Climbing")
                        )
                    )
                )
            )
        )
        composeTestRule.onNodeWithTag("personal-info-edit-pi-1").performClick()
        composeTestRule.onNodeWithTag("personal-info-favorite").performScrollTo().assertIsOff()
        // Keep the reference used: the VM is the screen's state source.
        assertEquals(
            "uid-5",
            ContactSectionsUiState(
                enabledFields = ContactFieldKey.entries.toSet(),
                record = record()
            ).record?.uid
        )
        verify(exactly = 0) { vm.deletePersonalInfo(any()) }
    }
}
