package com.mycorrhizal.crm.feature.tags

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.TagDetail
import com.mycorrhizal.crm.domain.repository.TagRepository
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag
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

class TagsViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val tagRepository = mockk<TagRepository>()

    @Test
    fun `loads tags on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.success(
            listOf(Tag(id = "t1", name = "Friend"), Tag(id = "t2", name = "Family")),
        )

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals(2, vm.uiState.value.tags.size)
        assertEquals("Friend", vm.uiState.value.tags[0].name)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error)
    }

    @Test
    fun `create adds the tag to the list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())
        coEvery { tagRepository.create("Family") } returns Result.success(Tag(id = "t9", name = "Family"))

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        var done = false
        vm.create("Family") { done = true }
        advanceUntilIdle()

        assertTrue(done)
        assertEquals(listOf("Family"), vm.uiState.value.tags.map { it.name })
    }

    @Test
    fun `blank name does not create`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.success(emptyList())

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        vm.create("   ")
        advanceUntilIdle()

        coVerify(exactly = 0) { tagRepository.create(any()) }
    }

    @Test
    fun `rename updates the tag in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.success(
            listOf(Tag(id = "t1", name = "Friend")),
        )
        coEvery { tagRepository.rename("t1", "Close Friend") } returns
            Result.success(Tag(id = "t1", name = "Close Friend"))

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        vm.rename("t1", "Close Friend")
        advanceUntilIdle()

        assertEquals(listOf("Close Friend"), vm.uiState.value.tags.map { it.name })
    }

    @Test
    fun `delete removes the tag`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.list(any(), any()) } returns Result.success(
            listOf(Tag(id = "t1", name = "Friend"), Tag(id = "t2", name = "Family")),
        )
        coEvery { tagRepository.delete("t1") } returns Result.success(Unit)

        val vm = TagsViewModel(tagRepository)
        advanceUntilIdle()

        vm.delete("t1")
        advanceUntilIdle()

        assertEquals(listOf("Family"), vm.uiState.value.tags.map { it.name })
    }
}

class TagDetailViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val tagRepository = mockk<TagRepository>()

    private fun viewModel(id: String = "t1"): TagDetailViewModel =
        TagDetailViewModel(tagRepository, SavedStateHandle(mapOf("tagId" to id)))

    @Test
    fun `loads tag and contacts on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.getWithContacts("t1") } returns Result.success(
            TagDetail(
                tag = Tag(id = "t1", name = "Friend"),
                contacts = listOf(ContactTag(id = 1, tagId = "t1", contactVCardUid = "uid-1")),
            ),
        )

        val vm = viewModel()
        advanceUntilIdle()

        assertFalse(vm.uiState.value.isLoading)
        assertEquals("Friend", vm.uiState.value.tag?.name)
        assertEquals(1, vm.uiState.value.contacts.size)
        assertEquals("uid-1", vm.uiState.value.contacts[0].contactVCardUid)
    }

    @Test
    fun `addContact appends to the contacts list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.getWithContacts("t1") } returns Result.success(
            TagDetail(tag = Tag(id = "t1", name = "Friend"), contacts = emptyList()),
        )
        coEvery { tagRepository.addContact("t1", "uid-2") } returns Result.success(
            ContactTag(id = 2, tagId = "t1", contactVCardUid = "uid-2"),
        )

        val vm = viewModel()
        advanceUntilIdle()

        vm.addContact("uid-2")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.contacts.map { it.contactVCardUid })
    }

    @Test
    fun `removeContact drops the contact`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { tagRepository.getWithContacts("t1") } returns Result.success(
            TagDetail(
                tag = Tag(id = "t1", name = "Friend"),
                contacts = listOf(
                    ContactTag(id = 1, tagId = "t1", contactVCardUid = "uid-1"),
                    ContactTag(id = 2, tagId = "t1", contactVCardUid = "uid-2"),
                ),
            ),
        )
        coEvery { tagRepository.removeContact("t1", "uid-1") } returns Result.success(Unit)

        val vm = viewModel()
        advanceUntilIdle()

        vm.removeContact("uid-1")
        advanceUntilIdle()

        assertEquals(listOf("uid-2"), vm.uiState.value.contacts.map { it.contactVCardUid })
    }
}
