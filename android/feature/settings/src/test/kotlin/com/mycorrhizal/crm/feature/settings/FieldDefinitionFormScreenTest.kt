package com.mycorrhizal.crm.feature.settings

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.mockk
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

/** Issue #830. Tests the stateless [FieldDefinitionFormContent] directly, mirroring
 *  `ImmichSettingsScreenTest`'s content-level style. */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class FieldDefinitionFormScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(state: FieldDefinitionFormState, onTypeChange: (String) -> Unit = {}) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FieldDefinitionFormContent(
                    state = state,
                    onLabelChange = {},
                    onKeyChange = {},
                    onTypeChange = onTypeChange,
                    onMultiChange = {},
                    onMinChange = {},
                    onMaxChange = {},
                    onMaxLengthChange = {},
                    onPatternChange = {},
                    onAddEnumValue = {},
                    onUpdateEnumValue = { _, _ -> },
                    onRemoveEnumValue = {},
                    onProjectionModeChange = {},
                    onVcardNameChange = {},
                    onSensitivityChange = {},
                    onSave = {},
                )
            }
        }
    }

    @Test
    fun `string type shows the max length and pattern fields`() {
        setContent(FieldDefinitionFormState(type = "string"))

        composeTestRule.onNodeWithText("Max length").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Pattern").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Minimum").assertDoesNotExist()
    }

    @Test
    fun `number type shows the min and max fields instead of string constraints`() {
        setContent(FieldDefinitionFormState(type = "number"))

        composeTestRule.onNodeWithText("Minimum").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Maximum").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Max length").assertDoesNotExist()
    }

    @Test
    fun `enum type shows the allowed-values list and add button`() {
        setContent(FieldDefinitionFormState(type = "enum", enumValues = listOf("Latte", "Espresso")))

        composeTestRule.onNodeWithText("Latte").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Espresso").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Add value").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `the key field is disabled when editing`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = "d1", key = "coffee_order"))

        composeTestRule.onNodeWithText("Key").performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun `the key field is enabled when creating`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = null))

        composeTestRule.onNodeWithText("Key").performScrollTo().assertIsEnabled()
    }

    @Test
    fun `the vcard name field is absent in internal projection mode`() {
        setContent(FieldDefinitionFormState(projectionMode = "internal"))
        composeTestRule.onNodeWithText("vCard property name").assertDoesNotExist()
    }

    @Test
    fun `the vcard name field appears in vcard projection mode`() {
        setContent(FieldDefinitionFormState(projectionMode = "vcard"))
        composeTestRule.onNodeWithText("vCard property name").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `save button shows Create in create mode`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = null))
        composeTestRule.onNodeWithText("Create").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `save button shows Save in edit mode`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = "d1"))
        composeTestRule.onNodeWithText("Save").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `save button is disabled while saving`() {
        setContent(FieldDefinitionFormState(isSaving = true))

        composeTestRule.onNodeWithText("Create").performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun `choosing a type from the dropdown invokes onTypeChange`() {
        var chosen: String? = null
        setContent(FieldDefinitionFormState(type = "string"), onTypeChange = { chosen = it })

        composeTestRule.onNodeWithText("Text (short)").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Number").performClick()

        assertEquals("number", chosen)
    }

    // --- The real top-level FieldDefinitionFormScreen (ViewModel + nav + snackbar wiring) ---

    private fun setScreen(
        fieldDefinitionId: String? = null,
        onBack: () -> Unit = {},
        onSaved: () -> Unit = {},
    ): FieldDefinitionRepository {
        val repository = mockk<FieldDefinitionRepository>()
        if (fieldDefinitionId != null) {
            coEvery { repository.get(fieldDefinitionId) } returns Result.success(
                FieldDefinition(id = fieldDefinitionId, label = "Coffee order", key = "coffee_order", type = "string"),
            )
        }
        val savedStateHandle = if (fieldDefinitionId != null) {
            SavedStateHandle(mapOf("fieldDefinitionId" to fieldDefinitionId))
        } else {
            SavedStateHandle()
        }
        val viewModel = FieldDefinitionFormViewModel(repository, savedStateHandle)
        composeTestRule.setContent {
            MycorrhizalTheme {
                FieldDefinitionFormScreen(onSaved = onSaved, onBack = onBack, viewModel = viewModel)
            }
        }
        return repository
    }

    @Test
    fun `the create screen shows the new-field title`() {
        setScreen()

        composeTestRule.onNodeWithText("New custom field").assertIsDisplayed()
    }

    @Test
    fun `the edit screen loads the existing definition and shows the edit title`() {
        setScreen(fieldDefinitionId = "d1")

        composeTestRule.onNodeWithText("Edit custom field").assertIsDisplayed()
        composeTestRule.onNodeWithText("Coffee order").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `tapping back invokes onBack`() {
        var backCalled = false
        setScreen(onBack = { backCalled = true })

        composeTestRule.onNodeWithContentDescription("Back").performClick()

        assertEquals(true, backCalled)
    }

    @Test
    fun `a successful save invokes onSaved`() {
        var savedCalled = false
        val repository = setScreen(onSaved = { savedCalled = true })
        coEvery { repository.create(any()) } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order", key = "coffee_order", type = "string"),
        )

        composeTestRule.onNodeWithText("Label").performScrollTo().performTextInput("Coffee order")
        composeTestRule.onNodeWithText("Key").performScrollTo().performTextInput("coffee_order")
        composeTestRule.onNodeWithText("Create").performScrollTo().performClick()
        composeTestRule.waitForIdle()

        assertEquals(true, savedCalled)
    }

    @Test
    fun `a save failure surfaces the error as a snackbar without invoking onSaved`() {
        var savedCalled = false
        val repository = setScreen(onSaved = { savedCalled = true })
        coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(409, "already exists"))

        composeTestRule.onNodeWithText("Label").performScrollTo().performTextInput("Coffee order")
        composeTestRule.onNodeWithText("Key").performScrollTo().performTextInput("coffee_order")
        composeTestRule.onNodeWithText("Create").performScrollTo().performClick()

        composeTestRule.onNodeWithText("already exists").assertIsDisplayed()
        assertEquals(false, savedCalled)
    }
}
