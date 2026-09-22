package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.model.network.FieldDefinitionInput
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

/** Issue #830. Mirrors `ReminderFormViewModel`'s create-vs-edit shape. */
class FieldDefinitionFormViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<FieldDefinitionRepository>()

    private fun createViewModel(): FieldDefinitionFormViewModel =
        FieldDefinitionFormViewModel(repository, SavedStateHandle())

    private fun editViewModel(id: String = "d1"): FieldDefinitionFormViewModel =
        FieldDefinitionFormViewModel(repository, SavedStateHandle(mapOf("fieldDefinitionId" to id)))

    @Test
    fun `create mode starts with the default form state and does not load`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isEdit)
        assertFalse(vm.uiState.value.isLoading)
        assertEquals("", vm.uiState.value.label)
        assertEquals("string", vm.uiState.value.type)
        assertEquals("normal", vm.uiState.value.sensitivity)
        coVerify(exactly = 0) { repository.get(any()) }
    }

    @Test
    fun `edit mode loads and prefills from the existing definition`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.get("d1") } returns Result.success(
            FieldDefinition(
                id = "d1",
                label = "Coffee order",
                key = "coffee_order",
                type = "enum",
                constraints = FieldConstraints(values = listOf("Latte", "Espresso"), multi = true),
                projection = "vcard:X-COFFEE",
                sensitivity = "private",
            ),
        )

        val vm = editViewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.isEdit)
        assertFalse(state.isLoading)
        assertEquals("Coffee order", state.label)
        assertEquals("coffee_order", state.key)
        assertEquals("enum", state.type)
        assertTrue(state.multi)
        assertEquals(listOf("Latte", "Espresso"), state.enumValues)
        assertEquals("vcard", state.projectionMode)
        assertEquals("COFFEE", state.vcardName)
        assertEquals("private", state.sensitivity)
    }

    @Test
    fun `blank label blocks save with the label-required error`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onKeyChange("k")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.settings_custom_fields_error_label_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `blank key blocks save on create but not on edit`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onLabelChange("Coffee order")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.settings_custom_fields_error_key_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `enum type with no allowed values blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onLabelChange("Coffee order")
        vm.onKeyChange("coffee_order")
        vm.onTypeChange("enum")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.settings_custom_fields_error_enum_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `vcard projection with a blank name blocks save`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.onLabelChange("Coffee order")
        vm.onKeyChange("coffee_order")
        vm.onProjectionModeChange("vcard")
        vm.save()
        advanceUntilIdle()

        assertEquals(R.string.settings_custom_fields_error_vcard_name_required, vm.uiState.value.errorRes)
        coVerify(exactly = 0) { repository.create(any()) }
    }

    @Test
    fun `save on create builds the expected input and emits Saved`() = runTest(mainDispatcherRule.testDispatcher) {
        val slot = slot<FieldDefinitionInput>()
        coEvery { repository.create(capture(slot)) } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order", key = "coffee_order", type = "number"),
        )

        val vm = createViewModel()
        advanceUntilIdle()

        vm.onLabelChange("Coffee order")
        vm.onKeyChange("coffee_order")
        vm.onTypeChange("number")
        vm.onMinChange("1")
        vm.onMaxChange("5")
        vm.save()
        advanceUntilIdle()

        assertEquals(FieldDefinitionFormEvent.Saved, vm.events.value)
        assertFalse(vm.uiState.value.isSaving)
        assertNull(vm.uiState.value.errorRes)

        val input = slot.captured
        assertEquals("Coffee order", input.label)
        assertEquals("coffee_order", input.key)
        assertEquals("number", input.type)
        assertEquals(1.0, input.constraints?.min)
        assertEquals(5.0, input.constraints?.max)
        assertNull(input.constraints?.maxLength)
        assertNull(input.constraints?.values)
        assertEquals("internal-only", input.projection)
    }

    @Test
    fun `save on edit calls update and never sends a changed key`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.get("d1") } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order", key = "coffee_order", type = "string"),
        )
        val slot = slot<FieldDefinitionInput>()
        coEvery { repository.update("d1", capture(slot)) } returns Result.success(
            FieldDefinition(id = "d1", label = "Coffee order (renamed)", key = "coffee_order", type = "string"),
        )

        val vm = editViewModel()
        advanceUntilIdle()

        vm.onKeyChange("attempted-change") // ignored while isEdit
        vm.onLabelChange("Coffee order (renamed)")
        vm.save()
        advanceUntilIdle()

        assertEquals(FieldDefinitionFormEvent.Saved, vm.events.value)
        coVerify(exactly = 0) { repository.create(any()) }
        assertEquals("coffee_order", slot.captured.key)
        assertEquals("Coffee order (renamed)", slot.captured.label)
    }

    @Test
    fun `save failure surfaces the error and does not emit Saved`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.create(any()) } returns Result.failure(ApiError.Client(409, "already exists"))

        val vm = createViewModel()
        advanceUntilIdle()

        vm.onLabelChange("Coffee order")
        vm.onKeyChange("coffee_order")
        vm.save()
        advanceUntilIdle()

        assertEquals("already exists", vm.uiState.value.error)
        assertFalse(vm.uiState.value.isSaving)
        assertNull(vm.events.value)
    }

    @Test
    fun `enum values can be added updated and removed`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = createViewModel()
        advanceUntilIdle()

        vm.addEnumValue()
        vm.addEnumValue()
        vm.updateEnumValue(0, "Latte")
        vm.updateEnumValue(1, "Espresso")
        assertEquals(listOf("Latte", "Espresso"), vm.uiState.value.enumValues)

        vm.removeEnumValue(0)
        assertEquals(listOf("Espresso"), vm.uiState.value.enumValues)
    }
}
