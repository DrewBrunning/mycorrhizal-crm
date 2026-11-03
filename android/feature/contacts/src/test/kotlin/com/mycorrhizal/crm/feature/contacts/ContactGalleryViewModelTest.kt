package com.mycorrhizal.crm.feature.contacts

import com.mycorrhizal.crm.domain.repository.ContactImageRepository
import com.mycorrhizal.crm.domain.repository.ContactMainImage
import com.mycorrhizal.crm.model.network.ContactImage
import com.mycorrhizal.crm.model.network.ContactPhoto
import com.mycorrhizal.crm.model.network.ImageCrop
import com.mycorrhizal.crm.model.network.PhotoSource
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

    private val own = ContactPhoto(id = 7, thumbnailUrl = "/thumb")
    private val fromNote = ContactPhoto(
        id = 3,
        source = PhotoSource(PhotoSource.KIND_NOTE, 8, "Lunch"),
        thumbnailUrl = "/note-thumb"
    )

    private fun stubLoad(
        photos: List<ContactPhoto> = listOf(own),
        main: ContactMainImage = ContactMainImage(null, 3)
    ) {
        coEvery { repository.photos(5) } returns Result.success(photos)
        coEvery { repository.mainImage(5) } returns Result.success(main)
    }

    private suspend fun kotlinx.coroutines.test.TestScope.loaded(): ContactGalleryViewModel {
        val vm = ContactGalleryViewModel(repository)
        vm.setContact(5)
        advanceUntilIdle()
        return vm
    }

    @Test
    fun `setContact loads photos and the main image pointer`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(own, fromNote), ContactMainImage(7, 4))

        val vm = loaded()

        assertEquals(listOf(7, 3), vm.uiState.value.photos.map { it.id })
        assertEquals(7, vm.uiState.value.mainImageId)
        assertEquals(4, vm.uiState.value.revision)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `list failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.photos(5) } returns Result.failure(ApiError.Client(500, "boom"))
        coEvery { repository.mainImage(5) } returns Result.success(ContactMainImage(null, 1))

        val vm = loaded()

        assertEquals("boom", vm.uiState.value.error?.message)
    }

    @Test
    fun `a failing main image lookup does not hide the photos`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.photos(5) } returns Result.success(listOf(own))
        coEvery { repository.mainImage(5) } returns Result.failure(ApiError.Client(500, "x"))

        val vm = loaded()

        assertEquals(1, vm.uiState.value.photos.size)
        assertNull(vm.uiState.value.error)
        assertNull(vm.uiState.value.mainImageId)
    }

    @Test
    fun `uploadAll posts every picked image and refetches`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(emptyList())
        coEvery { repository.upload(5, any(), any(), any()) } returns
            Result.success(ContactImage(id = 7))
        val vm = loaded()

        vm.uploadAll(
            listOf(
                PickedContactImage("a.jpg", "image/jpeg", byteArrayOf(1)),
                PickedContactImage("b.jpg", "image/jpeg", byteArrayOf(2))
            )
        )
        advanceUntilIdle()

        coVerify(exactly = 2) { repository.upload(5, any(), any(), any()) }
        coVerify(exactly = 2) { repository.photos(5) }
        assertTrue(!vm.uiState.value.isUploading)
    }

    @Test
    fun `delete removes the row locally and refreshes the main pointer`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(own), ContactMainImage(7, 4))
        coEvery { repository.delete(5, 7) } returns Result.success(Unit)
        val vm = loaded()
        coEvery { repository.mainImage(5) } returns Result.success(ContactMainImage(null, 5))

        vm.delete(own)
        advanceUntilIdle()

        assertTrue(vm.uiState.value.photos.isEmpty())
        assertNull(vm.uiState.value.busyKey)
        assertNull(vm.uiState.value.mainImageId)
        assertEquals(5, vm.uiState.value.revision)
    }

    @Test
    fun `delete is refused for a note photo`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(listOf(fromNote))
        val vm = loaded()

        vm.delete(fromNote)
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.delete(any(), any()) }
        assertEquals(1, vm.uiState.value.photos.size)
    }

    @Test
    fun `setMain sends the current revision and stores the returned pointer`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(own), ContactMainImage(null, 3))
        coEvery { repository.setMainImage(5, 7, 3) } returns
            Result.success(ContactMainImage(7, 4))
        val vm = loaded()

        vm.setMain(own)
        advanceUntilIdle()

        assertEquals(7, vm.uiState.value.mainImageId)
        assertEquals(4, vm.uiState.value.revision)
    }

    @Test
    fun `setMain is refused for a note photo`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(listOf(fromNote))
        val vm = loaded()

        vm.setMain(fromNote)
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.setMainImage(any(), any(), any()) }
    }

    @Test
    fun `a stale setMain reloads the revision and reports the error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(own), ContactMainImage(null, 3))
        coEvery { repository.setMainImage(5, 7, 3) } returns
            Result.failure(ApiError.Client(412, "stale"))
        val vm = loaded()
        coEvery { repository.mainImage(5) } returns Result.success(ContactMainImage(null, 9))

        vm.setMain(own)
        advanceUntilIdle()

        assertEquals("stale", vm.uiState.value.error?.message)
        assertEquals(9, vm.uiState.value.revision)
        assertNull(vm.uiState.value.busyKey)
    }

    @Test
    fun `clearMain clears the pointer`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(listOf(own), ContactMainImage(7, 4))
        coEvery { repository.clearMainImage(5, 4) } returns
            Result.success(ContactMainImage(null, 5))
        val vm = loaded()

        vm.clearMain()
        advanceUntilIdle()

        assertNull(vm.uiState.value.mainImageId)
        assertEquals(5, vm.uiState.value.revision)
    }

    @Test
    fun `saveCrop patches the crop then refreshes the main revision`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(own), ContactMainImage(7, 4))
        val crop = ImageCrop(0.1, 0.0, 0.8, 1.0)
        coEvery { repository.setCrop(5, 7, crop) } returns Result.success(ContactImage(id = 7))
        val vm = loaded()
        coEvery { repository.mainImage(5) } returns Result.success(ContactMainImage(7, 5))

        vm.saveCrop(own, crop)
        advanceUntilIdle()

        coVerify { repository.setCrop(5, 7, crop) }
        assertEquals(5, vm.uiState.value.revision)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `adopt copies a note photo into the gallery and refetches`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        stubLoad(listOf(fromNote))
        coEvery { repository.adopt(5, "note", 8, 3) } returns Result.success(ContactImage(id = 12))
        val vm = loaded()

        vm.adopt(fromNote)
        advanceUntilIdle()

        coVerify { repository.adopt(5, "note", 8, 3) }
        coVerify(exactly = 2) { repository.photos(5) }
    }

    @Test
    fun `adopt is refused for an own photo`() = runTest(mainDispatcherRule.testDispatcher) {
        stubLoad(listOf(own))
        val vm = loaded()

        vm.adopt(own)
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.adopt(any(), any(), any(), any()) }
    }

    @Test
    fun `onErrorShown clears the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.photos(5) } returns Result.failure(ApiError.Client(500, "boom"))
        coEvery { repository.mainImage(5) } returns Result.success(ContactMainImage(null, 1))
        val vm = loaded()

        vm.onErrorShown()

        assertNull(vm.uiState.value.error)
    }
}
