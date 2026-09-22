package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.SemanticsProperties
import androidx.compose.ui.test.SemanticsMatcher
import androidx.compose.ui.test.assert
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertIsToggleable
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
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
class ContactFieldSettingsScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    @Test
    fun `a default-enabled field shows its switch on`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(
                    state = ContactFieldSettingsUiState(isLoading = false, enabled = DEFAULT_ENABLED_CONTACT_FIELDS),
                )
            }
        }

        composeTestRule.onNodeWithText("Email").performScrollTo().assertIsToggleable().assertIsOn()
    }

    @Test
    fun `an opt-in field not yet enabled shows its switch off`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(
                    state = ContactFieldSettingsUiState(isLoading = false, enabled = DEFAULT_ENABLED_CONTACT_FIELDS),
                )
            }
        }

        composeTestRule.onNodeWithText("Keywords").performScrollTo().assertIsToggleable().assertIsOff()
    }

    @Test
    fun `tapping a field's row invokes onToggle with that key`() {
        var toggled: ContactFieldKey? = null
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(
                    state = ContactFieldSettingsUiState(isLoading = false, enabled = DEFAULT_ENABLED_CONTACT_FIELDS),
                    onToggle = { toggled = it },
                )
            }
        }

        composeTestRule.onNodeWithText("Keywords").performScrollTo().performClick()
        assertEquals(ContactFieldKey.KEYWORDS, toggled)
    }

    @Test
    fun `group headers are shown`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(state = ContactFieldSettingsUiState(isLoading = false))
            }
        }

        composeTestRule.onNodeWithText("Communication").assertIsDisplayed()
        composeTestRule.onNodeWithText("Personal").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `every switch is disabled while a save is in flight`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(
                    state = ContactFieldSettingsUiState(
                        isLoading = false,
                        enabled = DEFAULT_ENABLED_CONTACT_FIELDS,
                        savingKey = ContactFieldKey.EMAILS,
                    ),
                )
            }
        }

        composeTestRule.onNodeWithText("Keywords").performScrollTo().assertIsToggleable()
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.Disabled, Unit))
    }

    @Test
    fun `save error is announced as an assertive live region`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(
                    state = ContactFieldSettingsUiState(isLoading = false, saveError = "Could not save"),
                )
            }
        }

        composeTestRule.onNodeWithText("Could not save")
            .assert(SemanticsMatcher.expectValue(SemanticsProperties.LiveRegion, LiveRegionMode.Assertive))
    }

    @Test
    fun `loading state shows no field rows`() {
        composeTestRule.setContent {
            MycorrhizalTheme {
                ContactFieldSettingsContent(state = ContactFieldSettingsUiState(isLoading = true))
            }
        }

        composeTestRule.onNodeWithText("Email").assertDoesNotExist()
    }
}
