package com.mycorrhizal.crm.feature.imports

import com.mycorrhizal.crm.model.network.DiscoveredCustomProperty
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionsResponse
import com.mycorrhizal.crm.model.network.ImportConfirmRequest
import com.mycorrhizal.crm.model.network.ImportFieldMapping
import com.mycorrhizal.crm.model.network.ImportPreviewResponse
import com.mycorrhizal.crm.model.network.ImportResult
import com.mycorrhizal.crm.model.network.ImportRowEdit
import com.mycorrhizal.crm.model.network.ImportRowPreview
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import com.mycorrhizal.crm.ui.R
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import io.mockk.slot
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

// M9 item 4: uploadVcfImport() existed with zero UI callers; this is its first real caller.
class VcfImportViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val apiClient = mockk<ApiClient>()

    // The primary size gate: the screen probes the provider's declared size and calls this
    // WITHOUT ever reading the file, so picking a multi-gigabyte file can't OOM the app.
    @Test
    fun `a file whose declared size is over 50MB is rejected before its bytes are read`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = VcfImportViewModel(apiClient)

            vm.onFileTooLarge()
            advanceUntilIdle()

            assertEquals(R.string.import_vcf_error_too_large, vm.uiState.value.errorRes)
            assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
            coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
        }

    // The backstop, for content providers that declare no size at all.
    @Test
    fun `a file over 50MB is rejected client-side without calling the API`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = VcfImportViewModel(apiClient)

        vm.onFilePicked("big.vcf", ByteArray(VcfImportViewModel.MAX_VCF_SIZE_BYTES + 1))
        advanceUntilIdle()

        assertEquals(R.string.import_vcf_error_too_large, vm.uiState.value.errorRes)
        assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
        coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
    }

    @Test
    fun `an empty file is rejected client-side without calling the API`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = VcfImportViewModel(apiClient)

        vm.onFilePicked("empty.vcf", ByteArray(0))
        advanceUntilIdle()

        assertEquals(R.string.import_vcf_error_invalid_file, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { apiClient.uploadVcfImport(any(), any()) }
    }

    @Test
    fun `a successful upload moves to the preview step defaulted to each row's suggested action`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), "contacts.vcf") } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "update")
                    )
                )
            )

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1, 2, 3))
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(VcfImportStep.PREVIEW, state.step)
            assertEquals("add", state.rowActions[0])
            assertEquals("update", state.rowActions[1])
        }

    @Test
    fun `a row with validation errors is forced to skip regardless of its suggested action`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(
                            rowIndex = 0,
                            suggestedAction = "add",
                            validationErrors = listOf("missing name")
                        )
                    )
                )
            )

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1))
            advanceUntilIdle()

            assertEquals("skip", vm.uiState.value.rowActions[0])

            // setRowAction is a no-op for an errored row — it stays forced to skip.
            vm.setRowAction(0, "add")
            assertEquals("skip", vm.uiState.value.rowActions[0])
        }

    @Test
    fun `a row without errors can have its action overridden`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
            ImportPreviewResponse(
                sessionId = "session-1",
                rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))
            )
        )

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()

        vm.setRowAction(0, "skip")

        assertEquals("skip", vm.uiState.value.rowActions[0])
    }

    @Test
    fun `resolveAll sets every valid row to its suggested action and leaves errored rows skipped`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "update"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "add"),
                        ImportRowPreview(
                            rowIndex = 2,
                            validationErrors = listOf("missing name"),
                            suggestedAction = "skip"
                        )
                    )
                )
            )

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1))
            advanceUntilIdle()

            // Flatten every row to skip, then resolve all.
            vm.setRowAction(0, "skip")
            vm.setRowAction(1, "skip")
            vm.resolveAll()
            advanceUntilIdle()

            val actions = vm.uiState.value.rowActions
            assertEquals("update", actions[0])
            assertEquals("add", actions[1])
            assertEquals("skip", actions[2])
        }

    @Test
    fun `upload failure surfaces the error and stays on the pick step`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery {
            apiClient.uploadVcfImport(
                any(),
                any()
            )
        } returns Result.failure(ApiError.Client(400, "bad vcf"))

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()

        assertEquals("bad vcf", vm.uiState.value.error?.message)
        assertEquals(VcfImportStep.PICK, vm.uiState.value.step)
    }

    @Test
    fun `confirm sends the session id and every row's chosen action, moving to the result step`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
                ImportPreviewResponse(
                    sessionId = "session-1",
                    rows = listOf(
                        ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
                        ImportRowPreview(rowIndex = 1, suggestedAction = "update")
                    )
                )
            )
            coEvery {
                apiClient.confirmVcfImport(
                    match { it.sessionId == "session-1" && it.actions.size == 2 }
                )
            } returns Result.success(ImportResult(totalProcessed = 2, created = 1, updated = 1))

            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("contacts.vcf", byteArrayOf(1))
            advanceUntilIdle()
            vm.setRowAction(1, "skip")
            vm.confirm()
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(VcfImportStep.RESULT, state.step)
            assertEquals(1, state.result?.created)
            coVerify {
                apiClient.confirmVcfImport(
                    match { request ->
                        request.actions.any { it.rowIndex == 0 && it.action == "add" } &&
                            request.actions.any { it.rowIndex == 1 && it.action == "skip" }
                    }
                )
            }
        }

    @Test
    fun `confirm failure surfaces the error and stays on the preview step`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(
            ImportPreviewResponse(
                sessionId = "session-1",
                rows = listOf(ImportRowPreview(rowIndex = 0, suggestedAction = "add"))
            )
        )
        coEvery {
            apiClient.confirmVcfImport(
                any()
            )
        } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("contacts.vcf", byteArrayOf(1))
        advanceUntilIdle()
        vm.confirm()
        advanceUntilIdle()

        assertEquals("server error", vm.uiState.value.error?.message)
        assertEquals(VcfImportStep.PREVIEW, vm.uiState.value.step)
    }

    // Issue #2186/#2210: an error row can be fixed in the review step instead of only skipped.
    private fun previewWithErrorRow() = ImportPreviewResponse(
        sessionId = "session-1",
        rows = listOf(
            ImportRowPreview(rowIndex = 0, suggestedAction = "add"),
            ImportRowPreview(
                rowIndex = 1,
                suggestedAction = "add",
                validationErrors = listOf("name required")
            )
        )
    )

    @Test
    fun `saving a fix keeps the row and sends the edit with the confirm`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns
                Result.success(previewWithErrorRow())
            val sent = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(sent)) } returns
                Result.success(ImportResult())
            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("c.vcf", byteArrayOf(1))
            advanceUntilIdle()
            assertEquals("skip", vm.uiState.value.rowActions[1])

            val edit = ImportRowEdit(rowIndex = 1, firstname = "Dana")
            vm.saveRowEdit(edit)
            assertEquals("add", vm.uiState.value.rowActions[1])
            assertEquals(edit, vm.uiState.value.rowEdits[1])

            vm.confirm()
            advanceUntilIdle()
            assertEquals(listOf(edit), sent.captured.rowEdits)
        }

    @Test
    fun `an edit for a row that is skipped again is not sent`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns
                Result.success(previewWithErrorRow())
            val sent = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(sent)) } returns
                Result.success(ImportResult())
            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("c.vcf", byteArrayOf(1))
            advanceUntilIdle()

            vm.saveRowEdit(ImportRowEdit(rowIndex = 1, firstname = "Dana"))
            vm.setRowAction(1, "skip")
            vm.confirm()
            advanceUntilIdle()

            assertNull(sent.captured.rowEdits)
        }

    @Test
    fun `an unfixed error row stays locked to skip`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns
            Result.success(previewWithErrorRow())
        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("c.vcf", byteArrayOf(1))
        advanceUntilIdle()

        vm.setRowAction(1, "add")

        assertEquals("skip", vm.uiState.value.rowActions[1])
    }

    @Test
    fun `saving a fix for a row that has no errors is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { apiClient.uploadVcfImport(any(), any()) } returns
                Result.success(previewWithErrorRow())
            val vm = VcfImportViewModel(apiClient)
            vm.onFilePicked("c.vcf", byteArrayOf(1))
            advanceUntilIdle()

            vm.saveRowEdit(ImportRowEdit(rowIndex = 0, firstname = "X"))

            assertEquals(emptyMap<Int, ImportRowEdit>(), vm.uiState.value.rowEdits)
        }

    @Test
    fun `resolve all does not unskip a fixed row`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns
            Result.success(previewWithErrorRow())
        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("c.vcf", byteArrayOf(1))
        advanceUntilIdle()
        vm.saveRowEdit(ImportRowEdit(rowIndex = 1, firstname = "Dana"))
        vm.setRowAction(1, "skip")

        vm.resolveAll()

        assertEquals("skip", vm.uiState.value.rowActions[1])
    }

    // ---- Issue #514 / #2523: the "Custom fields" step between upload and review ----

    private fun candidatePreview(vararg rowCandidates: List<DiscoveredCustomProperty>?) =
        ImportPreviewResponse(
            sessionId = "session-cf",
            rows = rowCandidates.mapIndexed { i, c ->
                ImportRowPreview(rowIndex = i, suggestedAction = "add", customFieldCandidates = c)
            }
        )

    private val color = DiscoveredCustomProperty(name = "x-favorite-color", value = "green")
    private val shirt = DiscoveredCustomProperty(name = "x-shirt", value = "M")

    private fun defs(vararg labels: Pair<String, String>) = Result.success(
        FieldDefinitionsResponse(
            fieldDefinitions = labels.map { (id, label) -> FieldDefinition(id = id, label = label) }
        )
    )

    private fun kotlinx.coroutines.test.TestScope.uploaded(
        preview: ImportPreviewResponse,
        definitions: Result<FieldDefinitionsResponse> = defs()
    ): VcfImportViewModel {
        coEvery { apiClient.uploadVcfImport(any(), any()) } returns Result.success(preview)
        coEvery { apiClient.listFieldDefinitions(any()) } returns definitions
        val vm = VcfImportViewModel(apiClient)
        vm.onFilePicked("c.vcf", byteArrayOf(1))
        advanceUntilIdle()
        return vm
    }

    @Test
    fun `a file with no X- properties skips the custom fields step and never loads definitions`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(null, emptyList()))

            assertEquals(VcfImportStep.PREVIEW, vm.uiState.value.step)
            assertEquals(
                emptyList<DiscoveredCustomProperty>(),
                vm.uiState.value.customFieldCandidates
            )
            coVerify(exactly = 0) { apiClient.listFieldDefinitions(any()) }
        }

    @Test
    fun `a file with X- properties goes to the custom fields step with the deduplicated union`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(
                candidatePreview(listOf(color), listOf(color.copy(value = "blue"), shirt), null),
                defs("d1" to "Hobby")
            )

            val state = vm.uiState.value
            assertEquals(VcfImportStep.CUSTOM_FIELDS, state.step)
            // First value seen wins, one entry per property name, first-seen order.
            assertEquals(listOf(color, shirt), state.customFieldCandidates)
            assertEquals(listOf("d1"), state.fieldDefinitions.map { it.id })
            // Every property defaults to "keep as passthrough" (nothing is promoted silently).
            assertEquals(
                listOf("ignore", "ignore"),
                state.customFieldCandidates.map { state.fieldMappings.getValue(it.name).action }
            )
        }

    @Test
    fun `create defaults seed a title-cased label and the text type`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)))

            vm.setFieldMappingAction("x-favorite-color", "create")

            val mapping = vm.uiState.value.fieldMappings.getValue("x-favorite-color")
            assertEquals("create", mapping.action)
            assertEquals("Favorite Color", mapping.label)
            assertEquals("text", mapping.type)
        }

    @Test
    fun `map to existing is refused when the user has no custom field definitions`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)), defs())

            vm.setFieldMappingAction("x-favorite-color", "map")

            assertEquals(
                "ignore",
                vm.uiState.value.fieldMappings.getValue("x-favorite-color").action
            )
        }

    @Test
    fun `a server-matched definition is preselected as map and available even if the list fetch fails`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val matched = shirt.copy(
                matchedDefinitionId = "def-9",
                matchedDefinitionLabel = "Shirt size"
            )
            val vm = uploaded(
                candidatePreview(listOf(matched)),
                Result.failure(ApiError.Network(java.io.IOException("offline")))
            )

            val mapping = vm.uiState.value.fieldMappings.getValue("x-shirt")
            assertEquals("map", mapping.action)
            assertEquals("def-9", mapping.fieldDefinitionId)
            assertEquals(
                listOf("def-9" to "Shirt size"),
                vm.uiState.value.fieldDefinitions.map { it.id to it.label }
            )
            // The failure is surfaced (localizedMessage path), not swallowed as "no fields yet".
            assertTrue(vm.uiState.value.error is ApiError.Network)
            assertEquals(VcfImportStep.CUSTOM_FIELDS, vm.uiState.value.step)
        }

    @Test
    fun `a matched definition already in the fetched list is not duplicated`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val matched = shirt.copy(matchedDefinitionId = "d1", matchedDefinitionLabel = "Old")
            val vm = uploaded(candidatePreview(listOf(matched)), defs("d1" to "Shirt size"))

            assertEquals(
                listOf("d1" to "Shirt size"),
                vm.uiState.value.fieldDefinitions.map { it.id to it.label }
            )
        }

    @Test
    fun `continue is blocked while a map decision has no field chosen`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)), defs("d1" to "Hobby"))
            vm.setFieldMappingAction("x-favorite-color", "map")

            assertFalse(vm.uiState.value.canContinueFromCustomFields)
            vm.continueFromCustomFields()
            assertEquals(VcfImportStep.CUSTOM_FIELDS, vm.uiState.value.step)

            vm.setFieldMappingDefinition("x-favorite-color", "d1")
            assertTrue(vm.uiState.value.canContinueFromCustomFields)
            vm.continueFromCustomFields()
            assertEquals(VcfImportStep.PREVIEW, vm.uiState.value.step)
        }

    @Test
    fun `a definition id that is not one of the user's definitions is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)), defs("d1" to "Hobby"))
            vm.setFieldMappingAction("x-favorite-color", "map")

            vm.setFieldMappingDefinition("x-favorite-color", "someone-elses")

            assertNull(
                vm.uiState.value.fieldMappings.getValue("x-favorite-color").fieldDefinitionId
            )
        }

    @Test
    fun `only contact-target definitions are offered and an excluded id is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(
                candidatePreview(listOf(color)),
                Result.success(
                    FieldDefinitionsResponse(
                        fieldDefinitions = listOf(
                            FieldDefinition(id = "c1", label = "Hobby", target = "contact"),
                            FieldDefinition(id = "c2", label = "Legacy", target = null),
                            FieldDefinition(id = "c3", label = "Blank", target = " "),
                            FieldDefinition(id = "o1", label = "Industry", target = "organisation"),
                            FieldDefinition(id = "p1", label = "Breed", target = "pet")
                        )
                    )
                )
            )

            assertEquals(
                listOf("c1", "c2", "c3"),
                vm.uiState.value.fieldDefinitions.map { it.id }
            )
            vm.setFieldMappingAction("x-favorite-color", "map")
            vm.setFieldMappingDefinition("x-favorite-color", "o1")
            assertNull(
                vm.uiState.value.fieldMappings.getValue("x-favorite-color").fieldDefinitionId
            )
            vm.setFieldMappingDefinition("x-favorite-color", "c1")
            assertEquals(
                "c1",
                vm.uiState.value.fieldMappings.getValue("x-favorite-color").fieldDefinitionId
            )
        }

    @Test
    fun `with only non-contact definitions the map action is unavailable`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(
                candidatePreview(listOf(color)),
                Result.success(
                    FieldDefinitionsResponse(
                        fieldDefinitions = listOf(
                            FieldDefinition(id = "o1", label = "Industry", target = "organisation")
                        )
                    )
                )
            )
            assertEquals(emptyList<FieldDefinition>(), vm.uiState.value.fieldDefinitions)
            vm.setFieldMappingAction("x-favorite-color", "map")
            assertEquals(
                "ignore",
                vm.uiState.value.fieldMappings.getValue("x-favorite-color").action
            )
        }

    @Test
    fun `confirm sends one mapping per property carrying only the fields its action uses`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(
                candidatePreview(listOf(color, shirt, DiscoveredCustomProperty("x-pet", "dog"))),
                defs("d1" to "Hobby")
            )
            vm.setFieldMappingAction("x-favorite-color", "map")
            vm.setFieldMappingDefinition("x-favorite-color", "d1")
            vm.setFieldMappingAction("x-shirt", "create")
            vm.setFieldMappingLabel("x-shirt", "Shirt size")
            vm.setFieldMappingType("x-shirt", "string")
            // x-pet stays on the default (keep as passthrough).
            vm.continueFromCustomFields()
            val request = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(request)) } returns
                Result.success(ImportResult(created = 1))

            vm.confirm()
            advanceUntilIdle()

            assertEquals(
                listOf(
                    ImportFieldMapping("x-favorite-color", "map", fieldDefinitionId = "d1"),
                    ImportFieldMapping("x-shirt", "create", label = "Shirt size", type = "string"),
                    ImportFieldMapping("x-pet", "ignore")
                ),
                request.captured.fieldMappings
            )
        }

    @Test
    fun `switching a create decision back to passthrough drops its label and type from the request`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)))
            vm.setFieldMappingAction("x-favorite-color", "create")
            vm.setFieldMappingLabel("x-favorite-color", "Colour")
            vm.setFieldMappingAction("x-favorite-color", "ignore")
            vm.continueFromCustomFields()
            val request = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(request)) } returns
                Result.success(ImportResult())

            vm.confirm()
            advanceUntilIdle()

            assertEquals(
                listOf(ImportFieldMapping("x-favorite-color", "ignore")),
                request.captured.fieldMappings
            )
        }

    @Test
    fun `a file without X- properties confirms with no field_mappings key at all`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(null))
            val request = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(request)) } returns
                Result.success(ImportResult())

            vm.confirm()
            advanceUntilIdle()

            assertNull(request.captured.fieldMappings)
        }

    @Test
    fun `a blank create label is sent as null so the server derives one`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)))
            vm.setFieldMappingAction("x-favorite-color", "create")
            vm.setFieldMappingLabel("x-favorite-color", "   ")
            vm.continueFromCustomFields()
            val request = slot<ImportConfirmRequest>()
            coEvery { apiClient.confirmVcfImport(capture(request)) } returns
                Result.success(ImportResult())

            vm.confirm()
            advanceUntilIdle()

            val sent = request.captured.fieldMappings!!.single()
            assertEquals("create", sent.action)
            assertNull(sent.label)
        }

    @Test
    fun `an unknown property name or a type outside the field types is ignored`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = uploaded(candidatePreview(listOf(color)))

            vm.setFieldMappingAction("x-nope", "create")
            vm.setFieldMappingAction("x-favorite-color", "bogus")
            vm.setFieldMappingAction("x-favorite-color", "create")
            vm.setFieldMappingType("x-favorite-color", "hologram")

            assertEquals(setOf("x-favorite-color"), vm.uiState.value.fieldMappings.keys)
            assertEquals("text", vm.uiState.value.fieldMappings.getValue("x-favorite-color").type)
        }
}
