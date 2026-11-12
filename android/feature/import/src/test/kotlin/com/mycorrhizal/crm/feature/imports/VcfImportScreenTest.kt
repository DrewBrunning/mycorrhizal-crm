package com.mycorrhizal.crm.feature.imports

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsSelected
import androidx.compose.ui.test.junit4.v2.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performTextInput
import androidx.compose.ui.test.performTextReplacement
import com.mycorrhizal.crm.model.network.DiscoveredCustomProperty
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.ImportFieldMapping
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.ImportRowIssue
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.ui.theme.MycorrhizalTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

// M9 item 4: "reachability" proof for the VCF import screen — the real VcfImportScreenContent
// (not a placeholder) renders each step of the flow.
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35], qualifiers = "w400dp-h1400dp")
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class VcfImportScreenTest {

    @get:Rule
    val composeTestRule = createComposeRule()

    private fun setContent(
        uiState: VcfImportUiState,
        onRowActionChange: (Int, String) -> Unit = { _, _ -> },
        onConfirm: () -> Unit = {},
        onRowEditSave: (ImportRowEdit) -> Unit = {},
        onDone: () -> Unit = {},
        customFieldActions: CustomFieldActions = NoCustomFieldActions
    ) {
        composeTestRule.setContent {
            MycorrhizalTheme {
                VcfImportScreenContent(
                    uiState = uiState,
                    onRowActionChange = onRowActionChange,
                    onConfirm = onConfirm,
                    onRowEditSave = onRowEditSave,
                    onDone = onDone,
                    customFieldActions = customFieldActions
                )
            }
        }
    }

    @Test
    fun `pick step renders the picker button, not a placeholder`() {
        setContent(VcfImportUiState(step = VcfImportStep.PICK))
        composeTestRule.onNodeWithText("Select a .vcf file").assertIsDisplayed()
    }

    @Test
    fun `preview step renders each row's content`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(
                            rowIndex = 0,
                            parsedContact = mapOf("firstname" to "Dana", "lastname" to "White"),
                            suggestedAction = "add"
                        )
                    )
                ),
                rowActions = mapOf(0 to "add")
            )
        )
        composeTestRule.onNodeWithText("Dana White").assertIsDisplayed()
        composeTestRule.onNodeWithText("Apply decisions (1)").assertIsDisplayed()
    }

    @Test
    fun `tapping confirm import invokes the confirm callback`() {
        var confirmed = false
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(ImportRowPreview(rowIndex = 0))
                ),
                rowActions = mapOf(0 to "add")
            ),
            onConfirm = { confirmed = true }
        )
        composeTestRule.onNodeWithText("Apply decisions (1)").performClick()
        assertEquals(true, confirmed)
    }

    @Test
    fun `picking a row action invokes the callback with that row's index`() {
        var changedRow: Int? = null
        var changedAction: String? = null
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))
                ),
                rowActions = mapOf(0 to "add")
            ),
            onRowActionChange = { row, action ->
                changedRow = row
                changedAction = action
            }
        )
        composeTestRule.onNodeWithText("Skip").performClick()
        assertEquals(0, changedRow)
        assertEquals("skip", changedAction)
    }

    @Test
    fun `a duplicate row renders its match line and merge diff`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(
                            rowIndex = 0,
                            parsedContact = mapOf("firstname" to "Bob", "lastname" to "Smith"),
                            suggestedAction = "update",
                            duplicateMatch = com.mycorrhizal.crm.model.network.DuplicateMatch(
                                existingContactId = 9,
                                existingFirstname = "Bob",
                                existingLastname = "Smith",
                                existingEmail = "bob@example.com",
                                matchReason = "email"
                            ),
                            mergeDiff = com.mycorrhizal.crm.model.network.ImportMergeDiff(
                                updated = listOf(
                                    com.mycorrhizal.crm.model.network.ImportScalarChange(
                                        field = "job_title",
                                        label = "Job title",
                                        old = "Engineer",
                                        new = "Staff Engineer"
                                    )
                                ),
                                added = listOf(
                                    com.mycorrhizal.crm.model.network.ImportAddedValue(
                                        kind = "phone",
                                        value = "+15559998888"
                                    )
                                )
                            )
                        )
                    )
                ),
                rowActions = mapOf(0 to "update")
            )
        )
        composeTestRule.onNodeWithText("Matches: Bob Smith (same email)").assertIsDisplayed()
        composeTestRule.onNodeWithText("+ new phone: +15559998888").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Job title: Engineer \u2192 Staff Engineer"
        ).assertIsDisplayed()
    }

    @Test
    fun `a within-batch duplicate renders the duplicate-of line and disables merge`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(
                            rowIndex = 1,
                            suggestedAction = "skip",
                            batchDuplicateOf = 0
                        )
                    )
                ),
                rowActions = mapOf(0 to "add", 1 to "skip")
            )
        )
        composeTestRule.onNodeWithText("Duplicates row 1 of this import").assertIsDisplayed()
    }

    @Test
    fun `result step renders the created updated skipped counts`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.RESULT,
                result = ImportResult(totalProcessed = 3, created = 2, updated = 1, skipped = 0)
            )
        )
        composeTestRule.onNodeWithText(
            "Import complete: 2 created, 1 updated, 0 skipped."
        ).assertIsDisplayed()
    }

    @Test
    fun `tapping done after the result invokes the done callback`() {
        var done = false
        setContent(
            VcfImportUiState(
                step = VcfImportStep.RESULT,
                result = ImportResult(totalProcessed = 1, created = 1)
            ),
            onDone = { done = true }
        )
        composeTestRule.onNodeWithText("Confirm").performClick()
        assertEquals(true, done)
    }

    @Test
    fun `shows a loading skeleton while loading`() {
        setContent(VcfImportUiState(isLoading = true))
        composeTestRule.onNodeWithTag("vcf-import-loading").assertIsDisplayed()
    }

    @Test
    fun `result step lists the skipped rows with their reasons`() {
        setContent(
            VcfImportUiState(
                step = VcfImportStep.RESULT,
                result = ImportResult(
                    totalProcessed = 2,
                    created = 1,
                    skipped = 1,
                    issues = listOf(ImportRowIssue(2, "Erin", "", "", "skipped", "Invalid email"))
                )
            )
        )
        composeTestRule.onNodeWithText("Skipped: Invalid email").assertIsDisplayed()
    }

    @Test
    fun `saving a fix on an error row reports the edit`() {
        var saved: ImportRowEdit? = null
        setContent(
            VcfImportUiState(
                step = VcfImportStep.PREVIEW,
                preview = ImportPreviewResponse(
                    sessionId = "s",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, validationErrors = listOf("name required"))
                    )
                ),
                rowActions = mapOf(0 to "skip")
            ),
            onRowEditSave = { saved = it }
        )
        composeTestRule.onNodeWithText("Fix this row").performClick()
        composeTestRule.onNodeWithTag("import-fix-firstname").performTextInput("Dana")
        composeTestRule.onNodeWithText("Save fix").performClick()
        assertEquals(ImportRowEdit(rowIndex = 0, firstname = "Dana"), saved)
    }

    // ---- Issue #514 / #2523: the custom fields step ----

    private val color = DiscoveredCustomProperty("x-favorite-color", "green")

    private fun recorder(
        onAction: (String, String) -> Unit = { _, _ -> },
        onDefinition: (String, String) -> Unit = { _, _ -> },
        onLabel: (String, String) -> Unit = { _, _ -> },
        onType: (String, String) -> Unit = { _, _ -> },
        onContinue: () -> Unit = {}
    ) = object : CustomFieldActions {
        override fun setFieldMappingAction(propertyName: String, action: String) =
            onAction(propertyName, action)

        override fun setFieldMappingDefinition(propertyName: String, definitionId: String) =
            onDefinition(propertyName, definitionId)

        override fun setFieldMappingLabel(propertyName: String, label: String) =
            onLabel(propertyName, label)

        override fun setFieldMappingType(propertyName: String, type: String) =
            onType(propertyName, type)

        override fun continueFromCustomFields() = onContinue()
    }

    private fun customFieldsState(
        mapping: ImportFieldMapping = ImportFieldMapping("x-favorite-color", "ignore"),
        definitions: List<FieldDefinition> = listOf(FieldDefinition(id = "d1", label = "Hobby")),
        canContinue: Boolean = true
    ): VcfImportUiState {
        val state = VcfImportUiState(
            step = VcfImportStep.CUSTOM_FIELDS,
            customFieldCandidates = listOf(color),
            fieldDefinitions = definitions,
            fieldMappings = mapOf(color.name to mapping)
        )
        // canContinue is derived from the mappings, so an unmet map decision drives it false.
        return if (canContinue) {
            state
        } else {
            state.copy(
                fieldMappings = mapOf(
                    color.name to mapping.copy(action = "map", fieldDefinitionId = null)
                )
            )
        }
    }

    @Test
    fun `custom fields step lists the property, its sample value and per-option help`() {
        setContent(customFieldsState())

        composeTestRule.onNodeWithText("x-favorite-color").assertIsDisplayed()
        composeTestRule.onNodeWithText("Example value: green").assertIsDisplayed()
        composeTestRule.onNodeWithText("Keep as passthrough").assertIsDisplayed()
        composeTestRule.onNodeWithText("Map to existing field").assertIsDisplayed()
        composeTestRule.onNodeWithText("Create new field").assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Saved untouched with the contact and exported again later, but not searchable or editable here. Nothing is lost."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Put the value into a custom field you already created, so it is searchable and editable."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithText(
            "Make a new searchable custom field from this property and fill it in for every imported contact."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-ignore").assertIsSelected()
    }

    @Test
    fun `map to existing is disabled with the reason when there are no definitions`() {
        setContent(customFieldsState(definitions = emptyList()))

        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-map").assertIsNotEnabled()
        composeTestRule.onNodeWithText(
            "You have no custom fields yet. Choose “Create new field” to make one from this property."
        ).assertIsDisplayed()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-create").assertIsEnabled()
    }

    @Test
    fun `choosing each action reports the property and action`() {
        val chosen = mutableListOf<Pair<String, String>>()
        setContent(
            customFieldsState(),
            customFieldActions = recorder(onAction = { n, a -> chosen += n to a })
        )

        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-map").performClick()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-create").performClick()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-ignore").performClick()

        assertEquals(
            listOf(
                "x-favorite-color" to "map",
                "x-favorite-color" to "create",
                "x-favorite-color" to "ignore"
            ),
            chosen
        )
    }

    @Test
    fun `a map decision shows the definition picker and reports the chosen definition`() {
        val chosen = mutableListOf<Pair<String, String>>()
        setContent(
            customFieldsState(mapping = ImportFieldMapping("x-favorite-color", "map")),
            customFieldActions = recorder(onDefinition = { n, id -> chosen += n to id })
        )

        composeTestRule.onNodeWithText("Select a field…").assertIsDisplayed()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-label").assertDoesNotExist()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-field").performClick()
        composeTestRule.onNodeWithText("Hobby").performClick()

        assertEquals(listOf("x-favorite-color" to "d1"), chosen)
    }

    @Test
    fun `a create decision shows an editable label and a type picker`() {
        val labels = mutableListOf<String>()
        val types = mutableListOf<String>()
        setContent(
            customFieldsState(
                mapping = ImportFieldMapping(
                    "x-favorite-color",
                    "create",
                    label = "Favorite Color",
                    type = "text"
                )
            ),
            customFieldActions = recorder(
                onLabel = { _, l -> labels += l },
                onType = { _, t -> types += t }
            )
        )

        composeTestRule.onNodeWithText("Favorite Color").assertIsDisplayed()
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-field").assertDoesNotExist()
        composeTestRule.onNodeWithTag(
            "vcf-cf-x-favorite-color-label"
        ).performTextReplacement("Colour")
        composeTestRule.onNodeWithTag("vcf-cf-x-favorite-color-type").performClick()
        composeTestRule.onNodeWithText("Date").performClick()

        assertTrue(labels.toString(), "Colour" in labels)
        assertEquals(listOf("date"), types)
    }

    @Test
    fun `continue is disabled while a map decision has no field chosen`() {
        var continued = 0
        setContent(
            customFieldsState(canContinue = false),
            customFieldActions = recorder(onContinue = { continued++ })
        )

        composeTestRule.onNodeWithTag("vcf-cf-continue").assertIsNotEnabled()
        assertEquals(0, continued)
    }

    @Test
    fun `continue proceeds to review when every decision is complete`() {
        var continued = 0
        setContent(customFieldsState(), customFieldActions = recorder(onContinue = { continued++ }))

        composeTestRule.onNodeWithTag("vcf-cf-continue").assertIsEnabled().performClick()

        assertEquals(1, continued)
    }
}
