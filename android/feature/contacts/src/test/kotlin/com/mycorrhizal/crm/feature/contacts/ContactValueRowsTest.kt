package com.mycorrhizal.crm.feature.contacts

import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performScrollToNode
import androidx.compose.ui.unit.Density
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.Email
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.PersonalInfo
import com.mycorrhizal.crm.model.network.Phone
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/**
 * Issues #1755 + #1756: the contact detail email/phone rows show a translated type label
 * (never the raw `cell`/`home` token) on its own line, separate from the value, including
 * at font scale 2.0.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@OptIn(ExperimentalMaterial3Api::class)
class ContactValueRowsTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val longEmail = "nadia.okonkwo.long-address@example-domain.com"

    private fun show(card: Card, fontScale: Float = 1f) {
        composeTestRule.setContent {
            val density = LocalDensity.current
            CompositionLocalProvider(LocalDensity provides Density(density.density, fontScale)) {
                MycorrhizalTheme {
                    ContactDetailContent(contact = ContactRecordResponse(id = 9, card = card))
                }
            }
        }
    }

    private fun scrollTo(text: String) =
        composeTestRule.onNodeWithTag("contact-detail-list").performScrollToNode(hasText(text))

    private val card = Card(
        name = Name(full = "Nadia Okonkwo"),
        phones = listOf(Phone(number = "+44 20 7946 1234", features = listOf("cell"))),
        emails = listOf(Email(address = longEmail, contexts = listOf("home"))),
        personalInfo = listOf(PersonalInfo(kind = "hobby", value = "pottery"))
    )

    @Test
    fun `phone and email show the translated type, not the raw token`() {
        show(card)
        scrollTo("Mobile").assertIsDisplayed()
        composeTestRule.onNodeWithText("Mobile").assertIsDisplayed()
        scrollTo("Home")
        composeTestRule.onNodeWithText("Home").assertIsDisplayed()
        assertEquals(0, composeTestRule.onAllNodesWithTextCount("cell"))
        assertEquals(0, composeTestRule.onAllNodesWithTextCount("home"))
    }

    private fun assertSeparateLines(scale: Float) {
        show(card, scale)
        scrollTo(longEmail)
        composeTestRule.onNodeWithText(longEmail).assertIsDisplayed()
        scrollTo("Home")
        composeTestRule.onNodeWithText("Home").assertIsDisplayed()
        val value = composeTestRule.onNodeWithText(longEmail).fetchSemanticsNode()
        val type = composeTestRule.onNodeWithText("Home").fetchSemanticsNode()
        assertTrue(
            "type label must sit below the value",
            type.boundsInRoot.top >= value.boundsInRoot.bottom - 1f
        )
    }

    @Test
    fun `value and type are separate lines at font scale 1`() = assertSeparateLines(1f)

    @Test
    fun `value and type are separate lines at font scale 2`() = assertSeparateLines(2f)

    @Test
    fun `unknown custom type is humanized and the skills drill-down shows the personal info count`() {
        show(
            card.copy(
                phones = listOf(Phone(number = "+1 555 0100", contexts = listOf("beach_house"))),
                emails = emptyList()
            )
        )
        scrollTo("Beach house").assertIsDisplayed()
        // Issue #1728: the inline "Hobby: pottery" row moved behind the
        // Skills & interests drill-down, which now carries the count.
        scrollTo("Skills & interests · 1").assertIsDisplayed()
        assertEquals(0, composeTestRule.onAllNodesWithTextCount("Hobby: pottery"))
    }
}

private fun androidx.compose.ui.test.junit4.ComposeContentTestRule.onAllNodesWithTextCount(
    text: String
): Int = onAllNodes(hasText(text)).fetchSemanticsNodes().size
