package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactGalleryViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ContactImageRepository>()

    private val image = ContactImage(id = 7, contactId = 5, thumbnailUrl = "/thumb")

    @Test
    fun `setContact loads the gallery`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(5) } returns Result.success(listOf(image))

        val vm = ContactGalleryViewModel(repository)
        vm.setContact(5)
        advanceUntilIdle()

        assertEquals(listOf(7), vm.uiState.value.images.map { it.id })
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `list failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(5) } returns Result.failure(ApiError.Client(500, "boom"))

        val vm = ContactGalleryViewModel(repository)
        vm.setContact(5)
        advanceUntilIdle()

        assertEquals("boom", vm.uiState.value.error?.message)
    }

    @Test
    fun `uploadAll posts every picked image and refetches`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.list(5) } returns Result.success(emptyList())
        coEvery { repository.upload(5, any(), any(), any()) } returns Result.success(image)

        val vm = ContactGalleryViewModel(repository)
        vm.setContact(5)
        advanceUntilIdle()

        vm.uploadAll(
            listOf(
                PickedContactImage("a.jpg", "image/jpeg", byteArrayOf(1)),
                PickedContactImage("b.jpg", "image/jpeg", byteArrayOf(2))
            )
        )
        advanceUntilIdle()

        coVerify(exactly = 2) { repository.upload(5, any(), any(), any()) }
        coVerify(exactly = 2) { repository.list(5) }
        assertTrue(!vm.uiState.value.isUploading)
    }

    @Test
    fun `delete removes the row locally`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list(5) } returns Result.success(listOf(image))
        coEvery { repository.delete(5, 7) } returns Result.success(Unit)

        val vm = ContactGalleryViewModel(repository)
        vm.setContact(5)
        advanceUntilIdle()

        vm.delete(image)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.images.isEmpty())
        assertNull(vm.uiState.value.deletingId)
    }
}
