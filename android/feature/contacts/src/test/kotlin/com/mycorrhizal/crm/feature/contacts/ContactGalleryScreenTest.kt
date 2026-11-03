package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.PhotoSource
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactGalleryScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val own = ContactPhoto(id = 7, thumbnailUrl = "/thumb", width = 800, height = 600)
    private val fromNote = ContactPhoto(
        id = 3,
        source = PhotoSource(PhotoSource.KIND_NOTE, 8, "Lunch"),
        sensitivity = "private",
        thumbnailUrl = "/note-thumb"
    )

    private var added = 0
    private var deleted: ContactPhoto? = null
    private var mainSet: ContactPhoto? = null
    private var mainCleared = 0
    private var adopted: ContactPhoto? = null
    private var cropped: Pair<ContactPhoto, ImageCrop?>? = null

    private fun setContent(uiState: ContactGalleryUiState) {
        added = 0
        deleted = null
        mainSet = null
        mainCleared = 0
        adopted = null
        cropped = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactGalleryScreenContent(
                    uiState = uiState,
                    onAddClick = { added += 1 },
                    onDelete = { deleted = it },
                    onSetMain = { mainSet = it },
                    onClearMain = { mainCleared += 1 },
                    onAdopt = { adopted = it },
                    onCrop = { photo, crop -> cropped = photo to crop }
                )
            }
        }
    }

    private fun openMenu(tag: String) {
        composeTestRule.onNodeWithTag(tag).performClick()
    }

    @Test
    fun `empty gallery shows the upload prompt`() {
        setContent(ContactGalleryUiState(isLoading = false))
        composeTestRule.onNodeWithTag("contact-gallery-empty").assertIsDisplayed()
        composeTestRule.onNodeWithText("No photos yet").assertIsDisplayed()
        composeTestRule.onNodeWithText("Add photos").performClick()
        assertEquals(1, added)
    }

    @Test
    fun `the fab opens the picker`() {
        setContent(ContactGalleryUiState(photos = listOf(own)))
        composeTestRule.onNodeWithTag("add-contact-image").performClick()
        assertEquals(1, added)
    }

    @Test
    fun `populated gallery shows tiles with source and sensitivity labels`() {
        setContent(ContactGalleryUiState(photos = listOf(own, fromNote)))
        composeTestRule.onNodeWithTag("contact-photo-contact-7").assertIsDisplayed()
        composeTestRule.onNodeWithTag("contact-photo-note-3").assertIsDisplayed()
        composeTestRule.onNodeWithText("Note: Lunch").assertIsDisplayed()
        composeTestRule.onNodeWithText("Private").assertIsDisplayed()
    }

    @Test
    fun `the main photo carries a badge`() {
        setContent(ContactGalleryUiState(photos = listOf(own), mainImageId = 7))
        composeTestRule.onNodeWithTag("main-badge-contact-7", useUnmergedTree = true)
            .assertIsDisplayed()
    }

    @Test
    fun `a note photo is not badged main even when its id matches the pointer`() {
        val noteWithSameId = fromNote.copy(id = 7)
        setContent(ContactGalleryUiState(photos = listOf(noteWithSameId), mainImageId = 7))
        composeTestRule.onNodeWithTag("main-badge-note-7", useUnmergedTree = true)
            .assertDoesNotExist()
    }

    @Test
    fun `delete asks for confirmation then fires`() {
        setContent(ContactGalleryUiState(photos = listOf(own)))
        openMenu("photo-menu-contact-7")
        composeTestRule.onNodeWithText("Delete photo").performClick()
        composeTestRule.onNodeWithText("Delete photo?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()
        assertEquals(7, deleted?.id)
    }

    @Test
    fun `set as main fires for an own photo`() {
        setContent(ContactGalleryUiState(photos = listOf(own)))
        openMenu("photo-menu-contact-7")
        composeTestRule.onNodeWithText("Set as main photo").performClick()
        assertEquals(7, mainSet?.id)
    }

    @Test
    fun `the main photo offers remove instead of set`() {
        setContent(ContactGalleryUiState(photos = listOf(own), mainImageId = 7))
        openMenu("photo-menu-contact-7")
        composeTestRule.onNodeWithText("Set as main photo").assertDoesNotExist()
        composeTestRule.onNodeWithText("Remove as main photo").performClick()
        assertEquals(1, mainCleared)
    }

    @Test
    fun `a note photo offers adopt only`() {
        setContent(ContactGalleryUiState(photos = listOf(fromNote)))
        openMenu("photo-menu-note-3")
        composeTestRule.onNodeWithText("Set as main photo").assertDoesNotExist()
        composeTestRule.onNodeWithText("Crop for profile photo").assertDoesNotExist()
        composeTestRule.onNodeWithText("Delete photo").assertDoesNotExist()
        composeTestRule.onNodeWithText("Add to contact photos").performClick()
        assertEquals(3, adopted?.id)
    }

    @Test
    fun `crop dialog saves a centred square crop`() {
        setContent(ContactGalleryUiState(photos = listOf(own)))
        openMenu("photo-menu-contact-7")
        composeTestRule.onNodeWithText("Crop for profile photo").performClick()
        composeTestRule.onNodeWithText("Crop photo").assertIsDisplayed()
        composeTestRule.onNodeWithText("Save crop").performClick()
        val (photo, crop) = cropped!!
        assertEquals(7, photo.id)
        assertNotNull(crop)
        // 800x600 at the default full zoom and centre: a 600px square, centred horizontally.
        assertEquals(0.125, crop!!.x, 1e-6)
        assertEquals(0.75, crop.w, 1e-6)
        assertEquals(1.0, crop.h, 1e-6)
    }

    @Test
    fun `crop dialog can clear the crop`() {
        setContent(ContactGalleryUiState(photos = listOf(own)))
        openMenu("photo-menu-contact-7")
        composeTestRule.onNodeWithText("Crop for profile photo").performClick()
        composeTestRule.onNodeWithText("Use whole photo").performScrollTo().performClick()
        assertEquals(7, cropped?.first?.id)
        assertNull(cropped?.second)
    }
}
