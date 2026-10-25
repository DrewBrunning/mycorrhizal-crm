package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.hasText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import java.util.UUID
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #1948 acceptance: the v1.5.0 contact drill-downs must round-trip
 * through the real backend — the target-closeness picker
 * (`PUT /contacts/:id/target-closeness`), the ambient toggle (contact update),
 * and the personal-info drill-down (contact update). The JVM suite covers the
 * screens; only this layer proves the new fields serialize and the entry
 * points are reachable on a device.
 */
@RunWith(AndroidJUnit4::class)
class ContactV15DrillDownsE2eTest : E2eBaseTest() {

    private val givenName = uniqueName("V15")
    private val surname = "Temp"
    private val displayName = "$givenName $surname"
    private val hobbyValue = "Bouldering ${UUID.randomUUID().toString().take(8)}"

    private lateinit var contact: E2eBackend.SeedContact

    @Before
    fun seedContact() {
        contact = createTestContact(givenName, surname)
    }

    @Test
    fun targetClosenessRoundTripsAndShowsOnTheChip() {
        navigateToContact()

        waitForTag("health-score-badge")
        compose.onNodeWithTag("health-score-badge").performClick()
        waitForText("Relationship health")
        compose.onNodeWithTag("target-closeness-picker").performClick()
        waitForText("Casual")
        compose.onNodeWithText("Casual").performClick()

        // A contact with no cadence policy offers to create one; defer it so
        // the target (not the policy) is what this test round-trips.
        waitForText("Create a cadence?")
        clickTextWhenStable("Not now")

        // "Casual" (unlike the "Close" tier) cannot collide with the dialog's
        // Close button, so the picker's new value is unambiguous.
        waitForText("Casual")
        onLastText("Close").performClick()
        waitForTextGone("Relationship health")

        // Navigate away and back: the chip must come from a fresh server read,
        // not the in-memory record the picker just updated.
        clickBack()
        openSeededContactFromList()
        waitForTag("target-closeness-chip")
        compose.onNodeWithTag("target-closeness-chip")
            .assertIsDisplayed()
            .assert(hasText("Target: Casual", substring = true))
        // The UI read could still be a local cache; the server is the oracle
        // for the dedicated target-closeness route's round trip.
        compose.waitUntil(15_000) {
            backend.contactState(contact.id).optJSONObject("crm")
                ?.optString("target_closeness") == "casual"
        }
    }

    @Test
    fun ambientToggleSurvivesSaveAndReload() {
        navigateToContact()

        clickContentDescription("Edit contact")
        waitForText("Save changes")
        // The form fetches the record asynchronously; its given-name value is
        // the signal that the loaded state (including ambient) has landed.
        waitForText(givenName)
        compose.onNodeWithTag("contact-ambient-toggle").performScrollTo().performClick()
        clickTextWhenStable("Save changes")

        // Back on the refreshed detail page.
        waitForTag("health-score-badge")

        // Re-open the form against a fresh read and assert the switch came back
        // on — the flag serialized through the contact-update round trip.
        clickContentDescription("Edit contact")
        waitForText(givenName)
        compose.onNodeWithTag("contact-ambient-toggle").performScrollTo().assertIsOn()
        // The form could have loaded from a local cache; assert the flag is on
        // the server the update was sent to.
        compose.waitUntil(15_000) {
            backend.contactState(contact.id).optJSONObject("crm")?.optBoolean("ambient") == true
        }
    }

    @Test
    fun hobbyFromPersonalInfoDrillDownRendersAfterReload() {
        navigateToContact()

        scrollToNodeWhenStable("contact-detail-list", hasTestTag("drill-down-skills"))
        compose.onNodeWithTag("drill-down-skills").performClick()

        waitForText("No skills or interests yet")
        clickContentDescription("Add skill or interest")
        compose.onNodeWithTag("personal-info-kind").performClick()
        waitForText("Hobby")
        compose.onNodeWithText("Hobby").performClick()
        compose.onNodeWithTag("personal-info-value").performTextInput(hobbyValue)
        clickTextWhenStable("Create")

        // The drill-down saves through the full-record PUT and reloads its rows.
        waitForText(hobbyValue)
        clickBack()

        // The detail counts the new entry, and re-entering the drill-down reads
        // it back from the server rather than from the add form's state.
        scrollToNodeWhenStable("contact-detail-list", hasTestTag("drill-down-skills"))
        waitForText("Skills & interests · 1")
        compose.onNodeWithTag("drill-down-skills").performClick()
        waitForText(hobbyValue)

        // The saved entry is on the server, not just in the form's state or a
        // local cache: the full-record PUT carried the new personal-info row.
        compose.waitUntil(15_000) {
            val entries = backend.contactState(
                contact.id
            ).optJSONObject("card")?.optJSONArray("personalInfo")
            (0 until (entries?.length() ?: 0)).any { index ->
                val entry = entries!!.getJSONObject(index)
                entry.optString("kind") == "hobby" && entry.optString("value") == hobbyValue
            }
        }
    }

    private fun navigateToContact() {
        waitForText("Dashboard")
        navigateViaDrawer("Contacts")
        openSeededContactFromList()
    }

    private fun openSeededContactFromList() {
        waitForText("Search contacts")
        searchFor(givenName)
        waitForText(displayName)
        clickTextWhenStable(displayName)
        waitForTag("contact-detail-list")
    }
}
