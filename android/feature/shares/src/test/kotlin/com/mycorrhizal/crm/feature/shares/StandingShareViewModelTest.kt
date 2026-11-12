package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ContactShareRepository
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.OutgoingShareGrant
import com.mycorrhizal.crm.model.network.ShareGrantEntityTypes
import com.mycorrhizal.crm.model.network.ShareGrantInput
import com.mycorrhizal.crm.model.network.UserDirectoryEntry
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import java.time.Instant
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** Issue #2518: standing shares of a note, an activity or a circle (ADR 0042 decision 12). */
class StandingShareViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val directory = mockk<ContactShareRepository>()
    private val grants = mockk<ShareGrantRepository>()

    private fun viewModel(
        type: String = ShareGrantEntityTypes.NOTE,
        id: String = "entity-1"
    ): StandingShareViewModel {
        coEvery { directory.userDirectory() } returns Result.success(
            listOf(UserDirectoryEntry(id = 9, username = "casey"))
        )
        return StandingShareViewModel(
            directory,
            grants,
            SavedStateHandle(mapOf("entityType" to type, "entityId" to id))
        )
    }

    private val types = listOf(
        ShareGrantEntityTypes.NOTE,
        ShareGrantEntityTypes.ACTIVITY,
        ShareGrantEntityTypes.CIRCLE
    )

    @Test
    fun `loads the recipient directory and exposes the entity type`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel(ShareGrantEntityTypes.CIRCLE)
        advanceUntilIdle()

        assertEquals(listOf("casey"), vm.uiState.value.recipients.map { it.username })
        assertEquals(ShareGrantEntityTypes.CIRCLE, vm.uiState.value.entityType)
        assertFalse(vm.uiState.value.isLoading)
        assertNull(vm.uiState.value.error)
    }

    @Test
    fun `a failed directory load surfaces the error`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        val vm = viewModel()
        coEvery { directory.userDirectory() } returns Result.failure(ApiError.Client(404, "gone"))
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.uiState.value.error is ApiError.Client)
        assertTrue(vm.uiState.value.recipients.isEmpty())
    }

    @Test
    fun `each type creates a grant with no sections and no expiry`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        types.forEach { type ->
            coEvery { grants.create(any()) } returns Result.success(OutgoingShareGrant("g-$type"))
            val vm = viewModel(type, "id-$type")
            advanceUntilIdle()

            vm.selectRecipient(9)
            assertTrue(vm.uiState.value.canShare)
            vm.share()
            advanceUntilIdle()

            coVerify(exactly = 1) {
                grants.create(
                    ShareGrantInput(
                        toUserId = 9,
                        entityType = type,
                        entityId = "id-$type",
                        sections = emptyList(),
                        expiresAt = null
                    )
                )
            }
            assertTrue("$type shared", vm.uiState.value.shared)
            assertFalse(vm.uiState.value.isSharing)
        }
    }

    @Test
    fun `each type refuses to share without a recipient`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        types.forEach { type ->
            val vm = viewModel(type)
            advanceUntilIdle()

            assertFalse(vm.uiState.value.canShare)
            vm.share()
            advanceUntilIdle()

            assertFalse(vm.uiState.value.shared)
        }
        coVerify(exactly = 0) { grants.create(any()) }
    }

    @Test
    fun `each type keeps the error and stays on the screen when the create fails`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        types.forEach { type ->
            coEvery { grants.create(any()) } returns Result.failure(ApiError.Client(409, "dup"))
            val vm = viewModel(type)
            advanceUntilIdle()
            vm.selectRecipient(9)
            vm.share()
            advanceUntilIdle()

            assertFalse("$type shared", vm.uiState.value.shared)
            assertFalse(vm.uiState.value.isSharing)
            assertTrue(vm.uiState.value.error is ApiError.Client)
            vm.onErrorShown()
            assertNull(vm.uiState.value.error)
        }
    }

    @Test
    fun `an expiry preset is turned into an instant from the injected clock`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { grants.create(any()) } returns Result.success(OutgoingShareGrant("g1"))
        val vm = viewModel()
        advanceUntilIdle()
        vm.now = { Instant.parse("2026-11-10T00:00:00Z") }
        vm.setExpiryDays(90)
        vm.selectRecipient(9)
        vm.share()
        advanceUntilIdle()

        coVerify { grants.create(match { it.expiresAt == "2027-02-08T00:00:00Z" }) }
    }

    @Test
    fun `an unknown entity type or a blank id never reaches the repository`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        listOf(
            viewModel("contact", "x"),
            viewModel("bogus", "x"),
            viewModel(ShareGrantEntityTypes.NOTE, "")
        ).forEach { vm ->
            advanceUntilIdle()
            vm.selectRecipient(9)
            vm.share()
            advanceUntilIdle()
            assertNotNull(vm.uiState.value.errorRes)
            assertFalse(vm.uiState.value.shared)
        }
        coVerify(exactly = 0) { grants.create(any()) }
    }
}
