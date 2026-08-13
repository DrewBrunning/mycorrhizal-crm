package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.BulkOperationRepository
import com.mycorrhizal.crm.domain.repository.CircleRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.domain.repository.MergeRepository
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.BulkOperationResult
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.ContactMergePreviewResponse
import com.mycorrhizal.crm.model.network.ContactMergeRequest
import com.mycorrhizal.crm.model.network.ContactMergeResolution
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Tag
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
    private val circleRepository = mockk<CircleRepository>()
    private val tagRepository = mockk<TagRepository>()

    private val threeContacts = ContactsPage(
        contacts = listOf(
            ContactSummary(id = 1, uid = "uid-1", fn = "Dana"),
            ContactSummary(id = 2, uid = "uid-2", fn = "Carol"),
            ContactSummary(id = 3, uid = "uid-3", fn = "Erin"),
        ),
        nextCursor = null,
        limit = 100,
        sync = null,
    )

    private fun stubCirclesAndTags(
        circles: List<Circle> = listOf(Circle(id = "circle-1", name = "Book club")),
        tags: List<Tag> = listOf(Tag(id = "tag-1", name = "VIP")),
    ) {
        coEvery { circleRepository.list() } returns Result.success(circles)
        coEvery { tagRepository.list() } returns Result.success(tags)
    }

    private fun viewModel(): BulkOperationsViewModel =
        BulkOperationsViewModel(bulkRepository, contactRepository, circleRepository, tagRepository)

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
        stubCirclesAndTags()
        coEvery {
            bulkRepository.run(
                match { it.action == "archive" && it.vcardUids == listOf("uid-1") },
            )
        } returns Result.success(BulkOperationResult(action = "archive", total = 1, succeeded = 1, failed = 0))

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.contacts.size)
        vm.toggle(1)
        vm.run("archive")
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.result?.succeeded)
    }

    // M9 item 2 / ticket test case 2 (first half): selecting several contacts and running a
    // circle action issues exactly one bulkOperation call carrying every selected uid plus the
    // chosen circle id.
    @Test
    fun `adding a circle sends every selected uid and the circle id in one call`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts() } returns Result.success(threeContacts)
        stubCirclesAndTags()
        coEvery {
            bulkRepository.run(
                match { it.action == "add_circle" && it.circleId == "circle-1" },
            )
        } returns Result.success(BulkOperationResult(action = "add_circle", total = 3, succeeded = 3, failed = 0))

        val vm = viewModel()
        advanceUntilIdle()

        vm.toggle(1)
        vm.toggle(2)
        vm.toggle(3)
        vm.run("add_circle", circleId = "circle-1")
        advanceUntilIdle()

        coVerify(exactly = 1) {
            bulkRepository.run(
                match { it.action == "add_circle" && it.circleId == "circle-1" && it.vcardUids.toSet() == setOf("uid-1", "uid-2", "uid-3") },
            )
        }
        assertEquals(3, vm.uiState.value.result?.succeeded)
        assertTrue(vm.uiState.value.selected.isEmpty())
    }

    // M9 item 2 / ticket test case 2 (second half): a failed run must NOT clear the selection —
    // only a successful one does. This is what the ViewModel already does; this test is what
    // proves it and would fail if that behavior regressed.
    @Test
    fun `a failed bulk action leaves the selection untouched`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts() } returns Result.success(threeContacts)
        stubCirclesAndTags()
        coEvery {
            bulkRepository.run(match { it.action == "add_tag" })
        } returns Result.failure(ApiError.Client(500, "server error"))

        val vm = viewModel()
        advanceUntilIdle()

        vm.toggle(1)
        vm.toggle(2)
        vm.run("add_tag", tagId = "tag-1")
        advanceUntilIdle()

        assertEquals(setOf(1, 2), vm.uiState.value.selected)
        assertEquals("server error", vm.uiState.value.error)
    }

    @Test
    fun `loads circles and tags for the pickers`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts() } returns Result.success(threeContacts)
        stubCirclesAndTags(
            circles = listOf(Circle(id = "c-1", name = "Book club"), Circle(id = "c-2", name = "Family")),
            tags = listOf(Tag(id = "t-1", name = "VIP")),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertEquals(2, vm.uiState.value.circles.size)
        assertEquals(1, vm.uiState.value.tags.size)
    }
}
