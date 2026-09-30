package com.mycorrhizal.crm.e2e

import android.content.Intent
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import com.mycorrhizal.crm.MainActivity
import com.mycorrhizal.crm.domain.repository.AutoLockDelay
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Issue #1271 (ADR 0029 §6): text shared into the app via `ACTION_SEND` opens the
 * "Share to…" picker, then a note form prefilled with the text — and nothing is
 * written until the user saves. Against the real backend, with the notes read
 * back through the API rather than the UI.
 *
 * Delivered from inside the running activity (→ onNewIntent of the rule's own instance;
 * issue #1321) — the in-app route only. The system share sheet starts the activity from
 * another task with FLAG_ACTIVITY_NEW_TASK; that cross-task path is covered by
 * [CrossTaskIntentE2eTest] (issue #1343).
 */
@RunWith(AndroidJUnit4::class)
class ShareToCrmE2eTest : E2eBaseTest() {

    private val given = uniqueName("Share")
    private val surname = "Target"
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

    private fun share(text: String, subject: String? = null) {
        val intent = Intent(Intent.ACTION_SEND)
            .setType("text/plain")
            .putExtra(Intent.EXTRA_TEXT, text)
            .apply { subject?.let { putExtra(Intent.EXTRA_SUBJECT, it) } }
        deliverToRunningActivity(intent)
    }

    @Test
    fun sharedTextPrefillsTheNoteFormAndBackingOutCreatesNothing() {
        val shared = "Met at the conference — follow up on the grant"
        share(shared)

        waitForText("Share to…")
        waitForText(displayName)
        compose.onNodeWithText(displayName).performClick()

        // The form's content field carries exactly the shared text.
        waitForText(shared)

        // Back out of the form: the draft is discarded and no note exists.
        clickBack()
        compose.waitForIdle()
        assertEquals(emptyList<String>(), backend.noteContents(contactId))
    }

    @Test
    fun subjectIsPrependedWithABlankLine() {
        share("body text", subject = "Subject line")
        waitForText("Share to…")
        waitForText(displayName)
        compose.onNodeWithText(displayName).performClick()
        waitForText("Subject line\n\nbody text")
    }

    @Test
    fun backingOutOfThePickerDiscardsTheDraft() {
        share("never saved")
        waitForText("Share to…")
        clickBack()
        waitForTextGone("Share to…")
        assertTrue(compose.onAllNodesWithText("never saved").fetchSemanticsNodes().isEmpty())
        assertEquals(emptyList<String>(), backend.noteContents(contactId))
    }

    @Test
    fun appLockShowsTheLockScreenBeforeThePicker() {
        val activity = compose.activity as MainActivity
        armAppLock()
        waitForText("Your data is locked")

        share("locked share")
        compose.waitForIdle()
        check(compose.onAllNodesWithText("Share to…").fetchSemanticsNodes().isEmpty()) {
            "the picker must not render behind the lock"
        }

        activity.appLockController.onUserAuthenticated()
        waitForText("Share to…")
    }
}
