package com.mycorrhizal.crm.ui.components

import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #1751: the shared unsaved-changes guard on [FormScaffold]. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class FormScaffoldTest {

    @get:Rule
    val composeTestRule = createAndroidComposeRule<ComponentActivity>()

    private fun setContent(
        isDirty: Boolean,
        onBack: () -> Unit = {},
        onSave: () -> Unit = {},
        saveEnabled: Boolean = true,
        entityLabel: String = "contact",
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Edit contact",
                    onBack = onBack,
                    saveLabel = "Save changes",
                    isSaving = false,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() },
                    saveEnabled = saveEnabled,
                    isDirty = isDirty,
                    entityLabel = entityLabel,
                ) { padding ->
                    Text("Form body", modifier = Modifier.padding(padding))
                }
            }
        }
    }

    @Test
    fun `an untouched form leaves immediately from the up arrow`() {
        var back = false
        setContent(isDirty = false, onBack = { back = true })

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        assertTrue(back)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `a dirty form asks before the up arrow discards it, and keep editing preserves it`() {
        var back = false
        setContent(isDirty = true, onBack = { back = true })

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Your edits to this contact haven't been saved.").assertIsDisplayed()
        assertFalse(back)

        composeTestRule.onNodeWithText("Keep editing").performClick()

        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
        composeTestRule.onNodeWithText("Form body").assertIsDisplayed()
        assertFalse(back)
    }

    @Test
    fun `discard leaves the form`() {
        var back = false
        setContent(isDirty = true, onBack = { back = true })

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithText("Discard").performClick()

        assertTrue(back)
        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
    }

    @Test
    fun `system back on a dirty form shows the dialog instead of leaving`() {
        var back = false
        setContent(isDirty = true, onBack = { back = true })

        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }

        composeTestRule.onNodeWithText("Discard changes?").assertIsDisplayed()
        assertFalse(back)
    }

    @Test
    fun `system back on an untouched form does not show the dialog`() {
        var back = false
        setContent(isDirty = false, onBack = { back = true })

        composeTestRule.runOnUiThread {
            composeTestRule.activity.onBackPressedDispatcher.onBackPressed()
        }

        composeTestRule.onNodeWithText("Discard changes?").assertDoesNotExist()
        assertFalse(back)
    }

    @Test
    fun `the dialog names the entity being edited`() {
        setContent(isDirty = true, entityLabel = "note")

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        composeTestRule.onNodeWithText("Your edits to this note haven't been saved.").assertIsDisplayed()
    }

    @Test
    fun `the save action stays available while the discard dialog is open`() {
        var saved = 0
        setContent(isDirty = true, onSave = { saved++ })

        composeTestRule.onNodeWithContentDescription("Back").performClick()
        composeTestRule.onNodeWithText("Keep editing").performClick()
        composeTestRule.onNodeWithText("Save changes").performClick()

        assertEquals(1, saved)
    }
}
