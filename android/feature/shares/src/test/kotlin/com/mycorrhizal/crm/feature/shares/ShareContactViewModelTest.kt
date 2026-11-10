package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ContactShare
import com.mycorrhizal.crm.model.network.ContactShareInput
import com.mycorrhizal.crm.model.network.ContactShareStatuses
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.ShareFieldSections
import com.mycorrhizal.crm.model.network.ShareGrantInput
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
    private val grantRepository = mockk<ShareGrantRepository>()

    private fun viewModel(uid: String = "uid-5"): ShareContactViewModel {
        coEvery { shareRepository.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 9, username = "casey"))
        )
        return ShareContactViewModel(
            shareRepository,
            grantRepository,
            SavedStateHandle(mapOf("uid" to uid))
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
    fun `non-sensitive sections are selected by default`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(state.selectedSections.contains("emails"))
        assertTrue(state.selectedSections.contains("phones"))
        assertFalse(state.selectedSections.contains("related_to"))
        assertFalse(state.sensitiveRevealed)
    }

    @Test
    fun `share is disabled until a recipient is chosen`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
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
                ContactShare(
                    id = "s1",
                    fromUserId = 1,
                    toUserId = 9,
                    contactDisplayName = "Dana",
                    status = ContactShareStatuses.PENDING
                )
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
                        includeSensitive = false
                    )
                )
            }
            assertTrue(vm.uiState.value.shared)
        }

    @Test
    fun `include_sensitive is only set when sensitive sections are revealed and selected`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { shareRepository.create(any()) } returns Result.success(
                ContactShare(id = "s1", toUserId = 9, contactDisplayName = "Dana")
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
                        includeSensitive = false
                    )
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
                        includeSensitive = true
                    )
                )
            }
            assertTrue(vm.uiState.value.shared)
        }

    @Test
    fun `share failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery {
            shareRepository.create(
                any()
            )
        } returns Result.failure(ApiError.Client(400, "Cannot share"))
        val vm = viewModel()
        advanceUntilIdle()

        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.shared)
        assertEquals("Cannot share", vm.uiState.value.error?.message)
    }

    @Test
    fun `missing contact uid is surfaced without calling the repository`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
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
        coEvery { shareRepository.userDirectory() } returns Result.failure(
            ApiError.Client(500, "boom")
        )
        val vm = ShareContactViewModel(
            shareRepository,
            grantRepository,
            SavedStateHandle(mapOf("uid" to "uid-5"))
        )
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("boom", vm.uiState.value.error?.message)
    }

    // --- Standing shares (ADR 0042 decision 12): the one-time vs standing choice ---

    @Test
    fun `the mode defaults to one-time so the existing flow is unchanged`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        advanceUntilIdle()
        assertEquals(ShareMode.ONE_TIME, vm.uiState.value.mode)
    }

    @Test
    fun `a one-time share never touches the grant repository`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { shareRepository.create(any()) } returns Result.success(
            ContactShare(id = "s1", toUserId = 9, contactDisplayName = "Dana")
        )
        val vm = viewModel()
        advanceUntilIdle()
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()
        coVerify(exactly = 0) { grantRepository.create(any()) }
        assertTrue(vm.uiState.value.shared)
    }

    @Test
    fun `a standing share creates a grant with the sections and no sensitive flag`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { grantRepository.create(any()) } returns Result.success(OutgoingShareGrant("g1"))
        val vm = viewModel(uid = "uid-5")
        advanceUntilIdle()

        vm.setMode(ShareMode.STANDING)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()

        coVerify(exactly = 1) {
            grantRepository.create(
                ShareGrantInput(
                    toUserId = 9,
                    entityType = "contact",
                    entityId = "uid-5",
                    sections = ShareFieldSections.DEFAULT_SELECTED,
                    expiresAt = null
                )
            )
        }
        coVerify(exactly = 0) { shareRepository.create(any()) }
        assertTrue(vm.uiState.value.shared)
        assertTrue(vm.uiState.value.sharedStanding)
    }

    @Test
    fun `a standing share keeps a deliberately revealed sensitivity-bearing section`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { grantRepository.create(any()) } returns Result.success(OutgoingShareGrant("g1"))
        val vm = viewModel()
        advanceUntilIdle()
        vm.revealSensitive()
        vm.toggleSection("related_to", true)
        vm.setMode(ShareMode.STANDING)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()
        coVerify {
            grantRepository.create(match { it.sections.contains("related_to") })
        }
    }

    @Test
    fun `a standing share can carry an expiry`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { grantRepository.create(any()) } returns Result.success(OutgoingShareGrant("g1"))
        val vm = viewModel()
        advanceUntilIdle()
        vm.now = { java.time.Instant.parse("2026-11-10T00:00:00Z") }
        vm.setMode(ShareMode.STANDING)
        vm.setExpiryDays(90)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()
        coVerify { grantRepository.create(match { it.expiresAt == "2027-02-08T00:00:00Z" }) }
    }

    @Test
    fun `a standing share failure surfaces the error and stays on the screen`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { grantRepository.create(any()) } returns
            Result.failure(ApiError.Client(409, "dup"))
        val vm = viewModel()
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()
        assertFalse(vm.uiState.value.shared)
        assertTrue(vm.uiState.value.error != null)
    }

    @Test
    fun `a standing share with no contact uid does not call the repository`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel(uid = "")
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()
        coVerify(exactly = 0) { grantRepository.create(any()) }
        assertTrue(vm.uiState.value.errorRes != null)
    }
}
