package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.CustomLinkAction
import com.mycorrhizal.crm.domain.repository.CustomLinkActionRepository
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class CustomLinkActionsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<CustomLinkActionRepository>()

    private fun action(id: Long, protocol: String = "myapp") = CustomLinkAction(
        id = id,
        protocol = protocol,
        label = "Open in MyApp",
        kind = "APP_OPEN",
        mimeType = "text/plain",
        intentUriTemplate = "myapp://open?id={value}",
    )

    @Test
    fun `init loads the action list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getAll() } returns listOf(action(1), action(2))

        val vm = CustomLinkActionsViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(!state.isLoading)
        assertEquals(2, state.actions.size)
    }

    @Test
    fun `add trims fields, upserts, and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getAll() } returns emptyList()
        val vm = CustomLinkActionsViewModel(repository)
        advanceUntilIdle()

        coEvery { repository.upsert(any()) } returns Unit
        coEvery { repository.getAll() } returns listOf(action(1))

        vm.add(" myapp ", " Open in MyApp ", " text/plain ", " myapp://open?id={value} ")
        advanceUntilIdle()

        coVerify {
            repository.upsert(
                CustomLinkAction(
                    protocol = "myapp",
                    label = "Open in MyApp",
                    kind = "APP_OPEN",
                    mimeType = "text/plain",
                    intentUriTemplate = "myapp://open?id={value}",
                ),
            )
        }
        assertEquals(1, vm.uiState.value.actions.size)
    }

    @Test
    fun `add does nothing when any trimmed field is blank`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.getAll() } returns emptyList()
        val vm = CustomLinkActionsViewModel(repository)
        advanceUntilIdle()

        vm.add("  ", "label", "mime", "template")
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.upsert(any()) }
    }

    @Test
    fun `delete removes the action and reloads`() = runTest(mainDispatcherRule.testDispatcher) {
        val existing = action(1)
        coEvery { repository.getAll() } returns listOf(existing)
        val vm = CustomLinkActionsViewModel(repository)
        advanceUntilIdle()

        coEvery { repository.delete(existing) } returns Unit
        coEvery { repository.getAll() } returns emptyList()

        vm.delete(existing)
        advanceUntilIdle()

        coVerify { repository.delete(existing) }
        assertTrue(vm.uiState.value.actions.isEmpty())
    }
}
