package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
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

    private val image = ContactImage(id = 7, contactId = 5, thumbnailUrl = "/thumb")

    private var added = 0
    private var deleted: ContactImage? = null

    private fun setContent(uiState: ContactGalleryUiState) {
        added = 0
        deleted = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactGalleryScreenContent(
                    uiState = uiState,
                    onAddClick = { added += 1 },
                    onDelete = { deleted = it }
                )
            }
        }
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
        setContent(ContactGalleryUiState(images = listOf(image)))
        composeTestRule.onNodeWithTag("add-contact-image").performClick()
        assertEquals(1, added)
    }

    @Test
    fun `populated gallery shows a tile`() {
        setContent(ContactGalleryUiState(images = listOf(image)))
        composeTestRule.onNodeWithTag("contact-image-7").assertIsDisplayed()
    }

    @Test
    fun `delete asks for confirmation then fires`() {
        setContent(ContactGalleryUiState(images = listOf(image)))
        composeTestRule.onNodeWithTag("delete-contact-image-7").performClick()
        composeTestRule.onNodeWithText("Delete photo?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()
        assertEquals(7, deleted?.id)
    }
}
