package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.AuthRepository
import com.mycorrhizal.crm.model.network.CONTACT_FIELD_GROUP
import com.mycorrhizal.crm.model.network.ContactFieldKey
import com.mycorrhizal.crm.model.network.DEFAULT_ENABLED_CONTACT_FIELDS
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactFieldSettingsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<AuthRepository>()

    @Test
    fun `load with a never-configured null value applies the default enabled set`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getEnabledContactFields() } returns Result.success(null)
            val vm = ContactFieldSettingsViewModel(repository)
            advanceUntilIdle()

            val state = vm.uiState.value
            assertFalse(state.isLoading)
            assertEquals(DEFAULT_ENABLED_CONTACT_FIELDS, state.enabled)
        }

    @Test
    fun `load with an explicit empty list shows nothing enabled, not the defaults`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getEnabledContactFields() } returns Result.success(emptyList())
            val vm = ContactFieldSettingsViewModel(repository)
            advanceUntilIdle()

            assertTrue(vm.uiState.value.enabled.isEmpty())
        }

    @Test
    fun `load failure surfaces the load error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getEnabledContactFields() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = ContactFieldSettingsViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.loadError)
    }

    @Test
    fun `toggling on a disabled key enables it and patches the full resulting set`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getEnabledContactFields() } returns Result.success(
                DEFAULT_ENABLED_CONTACT_FIELDS.map { it.wireKey },
            )
            val captured = mutableListOf<List<String>>()
            coEvery { repository.updateEnabledContactFields(any()) } answers {
                captured.add(firstArg())
                Result.success(firstArg())
            }
            val vm = ContactFieldSettingsViewModel(repository)
            advanceUntilIdle()

            vm.onToggle(ContactFieldKey.KEYWORDS)
            advanceUntilIdle()

            assertTrue(ContactFieldKey.KEYWORDS in vm.uiState.value.enabled)
            assertTrue("keywords" in captured.single())
            assertNull(vm.uiState.value.savingKey)
        }

    @Test
    fun `toggling off an enabled key disables it`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getEnabledContactFields() } returns Result.success(
            DEFAULT_ENABLED_CONTACT_FIELDS.map { it.wireKey },
        )
        coEvery { repository.updateEnabledContactFields(any()) } answers { Result.success(firstArg()) }
        val vm = ContactFieldSettingsViewModel(repository)
        advanceUntilIdle()
        assertTrue(ContactFieldKey.EMAILS in vm.uiState.value.enabled)

        vm.onToggle(ContactFieldKey.EMAILS)
        advanceUntilIdle()

        assertFalse(ContactFieldKey.EMAILS in vm.uiState.value.enabled)
    }

    @Test
    fun `a failed toggle reverts the optimistic flip and surfaces an error`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.getEnabledContactFields() } returns Result.success(emptyList())
            coEvery { repository.updateEnabledContactFields(any()) } returns Result.failure(
                ApiError.Server(500, "boom"),
            )
            val vm = ContactFieldSettingsViewModel(repository)
            advanceUntilIdle()

            vm.onToggle(ContactFieldKey.KEYWORDS)
            advanceUntilIdle()

            assertFalse(ContactFieldKey.KEYWORDS in vm.uiState.value.enabled)
            assertEquals("Server error (500)", vm.uiState.value.saveError)
            assertNull(vm.uiState.value.savingKey)
        }

    @Test
    fun `a second toggle while one is already saving is ignored`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getEnabledContactFields() } returns Result.success(emptyList())
        coEvery { repository.updateEnabledContactFields(any()) } returns Result.success(listOf("emails"))
        val vm = ContactFieldSettingsViewModel(repository)
        advanceUntilIdle()

        vm.onToggle(ContactFieldKey.EMAILS)
        // Deliberately not advancing idle yet — the PATCH above is still "in flight"
        // from the state's point of view within this same test dispatcher tick.
        vm.onToggle(ContactFieldKey.PHONES)
        advanceUntilIdle()

        coVerify(exactly = 1) { repository.updateEnabledContactFields(any()) }
    }

    @Test
    fun `every key in the group map is toggleable`() {
        // Sanity check for the settings screen's render loop: every enum entry
        // must have a group, or it silently never appears in the UI.
        ContactFieldKey.entries.forEach { key ->
            assertTrue("$key must be grouped for the settings screen to render it", CONTACT_FIELD_GROUP.containsKey(key))
        }
    }
}
