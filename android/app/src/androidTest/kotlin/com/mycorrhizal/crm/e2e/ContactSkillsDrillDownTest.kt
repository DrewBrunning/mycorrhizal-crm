package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * Issue #1728 acceptance: add an expertise entry from the Skills & interests
 * drill-down and see it on the contact detail screen. Runs against the real
 * docker-compose.test.yml backend like the rest of the E2E suite.
 */
@RunWith(AndroidJUnit4::class)
class ContactSkillsDrillDownTest : E2eBaseTest() {

    private val givenName = uniqueName("Skills")
    private val surname = "Temp"
    private val displayName = "$givenName $surname"
    private val skillValue = "Go ${UUID.randomUUID().toString().take(8)}"

    @Before
    fun seedContact() {
        createTestContact(givenName, surname)
    }

    @Test
    fun addExpertiseFromDrillDownAndSeeItOnDetail() {
        waitForText("Dashboard")
        navigateViaDrawer("Contacts")
        searchFor(givenName)
        waitForText(displayName)
        compose.onNodeWithText(displayName).performClick()

        waitForText(displayName)
        // The Skills & interests row is near the bottom of the detail list.
        scrollToNodeWhenStable("contact-detail-list", hasTestTag("drill-down-skills"))
        compose.onNodeWithTag("drill-down-skills").performClick()

        waitForText("No skills or interests yet")
        compose.onNodeWithContentDescription("Add skill or interest").performClick()

        // Expertise is the form's default kind; record its level so the
        // round-trip through Card.PersonalInfo.level is exercised too.
        compose.onNodeWithTag("personal-info-value").performTextInput(skillValue)
        compose.onNodeWithTag("personal-info-level").performClick()
        waitForText("Expert")
        compose.onNodeWithText("Expert").performClick()
        compose.onNodeWithText("Create").performClick()

        // The drill-down saves through the full-record PUT and reloads the row.
        waitForText(skillValue)
        waitForText("Expert")

        clickBack()
        // Returning reloads the detail (ON_RESUME): its body is briefly a
        // skeleton while the retained record keeps the top-bar title visible.
        // The scroll helper waits through that, so retrying on the title alone
        // would race the list being swapped out.
        scrollToNodeWhenStable("contact-detail-list", hasTestTag("drill-down-skills"))
        waitForText("Skills & interests · 1")
    }
}
