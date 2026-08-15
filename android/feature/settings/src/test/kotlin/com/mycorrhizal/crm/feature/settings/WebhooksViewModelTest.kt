package com.mycorrhizal.crm.feature.settings

import com.mycorrhizal.crm.domain.repository.WebhookRepository
import com.mycorrhizal.crm.model.network.Webhook
import com.mycorrhizal.crm.model.network.WebhookCreateResponse
import com.mycorrhizal.crm.model.network.WebhookDelivery
import com.mycorrhizal.crm.model.network.WebhookInput
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

class WebhooksViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val repository = mockk<WebhookRepository>()

    private fun webhook(id: Int, name: String = "Hook $id", isActive: Boolean = true) =
        Webhook(id = id, name = name, url = "https://example.com/$id", events = listOf("contact.created"), isActive = isActive)

    @Test
    fun `load populates the webhook list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(webhook(1), webhook(2)))
        val vm = WebhooksViewModel(repository)
        advanceUntilIdle()

        val state = vm.uiState.value
        assertTrue(!state.isLoading)
        assertEquals(2, state.webhooks.size)
        assertEquals("Hook 1", state.webhooks[0].name)
    }

    @Test
    fun `load failure surfaces the error`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.failure(ApiError.Server(500, "boom"))
        val vm = WebhooksViewModel(repository)
        advanceUntilIdle()

        assertEquals("Server error (500)", vm.uiState.value.error)
    }

    @Test
    fun `create prepends the new webhook and reveals the secret once`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(emptyList())
            coEvery { repository.create(any()) } returns Result.success(
                WebhookCreateResponse(id = 9, name = "New", url = "https://example.com/new", events = listOf("note.created"), isActive = true, secret = "s3cret"),
            )
            val vm = WebhooksViewModel(repository)
            advanceUntilIdle()

            vm.save(WebhookInput(name = "New", url = "https://example.com/new", events = listOf("note.created"), isActive = true), editingId = null)
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals(1, state.webhooks.size)
            assertEquals(9, state.webhooks[0].id)
            assertEquals("s3cret", state.createdWebhook?.secret)
        }

    @Test
    fun `update replaces the edited webhook in place`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(webhook(1), webhook(2)))
        coEvery { repository.update(2, any()) } returns Result.success(
            webhook(2, name = "Renamed", isActive = false),
        )
        val vm = WebhooksViewModel(repository)
        advanceUntilIdle()

        vm.save(WebhookInput(name = "Renamed", url = "https://example.com/2", events = listOf("contact.created"), isActive = false), editingId = 2)
        advanceUntilIdle()

        assertEquals("Renamed", vm.uiState.value.webhooks[1].name)
        assertTrue(!vm.uiState.value.webhooks[1].isActive)
        assertNull(vm.uiState.value.createdWebhook)
    }

    @Test
    fun `delete removes the webhook after a confirmed call`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(webhook(1), webhook(2)))
        coEvery { repository.delete(1) } returns Result.success(Unit)
        val vm = WebhooksViewModel(repository)
        advanceUntilIdle()

        vm.delete(vm.uiState.value.webhooks[0])
        advanceUntilIdle()

        coVerify { repository.delete(1) }
        assertEquals(listOf(2), vm.uiState.value.webhooks.map { it.id })
    }

    @Test
    fun `test with a 2xx delivery shows success and prepends the delivery`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(listOf(webhook(1)))
            coEvery { repository.test(1) } returns Result.success(
                WebhookDelivery(id = 10, webhookId = 1, eventType = "test", statusCode = 200),
            )
            val vm = WebhooksViewModel(repository)
            advanceUntilIdle()

            vm.test(vm.uiState.value.webhooks[0])
            advanceUntilIdle()

            val state = vm.uiState.value
            assertEquals("200", state.message)
            assertNull(state.error)
            assertEquals(200, state.deliveries[1]?.first()?.statusCode)
            assertTrue(state.expandedIds.contains(1))
        }

    @Test
    fun `test with a failed delivery shows the error distinctly`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(listOf(webhook(1)))
            coEvery { repository.test(1) } returns Result.success(
                WebhookDelivery(id = 11, webhookId = 1, eventType = "test", statusCode = 500, error = "connection refused"),
            )
            val vm = WebhooksViewModel(repository)
            advanceUntilIdle()

            vm.test(vm.uiState.value.webhooks[0])
            advanceUntilIdle()

            val state = vm.uiState.value
            assertNull(state.message)
            assertEquals("connection refused", state.error)
            assertEquals(500, state.deliveries[1]?.first()?.statusCode)
        }

    @Test
    fun `expanding a webhook loads its delivery history once`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { repository.list() } returns Result.success(listOf(webhook(1)))
        coEvery { repository.deliveries(1) } returns Result.success(
            listOf(WebhookDelivery(id = 1, webhookId = 1, eventType = "contact.created", statusCode = 200)),
        )
        val vm = WebhooksViewModel(repository)
        advanceUntilIdle()

        vm.toggleDeliveries(1)
        advanceUntilIdle()
        vm.toggleDeliveries(1)
        advanceUntilIdle()

        assertEquals(1, vm.uiState.value.deliveries[1]?.size)
        coVerify(exactly = 1) { repository.deliveries(1) }
    }

    @Test
    fun `delivery history load failure is a transient error, not a crash`() =
        runTest(mainDispatcherRule.testDispatcher) {
            coEvery { repository.list() } returns Result.success(listOf(webhook(1)))
            coEvery { repository.deliveries(1) } returns Result.failure(ApiError.Server(500, "boom"))
            val vm = WebhooksViewModel(repository)
            advanceUntilIdle()

            vm.toggleDeliveries(1)
            advanceUntilIdle()

            assertEquals("Server error (500)", vm.uiState.value.error)
            assertTrue(vm.uiState.value.webhooks.isNotEmpty())
        }
}
