package com.mycorrhizal.crm.e2e

import android.content.Intent
import android.net.Uri
import androidx.compose.ui.test.onNodeWithText
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.domain.repository.AutoLockDelay
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import androidx.compose.ui.test.onAllNodesWithText

/**
 * Issue #1269 (ADR 0029 §2/§4): the public `mycorrhizal://` VIEW deep links
 * against the real backend — a logged-in link opens the seeded contact, a
 * search link filters the list, a rejected link leaves the start screen alone,
 * and with the app lock armed the lock screen comes first and the contact only
 * after unlock.
 *
 * The link is delivered to the running singleTask activity via the running activity's
 * startActivity (→ onNewIntent), exactly what the system does for `adb shell am start -a VIEW`.
 */
@RunWith(AndroidJUnit4::class)
class DeepLinkE2eTest : E2eBaseTest() {

    private val given = uniqueName("Deep")
    private val surname = "Linked"
    private val displayName = "$given $surname"
    private var contactId = 0L

    @Before
    fun seedContact() {
        contactId = createTestContact(given, surname).id
    }

    @After
    fun disarmAppLock() {
        val activity = compose.activity as MainActivity
        runBlocking { activity.localAuthSettings.setRequireLocalAuth(false) }
    }

    private fun deliver(uri: String) =
        deliverToRunningActivity(Intent(Intent.ACTION_VIEW, Uri.parse(uri)))

    @Test
    fun contactLinkOpensTheSeededContactDetail() {
        deliver("mycorrhizal://contacts/$contactId")
        waitForText(displayName)
        waitForContentDescription("Mark $displayName as favorite")
    }

    @Test
    fun searchLinkOpensTheListFilteredToTheQuery() {
        deliver("mycorrhizal://search?q=$given")
        waitForText(displayName)
        compose.onNodeWithText(given) // the prefilled search field
    }

    @Test
    fun rejectedLinksLeaveTheStartScreenAlone() {
        deliver("mycorrhizal://settings")
        deliver("mycorrhizal://contacts/42/edit")
        compose.waitForIdle()
        waitForText("Dashboard")
        compose.onAllNodesWithText(displayName).fetchSemanticsNodes().let {
            check(it.isEmpty()) { "a rejected link must not navigate" }
        }
    }

    @Test
    fun appLockShowsTheLockScreenFirstThenTheContactAfterUnlock() {
        val activity = compose.activity as MainActivity
        runBlocking {
            activity.localAuthSettings.setAutoLockDelay(AutoLockDelay.IMMEDIATELY)
            activity.localAuthSettings.setRequireLocalAuth(true)
        }
        // Background → foreground past a zero grace period re-arms the gate.
        activity.appLockController.onAppBackgrounded()
        activity.appLockController.onAppForegrounded()
        waitForText("Your data is locked")

        deliver("mycorrhizal://contacts/$contactId")
        compose.waitForIdle()
        check(compose.onAllNodesWithText(displayName).fetchSemanticsNodes().isEmpty()) {
            "the contact must not render behind the lock"
        }

        activity.appLockController.onUserAuthenticated()
        waitForText(displayName)
    }
}
