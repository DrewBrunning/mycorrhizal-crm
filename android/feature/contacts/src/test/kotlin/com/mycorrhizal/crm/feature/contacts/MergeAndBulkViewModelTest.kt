package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.MergeRepository
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.ContactMergeResolution
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.ApiError
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

class MergeContactsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val mergeRepository = mockk<MergeRepository>()

    @Test
    fun `preview loads and exposes conflicts`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { mergeRepository.preview(any()) } returns Result.success(
            ContactMergePreviewResponse(
                keepId = 1, mergeId = 2,
                resolution = ContactMergeResolution(
                    conflicts = listOf(
                        com.mycorrhizal.crm.model.network.ContactMergeFieldConflict(
                            field = "firstname", label = "First name",
                            keeperValue = "Dana", loserValue = "Dana",
                        ),
                    ),
                ),
            ),
        )

        val vm = MergeContactsViewModel(mergeRepository)
        vm.setPair(1, 2)
        vm.preview()
        advanceUntilIdle()

        val state = vm.uiState.value
        assertEquals(1, state.preview?.resolution?.conflicts?.size)
        assertEquals(1, vm.allConflicts(state.preview!!).size)
    }

    @Test
    fun `commit sends resolutions and marks merged`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { mergeRepository.commit(any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactRecordResponse(id = 1),
        )

        val vm = MergeContactsViewModel(mergeRepository)
        vm.setPair(1, 2)
        vm.resolve("firstname", "Dana")
        vm.commit()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.merged)
        coVerify {
            mergeRepository.commit(
                match<ContactMergeRequest> { it.resolutions?.get("firstname") == "Dana" },
            )
        }
    }

    @Test
    fun `preview failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { mergeRepository.preview(any()) } returns Result.failure(ApiError.Client(400, "bad pair"))

        val vm = MergeContactsViewModel(mergeRepository)
        vm.setPair(1, 1)
        vm.preview()
        advanceUntilIdle()

        assertEquals("bad pair", vm.uiState.value.error)
    }
}

class BulkOperationsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val bulkRepository = mockk<BulkOperationRepository>()
    private val contactRepository = mockk<ContactRepository>()

    @Test
    fun `loads contacts and runs a bulk action on selected uids`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts() } returns Result.success(
            ContactsPage(
                contacts = listOf(
                    ContactSummary(id = 1, uid = "uid-1", fn = "Dana"),
                    ContactSummary(id = 2, uid = "uid-2", fn = "Carol"),
                ),
                nextCursor = null,
                limit = 100,
                sync = null,
            ),
        )
        coEvery {
            bulkRepository.run(
                match { it.action == "archive" && it.vcardUids == listOf("uid-1") },
            )
        } returns Result.success(BulkOperationResult(action = "archive", total = 1, succeeded = 1, failed = 0))

        val vm = BulkOperationsViewModel(bulkRepository, contactRepository)
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.contacts.size)
        vm.toggle(1)
        vm.run("archive")
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.result?.succeeded)
    }
}
