package com.mycorrhizal.crm.feature.timeline

import android.content.ContentResolver
import android.net.Uri
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.test.swipeUp
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.ui.components.FormScaffold
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.every
import io.mockk.mockk
import java.io.ByteArrayInputStream
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M19: the note form's contact reassignment (web's EditTimelineItemDialog
// parity), driving the real NoteFormContent — the assigned-contact chip with
// its Clear action, and the debounced search-to-pick flow.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class NoteFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        state: NoteFormState,
        onSearchChange: (String) -> Unit = {},
        onPickContact: (ContactSummary) -> Unit = {},
        onClearContact: () -> Unit = {},
        onSave: () -> Unit = {},
        onAddImages: () -> Unit = {},
        onDeleteImage: (Int) -> Unit = {},
        onShare: (() -> Unit)? = null
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = {},
                    saveLabel = if (state.isEdit) "Save changes" else "Create note",
                    isSaving = state.isSaving,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() }
                ) { padding ->
                    NoteFormContent(
                        state = state,
                        onContentChange = {},
                        onDateChange = {},
                        onContactSearchChange = onSearchChange,
                        onPickContact = onPickContact,
                        onClearContact = onClearContact,
                        onAddImages = onAddImages,
                        onDeleteImage = onDeleteImage,
                        onShare = onShare,
                        modifier = Modifier.padding(padding)
                    )
                }
            }
        }
    }

    @Test
    fun `readPickedNoteImages maps bytes and mime types`() {
        val resolver = mockk<ContentResolver>()
        val uri = Uri.parse("content://test/1")
        every { resolver.openInputStream(uri) } returns ByteArrayInputStream(byteArrayOf(1, 2))
        every { resolver.getType(uri) } returns "image/png"

        val picked = readPickedNoteImages(resolver, listOf(uri))

        assertEquals(1, picked.size)
        assertEquals("image/png", picked[0].mimeType)
        assertEquals(2, picked[0].bytes.size)
    }

    @Test
    fun `the image section offers add photos`() {
        var added = 0
        setContent(NoteFormState(contactId = 5), onAddImages = { added += 1 })
        composeTestRule.onNodeWithContentDescription("Add photos")
            .performScrollTo()
            .performClick()
        assertEquals(1, added)
    }

    @Test
    fun `renders an existing image and forwards delete`() {
        var deleted: Int? = null
        setContent(
            NoteFormState(
                contactId = 5,
                images = listOf(NoteImage(id = 9, noteId = 5)),
                pendingImages = listOf(
                    PendingNoteImage("a.jpg", "image/jpeg", byteArrayOf(1))
                )
            ),
            onDeleteImage = { deleted = it }
        )
        composeTestRule.onNodeWithContentDescription("Delete photo")
            .performScrollTo()
            .performClick()
        assertEquals(9, deleted)
    }

    @Test
    fun `shows the assigned contact chip and its clear action`() {
        var cleared = false
        setContent(
            NoteFormState(contactId = 5, targetContactId = 9, targetContactName = "Dana Lee"),
            onClearContact = { cleared = true }
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Clear").performScrollTo().performClick()
        assertEquals(true, cleared)
    }

    @Test
    fun `typing in the search field invokes the contact search`() {
        var query = ""
        setContent(
            NoteFormState(contactId = 5),
            onSearchChange = { query = it }
        )
        composeTestRule.onNodeWithText("Search contacts").performScrollTo().performTextInput("dana")
        assertEquals("dana", query)
    }

    @Test
    fun `tapping a search result reassigns the note`() {
        var picked: ContactSummary? = null
        setContent(
            NoteFormState(
                contactId = 5,
                contactSearchQuery = "dana",
                contactSearchResults = listOf(
                    ContactSummary(id = 9, firstname = "Dana", lastname = "Lee")
                )
            ),
            onPickContact = { picked = it }
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().performClick()
        assertEquals(9, picked?.id)
    }

    @Test
    fun `save button invokes the save callback`() {
        var saved = false
        setContent(
            NoteFormState(contactId = 5, content = "Loves climbing"),
            onSave = { saved = true }
        )
        composeTestRule.onNodeWithText("Create note").performClick()
        assertEquals(true, saved)
    }

    // Issue #1404: the primary action is pinned in the Scaffold's bottomBar, so it
    // is on screen without scrolling and stays there as the form scrolls.
    @Test
    @Config(qualifiers = "w360dp-h400dp")
    fun `save button stays displayed without scrolling and while the form scrolls`() {
        var saved = false
        setContent(
            NoteFormState(contactId = 5, content = "Loves climbing"),
            onSave = { saved = true }
        )

        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeUp() }
        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput {
            swipeDown()
            swipeDown()
        }
        composeTestRule.onNodeWithText("Create note").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create note").performClick()
        assertEquals(true, saved)
    }

    // Issue #1773: with a contact already assigned, no empty search box sits under
    // it; Change reveals the search, and picking a result hides it again.
    @Test
    fun `an assigned contact hides the search until Change is tapped`() {
        setContent(
            NoteFormState(contactId = 5, targetContactId = 9, targetContactName = "Dana Lee")
        )
        composeTestRule.onNodeWithText("Dana Lee").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Search contacts").assertDoesNotExist()
        composeTestRule.onNodeWithText("Change").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Search contacts").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `an unassigned note shows the search straight away`() {
        setContent(NoteFormState(contactId = 5))
        composeTestRule.onNodeWithText("Change").assertDoesNotExist()
        composeTestRule.onNodeWithText("Search contacts").performScrollTo().assertIsDisplayed()
    }

    // Issue #2518: a saved note can be shared as a standing grant (keyed by its uuid).
    @Test
    fun `an existing note offers a standing share`() {
        var shared = 0
        setContent(
            NoteFormState(contactId = 5, noteId = 3, uuid = "n-uuid"),
            onShare = { shared += 1 }
        )
        composeTestRule.onNodeWithText(
            "Share this note"
        ).performScrollTo().assertIsDisplayed().performClick()
        assertEquals(1, shared)
    }

    @Test
    fun `a note that is not saved yet offers no standing share`() {
        setContent(NoteFormState(contactId = 5), onShare = {})
        composeTestRule.onNodeWithText("Share this note").assertDoesNotExist()
    }

    @Test
    fun `no share callback means no standing share entry`() {
        setContent(NoteFormState(contactId = 5, noteId = 3, uuid = "n-uuid"))
        composeTestRule.onNodeWithText("Share this note").assertDoesNotExist()
    }
}
