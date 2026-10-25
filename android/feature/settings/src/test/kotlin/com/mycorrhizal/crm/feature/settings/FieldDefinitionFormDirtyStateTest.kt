package com.mycorrhizal.crm.feature.settings

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.FieldDefinitionRepository
import com.mycorrhizal.crm.model.network.FieldConstraints
import com.mycorrhizal.crm.model.network.FieldDefinition
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #1751: [FieldDefinitionFormState.isDirty] drives the unsaved-changes guard. */
class FieldDefinitionFormDirtyStateTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<FieldDefinitionRepository>().also {
        coEvery { it.list(any()) } returns Result.success(emptyList())
    }

    @Test
    fun `a new field form starts clean, becomes dirty, and returns to clean`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = FieldDefinitionFormViewModel(repository, SavedStateHandle())
        advanceUntilIdle()
        assertFalse(vm.uiState.value.isDirty)

        vm.onLabelChange("Coffee order")
        assertTrue(vm.uiState.value.isDirty)

        vm.onLabelChange("")
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `enum value edits mark the form dirty`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = FieldDefinitionFormViewModel(repository, SavedStateHandle())
        advanceUntilIdle()

        vm.addEnumValue()
        assertTrue(vm.uiState.value.isDirty)

        vm.removeEnumValue(0)
        assertFalse(vm.uiState.value.isDirty)
    }

    @Test
    fun `a loaded field edit starts clean, and the key stays read-only and clean`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.get("d1") } returns Result.success(
            FieldDefinition(
                id = "d1",
                label = "Coffee order",
                key = "coffee_order",
                type = "enum",
                constraints = FieldConstraints(values = listOf("Latte"), multi = true),
                projection = "vcard:X-COFFEE",
                sensitivity = "private"
            )
        )
        val vm = FieldDefinitionFormViewModel(
            repository,
            SavedStateHandle(mapOf("fieldDefinitionId" to "d1"))
        )
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isDirty)

        vm.onKeyChange("another_key")
        assertFalse(vm.uiState.value.isDirty)

        vm.onGroupChange("Preferences")
        assertTrue(vm.uiState.value.isDirty)
    }
}
