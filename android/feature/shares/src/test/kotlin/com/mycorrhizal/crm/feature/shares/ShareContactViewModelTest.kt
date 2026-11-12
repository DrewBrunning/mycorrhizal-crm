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
import kotlinx.coroutines.CompletableDeferred
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
        coEvery { grantRepository.previewContact(any(), any()) } returns
            Result.success(emptyMap())
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

    // --- Issue #2519: the live "What they will see" preview of a standing share ---

    private val card = mapOf<String, Any?>("name" to mapOf("full" to "Dana"))

    @Test
    fun `a one-time share never asks for a preview`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.toggleSection("emails", false)
        advanceUntilIdle()

        coVerify(exactly = 0) { grantRepository.previewContact(any(), any()) }
        assertEquals(StandingPreview.Idle, vm.uiState.value.preview)
    }

    @Test
    fun `choosing standing previews the selected sections of this contact`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel(uid = "uid-5")
        coEvery { grantRepository.previewContact(any(), any()) } returns Result.success(card)
        advanceUntilIdle()

        vm.setMode(ShareMode.STANDING)
        assertEquals(StandingPreview.Loading, vm.uiState.value.preview)
        advanceUntilIdle()

        coVerify(exactly = 1) {
            grantRepository.previewContact("uid-5", ShareFieldSections.DEFAULT_SELECTED.sorted())
        }
        assertEquals(StandingPreview.Ready(card), vm.uiState.value.preview)
    }

    @Test
    fun `toggling a section re-requests the preview with exactly the new selection`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        vm.toggleSection("emails", false)
        advanceUntilIdle()

        val expected = (ShareFieldSections.DEFAULT_SELECTED - "emails").sorted()
        coVerify(exactly = 1) { grantRepository.previewContact("uid-5", expected) }
        assertFalse(expected.contains("emails"))
    }

    @Test
    fun `an unselected sensitive section is not requested, a revealed and selected one is`() =
        runTest(mainDispatcherRule.testDispatcher) {
            val vm = viewModel()
            advanceUntilIdle()
            vm.setMode(ShareMode.STANDING)
            advanceUntilIdle()
            coVerify(exactly = 0) {
                grantRepository.previewContact(any(), match { "related_to" in it })
            }

            vm.revealSensitive()
            vm.toggleSection("related_to", true)
            advanceUntilIdle()
            coVerify(exactly = 1) {
                grantRepository.previewContact(any(), match { "related_to" in it })
            }
        }

    @Test
    fun `deselecting every section clears the preview without a request`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        ShareFieldSections.DEFAULT_SELECTED.forEach { vm.toggleSection(it, false) }
        advanceUntilIdle()

        assertEquals(StandingPreview.Idle, vm.uiState.value.preview)
        assertFalse(vm.uiState.value.canShare)
    }

    @Test
    fun `a failed preview is an error state that the next change recovers from`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        coEvery { grantRepository.previewContact(any(), any()) } returns
            Result.failure(ApiError.Client(500, "boom"))
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        val failed = vm.uiState.value.preview as StandingPreview.Error
        assertEquals("boom", failed.error.message)
        // A preview failure never blocks sharing or raises the form-level error.
        assertNull(vm.uiState.value.error)

        coEvery { grantRepository.previewContact(any(), any()) } returns Result.success(card)
        vm.toggleSection("emails", false)
        advanceUntilIdle()
        assertEquals(StandingPreview.Ready(card), vm.uiState.value.preview)
    }

    @Test
    fun `a slower earlier preview never overwrites a newer one`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        val slow = CompletableDeferred<Result<Map<String, Any?>>>()
        val fresh = mapOf<String, Any?>("name" to mapOf("full" to "Fresh"))
        coEvery { grantRepository.previewContact(any(), any()) } coAnswers { slow.await() }
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        coEvery { grantRepository.previewContact(any(), any()) } returns Result.success(fresh)
        vm.toggleSection("emails", false)
        advanceUntilIdle()
        slow.complete(Result.success(card))
        advanceUntilIdle()

        assertEquals(StandingPreview.Ready(fresh), vm.uiState.value.preview)
    }

    @Test
    fun `going back to one-time drops the preview and ignores a late response`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        val slow = CompletableDeferred<Result<Map<String, Any?>>>()
        coEvery { grantRepository.previewContact(any(), any()) } coAnswers { slow.await() }
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        vm.setMode(ShareMode.ONE_TIME)
        slow.complete(Result.success(card))
        advanceUntilIdle()

        assertEquals(StandingPreview.Idle, vm.uiState.value.preview)
    }

    @Test
    fun `a contact without a uid has nothing to preview`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel(uid = "")
        advanceUntilIdle()
        vm.setMode(ShareMode.STANDING)
        advanceUntilIdle()

        coVerify(exactly = 0) { grantRepository.previewContact(any(), any()) }
        assertEquals(StandingPreview.Idle, vm.uiState.value.preview)
    }
}
