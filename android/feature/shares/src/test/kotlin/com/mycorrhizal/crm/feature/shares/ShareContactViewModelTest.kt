package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ContactShareStatuses
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
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

class ShareContactViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val shareRepository = mockk<ContactShareRepository>()

    private fun viewModel(uid: String = "uid-5"): ShareContactViewModel {
        coEvery { shareRepository.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 9, username = "casey")),
        )
        return ShareContactViewModel(
            shareRepository,
            SavedStateHandle(mapOf("uid" to uid)),
        )
    }

    @Test
    fun `loads the recipient directory on init`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(listOf("casey"), vm.uiState.value.recipients.map { it.username })
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `non-sensitive sections are selected by default`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.selectedSections.contains("emails"))
        assertTrue(state.selectedSections.contains("phones"))
        assertFalse(state.selectedSections.contains("related_to"))
        assertFalse(state.sensitiveRevealed)
    }

    @Test
    fun `share is disabled until a recipient is chosen`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.canShare)
        vm.selectRecipient(9)
        assertTrue(vm.uiState.value.canShare)
    }

    @Test
    fun `share creates the share with the selected sections and the passed-through uid`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { shareRepository.create(any()) } returns Result.success(
                ContactShare(id = "s1", fromUserId = 1, toUserId = 9, contactDisplayName = "Dana", status = ContactShareStatuses.PENDING),
            )
            val vm = viewModel(uid = "uid-5")
            advanceUntilIdle()

            vm.selectRecipient(9)
            vm.share()
            advanceUntilIdle()

            coVerify {
                shareRepository.create(
                    ContactShareInput(
                        toUserId = 9,
                        vcardUid = "uid-5",
                        sections = ShareFieldSections.DEFAULT_SELECTED,
                        includeSensitive = false,
                    ),
                )
            }
            assertTrue(vm.uiState.value.shared)
        }

    @Test
    fun `include_sensitive is only set when sensitive sections are revealed and selected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { shareRepository.create(any()) } returns Result.success(
                ContactShare(id = "s1", toUserId = 9, contactDisplayName = "Dana"),
            )
            val vm = viewModel()
            advanceUntilIdle()

            vm.selectRecipient(9)
            // Reveal alone is not enough — a sensitive section must be selected.
            vm.revealSensitive()
            vm.share()
            advanceUntilIdle()

            coVerify {
                shareRepository.create(
                    ContactShareInput(
                        toUserId = 9,
                        vcardUid = "uid-5",
                        sections = ShareFieldSections.DEFAULT_SELECTED,
                        includeSensitive = false,
                    ),
                )
            }

            // Now also select a sensitive section.
            vm.toggleSection("related_to", true)
            vm.share()
            advanceUntilIdle()

            coVerify {
                shareRepository.create(
                    ContactShareInput(
                        toUserId = 9,
                        vcardUid = "uid-5",
                        sections = ShareFieldSections.DEFAULT_SELECTED + "related_to",
                        includeSensitive = true,
                    ),
                )
            }
            assertTrue(vm.uiState.value.shared)
        }

    @Test
    fun `share failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { shareRepository.create(any()) } returns Result.failure(ApiError.Client(400, "Cannot share"))
        val vm = viewModel()
        advanceUntilIdle()

        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.shared)
        assertEquals("Cannot share", vm.uiState.value.error)
    }

    @Test
    fun `missing contact uid is surfaced without calling the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel(uid = "")
        advanceUntilIdle()

        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()

        coVerify(exactly = 0) { shareRepository.create(any()) }
        assertFalse(vm.uiState.value.shared)
        assertTrue(vm.uiState.value.errorRes != null)
    }

    @Test
    fun `directory load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { shareRepository.userDirectory() } returns Result.failure(ApiError.Client(500, "boom"))
        val vm = ShareContactViewModel(
            shareRepository,
            SavedStateHandle(mapOf("uid" to "uid-5")),
        )
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("boom", vm.uiState.value.error)
    }
}
