package com.mycorrhizal.crm.feature.contacts

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import com.mycorrhizal.crm.model.network.ContactFieldVisibility
import com.mycorrhizal.crm.model.network.ContactFieldVisibilityInput
import com.mycorrhizal.crm.model.network.ResolvedFieldVisibility
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w480dp-h2000dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactFieldVisibilityDialogTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private val visibility = ContactFieldVisibility(
        overrides = mapOf("emails" to "hide"),
        hideEmpty = "show",
        defaults = ResolvedFieldVisibility(
            fields = mapOf("emails" to true),
            hideEmpty = true
        ),
        resolved = ResolvedFieldVisibility(
            fields = mapOf("emails" to false),
            hideEmpty = false
        )
    )

    @Test
    fun `saving sends the seeded overrides and hide-empty state`() {
        var saved: ContactFieldVisibilityInput? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldVisibilityDialog(
                    visibility = visibility,
                    saving = false,
                    error = null,
                    onDismiss = {},
                    onSave = { saved = it },
                    onReset = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Customize fields for this contact").assertIsDisplayed()
        composeTestRule.onNodeWithText("Save").performClick()

        assertEquals("hide", saved?.overrides?.get("emails"))
        assertEquals("show", saved?.hideEmpty)
    }

    @Test
    fun `reset fires the reset callback`() {
        var resets = 0
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldVisibilityDialog(
                    visibility = visibility,
                    saving = false,
                    error = null,
                    onDismiss = {},
                    onSave = {},
                    onReset = { resets += 1 }
                )
            }
        }

        composeTestRule.onNodeWithText("Reset to default").performClick()

        assertEquals(1, resets)
    }

    @Test
    fun `a save error is shown in the dialog`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldVisibilityDialog(
                    visibility = visibility,
                    saving = false,
                    error = "Unable to save",
                    onDismiss = {},
                    onSave = {},
                    onReset = {}
                )
            }
        }

        composeTestRule.onNodeWithText("Unable to save").assertIsDisplayed()
    }
}
