package com.mycorrhizal.crm.feature.timeline

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ActivityImage
import com.mycorrhizal.crm.model.network.NoteImage
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// #2369 Android parity (ADR 0040 decision 8): note and activity photos show a
// private/secret badge wherever they appear, and the editable viewer sets the
// sensitivity, as the web gallery does.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h1600dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class OwnerImageSensitivityTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val changes = mutableListOf<Pair<Int, String>>()

    private fun setNoteSection(images: List<NoteImage>, failedId: Int? = null) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NoteImageSection(
                    images = images,
                    pendingCount = 0,
                    loading = false,
                    busy = false,
                    onAdd = {},
                    onDelete = {},
                    onSetSensitivity = { id, value -> changes += id to value },
                    sensitivityFailedImageId = failedId
                )
            }
        }
    }

    private fun openViewer() {
        composeTestRule.onAllNodesWithContentDescription("Note photo")[0].performClick()
    }

    @Test
    fun `an editable note photo's viewer sets its sensitivity`() {
        setNoteSection(listOf(NoteImage(id = 9, noteId = 3, sensitivity = "normal")))
        openViewer()

        composeTestRule.onNodeWithTag("image-sensitivity-normal").assertIsSelected()
        composeTestRule.onNodeWithTag("image-sensitivity-secret").performClick()

        assertEquals(listOf(9 to "secret"), changes)
    }

    @Test
    fun `choosing the current sensitivity sends nothing`() {
        setNoteSection(listOf(NoteImage(id = 9, noteId = 3, sensitivity = "private")))
        openViewer()

        composeTestRule.onNodeWithTag("image-sensitivity-private").assertIsSelected()
        composeTestRule.onNodeWithTag("image-sensitivity-private").performClick()

        assertEquals(emptyList<Pair<Int, String>>(), changes)
    }

    @Test
    fun `a failed change shows the error in the viewer`() {
        setNoteSection(listOf(NoteImage(id = 9, noteId = 3)), failedId = 9)
        openViewer()

        composeTestRule.onNodeWithText(
            "Could not change the photo's sensitivity."
        ).assertIsDisplayed()
    }

    @Test
    fun `a private photo is badged in the editor`() {
        setNoteSection(listOf(NoteImage(id = 9, noteId = 3, sensitivity = "private")))

        composeTestRule.onNodeWithText("Private").assertIsDisplayed()
    }

    @Test
    fun `a read-only note row badges a secret photo and offers no control`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                NoteImageRow(listOf(NoteImage(id = 9, noteId = 3, sensitivity = "secret")))
            }
        }
        composeTestRule.onNodeWithText("Secret").assertIsDisplayed()

        openViewer()
        composeTestRule.onNodeWithTag("image-sensitivity-normal").assertDoesNotExist()
    }

    @Test
    fun `an editable activity photo's viewer sets its sensitivity`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ActivityImageSection(
                    images = listOf(ActivityImage(id = 5, activityId = 2, sensitivity = "secret")),
                    pendingCount = 0,
                    loading = false,
                    busy = false,
                    onAdd = {},
                    onDelete = {},
                    onSetSensitivity = { id, value -> changes += id to value },
                    sensitivityFailedImageId = null
                )
            }
        }
        composeTestRule.onAllNodesWithContentDescription("Activity photo")[0].performClick()
        composeTestRule.onNodeWithTag("image-sensitivity-normal").performClick()

        assertEquals(listOf(5 to "normal"), changes)
    }
}
