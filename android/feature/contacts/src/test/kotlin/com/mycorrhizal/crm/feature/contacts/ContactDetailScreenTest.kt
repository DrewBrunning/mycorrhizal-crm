package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactDetailScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(state: ContactDetailUiState) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactDetailContent(contact = state.contact!!)
            }
        }
    }

    @Test
    fun `renders the contact name and sections`() {
        val contact = ContactRecordResponse(
            id = 5,
            card = Card(
                name = Name(full = "Dana White"),
                emails = listOf(Email(address = "dana@example.com", label = "Work")),
                phones = listOf(Phone(number = "+1-555-0100")),
            ),
            crm = CRMEnvelope(circles = listOf("friends", "work")),
        )
        setContent(ContactDetailUiState(contact = contact))

        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("Email").assertIsDisplayed()
        composeTestRule.onNodeWithText("dana@example.com").assertIsDisplayed()
        composeTestRule.onNodeWithText("Phone").assertIsDisplayed()
        composeTestRule.onNodeWithText("+1-555-0100").assertIsDisplayed()
        composeTestRule.onNodeWithText("Circles").assertIsDisplayed()
    }

    @Test
    fun `renders empty name fallback when no name present`() {
        val contact = ContactRecordResponse(id = 5, card = Card())
        setContent(ContactDetailUiState(contact = contact))
        composeTestRule.onNodeWithText("Contact").assertIsDisplayed()
    }
}
