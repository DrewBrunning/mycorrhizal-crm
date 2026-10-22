package com.mycorrhizal.crm.feature.settings

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.SnackbarHostState
import androidx.compose.runtime.remember
import androidx.compose.ui.Modifier
import com.mycorrhizal.crm.ui.components.FormScaffold
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.hasScrollAction
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTouchInput
import androidx.compose.ui.test.swipeDown
import androidx.compose.ui.test.swipeUp
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import io.mockk.coEvery
import io.mockk.coVerify
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

    private fun setContent(
        state: FieldDefinitionFormState,
        onTypeChange: (String) -> Unit = {},
        onSave: () -> Unit = {},
        onGroupChange: (String) -> Unit = {},
        onDelete: () -> Unit = {},
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                FormScaffold(
                    title = "Form",
                    onBack = {},
                    saveLabel = if (state.isEdit) "Save" else "Create",
                    isSaving = state.isSaving,
                    onSave = onSave,
                    snackbarHostState = remember { SnackbarHostState() },
                ) { padding ->
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
                        onGroupChange = onGroupChange,
                        onDelete = onDelete,
                        modifier = Modifier.padding(padding),
                    )
                }
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
        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
    }

    @Test
    fun `save button shows Save in edit mode`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = "d1"))
        composeTestRule.onNodeWithText("Save").assertIsDisplayed()
    }

    // Issue #1404: the primary action is pinned in the Scaffold's bottomBar, so it
    // is on screen without scrolling and stays there as the form scrolls.
    @Test
    @Config(qualifiers = "w360dp-h400dp")
    fun `save button stays displayed without scrolling and while the form scrolls`() {
        var saved = false
        setContent(FieldDefinitionFormState(type = "string"), onSave = { saved = true })

        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeUp() }
        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
        composeTestRule.onNode(hasScrollAction()).performTouchInput { swipeDown(); swipeDown() }
        composeTestRule.onNodeWithText("Create").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create").performClick()
        assertEquals(true, saved)
    }

    @Test
    fun `save button is disabled while saving`() {
        setContent(FieldDefinitionFormState(isSaving = true))

        composeTestRule.onNodeWithText("Create").assertIsNotEnabled()
    }

    @Test
    fun `choosing a type from the dropdown invokes onTypeChange`() {
        var chosen: String? = null
        setContent(FieldDefinitionFormState(type = "string"), onTypeChange = { chosen = it })

        composeTestRule.onNodeWithText("Text (short)").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Number").performClick()

        assertEquals("number", chosen)
    }

    @Test
    fun `advanced section hides the vCard projection fields until opened`() {
        setContent(FieldDefinitionFormState(projectionMode = "internal"))

        composeTestRule.onNodeWithText("vCard property name").assertDoesNotExist()
        composeTestRule.onNodeWithText("Advanced").performScrollTo().performClick()
        composeTestRule.onNodeWithText("vCard property name").assertDoesNotExist()
        composeTestRule.onNodeWithText("Standards projection").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `delete is shown in edit mode only`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = "d1"))
        composeTestRule.onNodeWithText("Delete custom field").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `delete is absent in create mode`() {
        setContent(FieldDefinitionFormState(fieldDefinitionId = null))
        composeTestRule.onNodeWithText("Delete custom field").assertDoesNotExist()
    }

    @Test
    fun `delete asks for confirmation before invoking onDelete`() {
        var deleted = false
        setContent(FieldDefinitionFormState(fieldDefinitionId = "d1", label = "Coffee order"), onDelete = { deleted = true })

        composeTestRule.onNodeWithText("Delete custom field").performScrollTo().performClick()
        assertEquals(false, deleted)

        composeTestRule.onNodeWithText("Delete custom field?").assertIsDisplayed()
        composeTestRule.onNodeWithText("Delete").performClick()
        assertEquals(true, deleted)
    }

    // --- The real top-level FieldDefinitionFormScreen (ViewModel + nav + snackbar wiring) ---

    private fun setScreen(
        fieldDefinitionId: String? = null,
        onBack: () -> Unit = {},
        onSaved: () -> Unit = {},
        onDeleted: () -> Unit = onSaved,
    ): FieldDefinitionRepository {
        val repository = mockk<FieldDefinitionRepository>()
        coEvery { repository.list(any()) } returns Result.success(emptyList())
        if (fieldDefinitionId != null) {
            coEvery { repository.get(fieldDefinitionId) } returns Result.success(
                FieldDefinition(id = fieldDefinitionId, label = "Coffee order", key = "coffee_order", type = "string"),
            )
        }
        coEvery { repository.delete(any()) } returns Result.success(Unit)
        val savedStateHandle = if (fieldDefinitionId != null) {
            SavedStateHandle(mapOf("fieldDefinitionId" to fieldDefinitionId))
        } else {
            SavedStateHandle()
        }
        val viewModel = FieldDefinitionFormViewModel(repository, savedStateHandle)
        composeTestRule.setContent {
            MycorrhizalTheme {
                FieldDefinitionFormScreen(
                    onSaved = onSaved,
                    onBack = onBack,
                    onDeleted = onDeleted,
                    viewModel = viewModel,
                )
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
        composeTestRule.onNodeWithText("Create").performClick()
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
        composeTestRule.onNodeWithText("Create").performClick()

        composeTestRule.onNodeWithText("already exists").assertIsDisplayed()
        assertEquals(false, savedCalled)
    }

    // --- Issue #1731: group field ---

    @Test
    fun `the group field is shown and prefilled`() {
        setContent(FieldDefinitionFormState(group = "Travel profile"))

        composeTestRule.onNodeWithText("Group").performScrollTo().assertIsDisplayed()
        composeTestRule.onNodeWithText("Travel profile").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun `typing in the group field reports the new text`() {
        var typed: String? = null
        setContent(FieldDefinitionFormState(group = ""), onGroupChange = { typed = it })

        composeTestRule.onNodeWithText("Group").performScrollTo().performTextInput("Health")

        assertEquals("Health", typed)
    }

    @Test
    fun `matching existing groups are suggested and picking one fills the field`() {
        var picked: String? = null
        setContent(
            FieldDefinitionFormState(group = "Tra", groupSuggestions = listOf("Health", "Travel profile")),
            onGroupChange = { picked = it },
        )

        composeTestRule.onNodeWithText("Tra").performScrollTo().performTextInput("v")
        composeTestRule.onNodeWithText("Health").assertDoesNotExist()
        composeTestRule.onNodeWithText("Travel profile").performClick()

        assertEquals("Travel profile", picked)
    }

    @Test
    fun `deleting from the edit screen calls the repository and invokes onDeleted`() {
        var deletedCalled = false
        val repository = setScreen(fieldDefinitionId = "d1", onDeleted = { deletedCalled = true })

        composeTestRule.onNodeWithText("Delete custom field").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Delete").performClick()
        composeTestRule.waitForIdle()

        coVerify { repository.delete("d1") }
        assertEquals(true, deletedCalled)
    }

    @Test
    fun `a delete failure surfaces the error and stays on the form`() {
        var deletedCalled = false
        val repository = setScreen(fieldDefinitionId = "d1", onDeleted = { deletedCalled = true })
        coEvery { repository.delete("d1") } returns Result.failure(ApiError.Client(500, "delete failed"))

        composeTestRule.onNodeWithText("Delete custom field").performScrollTo().performClick()
        composeTestRule.onNodeWithText("Delete").performClick()

        composeTestRule.onNodeWithText("delete failed").assertIsDisplayed()
        assertEquals(false, deletedCalled)
    }
}
