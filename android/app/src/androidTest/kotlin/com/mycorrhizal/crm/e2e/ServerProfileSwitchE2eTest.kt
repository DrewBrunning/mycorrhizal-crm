package com.mycorrhizal.crm.e2e

import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import com.mycorrhizal.crm.domain.profile.defaultProfileLabel
import org.junit.Test
import java.util.UUID

/**
 * ADR 0028 Decision 1, acceptance test: a second Remote profile pointed at the
 * same backend with a different user is isolated from the first. Logging into
 * one profile shows only its own contacts; switching back restores the first
 * profile's session without re-authenticating.
 *
 * The second account starts empty, so "isolated" is asserted both ways: the
 * seed user's contact is absent under the second profile and present again
 * under the first.
 */
class ServerProfileSwitchE2eTest : E2eBaseTest() {

    @Test
    fun switchingServerProfilesIsolatesEachAccountsContacts() {
        backend.registerSeedUser(
            E2eConfig.SECOND_USERNAME,
            E2eConfig.SECOND_EMAIL,
            E2eConfig.SECOND_PASSWORD,
        )

        val token = UUID.randomUUID().toString().replace("-", "").take(8)
        val displayName = "${E2eConfig.TEST_CONTACT_PREFIX}Profile $token"
        createTestContact("E2EProfile", token)

        // 1. The seed user sees its own contact.
        openContactsAndSearch(token)
        waitForText(displayName)

        // 2. Add a second Remote profile (same backend, different account). A
        //    fresh profile has no token, so the app lands on the login screen.
        val secondLabel = "E2E Second $token"
        addRemoteProfile(secondLabel)
        waitForText("Sign in")
        loginViaUi(username = E2eConfig.SECOND_USERNAME, password = E2eConfig.SECOND_PASSWORD)

        // 3. The second account must not see the first account's contact.
        openContactsAndSearch(token)
        assertTextAbsent(displayName)

        // 4. Switch back to the first profile: its session is restored (no
        //    re-login) and its contact is visible again.
        switchToProfile(defaultProfileLabel(E2eConfig.serverUrl))
        openContactsAndSearch(token)
        waitForText(displayName)

        // 5. Remove the profile this test added.
        removeProfile(secondLabel)
    }

    private fun openContactsAndSearch(query: String) {
        navigateViaDrawer("Contacts")
        searchFor(query)
    }

    private fun assertTextAbsent(text: String, timeoutMs: Long = 15_000L) {
        compose.waitUntil(timeoutMs) {
            compose.onAllNodesWithText(text).fetchSemanticsNodes().isEmpty()
        }
    }

    private fun openServers() {
        navigateViaDrawer("Settings")
        waitForText("Servers")
        compose.onNodeWithText("Servers").performClick()
        compose.waitForIdle()
    }

    private fun addRemoteProfile(label: String) {
        openServers()
        clickContentDescription("Add server")
        waitForText("Label")
        compose.onNodeWithTag("servers-add-label").performTextInput(label)
        compose.onNodeWithTag("servers-add-url").performTextInput(E2eConfig.serverUrl)
        compose.onNodeWithTag("servers-add-confirm").performClick()
    }

    private fun switchToProfile(label: String) {
        openServers()
        waitForText(label)
        compose.onNodeWithText(label).performClick()
        compose.waitForIdle()
    }

    private fun removeProfile(label: String) {
        openServers()
        clickContentDescription("Remove $label")
        // The confirmation dialog's confirm button reads "Remove".
        compose.onNodeWithText("Remove").performClick()
        compose.waitForIdle()
    }
}
