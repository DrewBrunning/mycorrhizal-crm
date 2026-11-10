package com.mycorrhizal.crm.feature.shares

import androidx.lifecycle.SavedStateHandle
import com.mycorrhizal.crm.domain.repository.ShareGrantRepository
import com.mycorrhizal.crm.model.network.ErrorReason
import com.mycorrhizal.crm.model.network.ErrorReasonCode
import com.mycorrhizal.crm.model.network.ShareGrantContent
import com.mycorrhizal.crm.model.network.SharedNote
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

/** ADR 0042 decision 10 + issue #2293: no stale content, a revoked grant just disappears. */
class ShareGrantContentViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<ShareGrantRepository>()

    private fun handle(sections: String = "emails") =
        SavedStateHandle(mapOf("grantId" to "g1", "sections" to sections))

    private val noteContent = ShareGrantContent(
        grantId = "g1",
        entityType = "note",
        ownerUsername = "dana",
        note = SharedNote(content = "hello", date = "2026-11-01T00:00:00Z")
    )

    private fun notFound() = ApiError.Client(
        404,
        "Share grant not found",
        reason = ErrorReason(ErrorReasonCode.SHARE_GRANT_NOT_FOUND, emptyMap())
    )

    @Test
    fun `loads the content for the grant id on init`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.content("g1") } returns Result.success(noteContent)
        val vm = ShareGrantContentViewModel(repository, handle())
        advanceUntilIdle()

        assertEquals("hello", vm.uiState.value.content?.note?.content)
        assertFalse(vm.uiState.value.isLoading)
        assertNull(vm.uiState.value.error)
        assertFalse(vm.uiState.value.unavailable)
    }

    @Test
    fun `a contact grant is projected through the allowlist and the granted sections`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.content("g1") } returns Result.success(
            ShareGrantContent(
                grantId = "g1",
                entityType = "contact",
                ownerUsername = "dana",
                contact = mapOf(
                    "name" to mapOf("full" to "Dana White"),
                    "emails" to mapOf("e" to mapOf("address" to "d@x.io")),
                    "phones" to mapOf("p" to mapOf("number" to "123"))
                )
            )
        )
        val vm = ShareGrantContentViewModel(repository, handle(sections = "emails"))
        advanceUntilIdle()

        val contact = vm.uiState.value.contact!!
        assertEquals("Dana White", contact.displayName)
        assertEquals(listOf("emails"), contact.sections.map { it.token })
    }

    @Test
    fun `a failed read is a retryable error with no content`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.content("g1") } returnsMany listOf(
            Result.failure(ApiError.Server(503, "down")),
            Result.success(noteContent)
        )
        val vm = ShareGrantContentViewModel(repository, handle())
        advanceUntilIdle()

        assertNull(vm.uiState.value.content)
        assertNotNull(vm.uiState.value.error)
        assertFalse(vm.uiState.value.unavailable)

        vm.load()
        advanceUntilIdle()
        assertNull(vm.uiState.value.error)
        assertEquals("hello", vm.uiState.value.content?.note?.content)
    }

    @Test
    fun `reloading never shows the previous content while a new read is pending or failed`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.content("g1") } returnsMany listOf(
            Result.success(noteContent),
            Result.failure(ApiError.Network(java.io.IOException("offline")))
        )
        val vm = ShareGrantContentViewModel(repository, handle())
        advanceUntilIdle()
        assertNotNull(vm.uiState.value.content)

        vm.load()
        advanceUntilIdle()

        assertNull(vm.uiState.value.content)
        assertNull(vm.uiState.value.contact)
        assertNotNull(vm.uiState.value.error)
    }

    @Test
    fun `a revoked grant is unavailable, not an error to retry`() = runTest(
        mainDispatcherRule.testDispatcher
    ) {
        coEvery { repository.content("g1") } returns Result.failure(notFound())
        val vm = ShareGrantContentViewModel(repository, handle())
        advanceUntilIdle()

        assertTrue(vm.uiState.value.unavailable)
        assertNull(vm.uiState.value.content)
    }

    @Test
    fun `a missing grant id never calls the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        val vm = ShareGrantContentViewModel(repository, SavedStateHandle())
        advanceUntilIdle()

        coVerify(exactly = 0) { repository.content(any()) }
        assertTrue(vm.uiState.value.unavailable)
    }
}
