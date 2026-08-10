package com.mycorrhizal.crm.feature.contacts

import app.cash.turbine.test
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.testing.MainDispatcherRule
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.flow.emptyFlow
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test

class ContactListViewModelTest {

    @get:Rule
    val mainDispatcherRule = MainDispatcherRule()

    private val contactRepository = mockk<ContactRepository>()

    private fun page(vararg contacts: ContactSummary, nextCursor: String? = null): ContactsPage =
        ContactsPage(contacts = contacts.toList(), nextCursor = nextCursor, limit = 50, sync = null)

    @Test
    fun `initial load fetches the first page`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice")))

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertEquals(1, state.contacts.size)
        assertEquals("Alice", state.contacts[0].fn)
    }

    @Test
    fun `error state sets the display message`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.failure(ApiError.Server(500, "boom"))

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertFalse(state.isLoading)
        assertEquals("Server error (500)", state.error)
    }

    @Test
    fun `401 error emits ForceLogout`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.failure(ApiError.Client(401, "Invalid token"))

        val viewModel = ContactListViewModel(contactRepository)
        viewModel.events.test {
            advanceUntilIdle()
            val event = awaitItem()
            assertTrue(event is ContactListEvent.ForceLogout)
        }
    }

    @Test
    fun `cursor pagination appends to the existing list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), nextCursor = "cursor2"))
        coEvery { contactRepository.listContacts(cursor = "cursor2", limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob"), nextCursor = null))

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.contacts.size)

        viewModel.loadNextPage()
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals(2, state.contacts.size)
        assertFalse(state.pagination.hasMore)
    }

    @Test
    fun `loadNextPage is a no-op while already loading`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice"), nextCursor = "cursor2"))
        coEvery { contactRepository.listContacts(cursor = "cursor2", limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 2, fn = "Bob"), nextCursor = null))

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()

        viewModel.loadNextPage()
        viewModel.loadNextPage() // second call while the first page request is in flight
        advanceUntilIdle()

        // Exactly one page-2 fetch appends Bob; the second call bailed on isLoadingMore.
        assertEquals(2, viewModel.uiState.value.contacts.size)
    }

    @Test
    fun `search query is forwarded to the repository`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page())
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "ali") } returns
            Result.success(page(ContactSummary(id = 3, fn = "Alicia")))

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()

        viewModel.onSearchQueryChange("ali")
        advanceUntilIdle()

        val state = viewModel.uiState.value
        assertEquals("ali", state.searchQuery)
        assertEquals(1, state.contacts.size)
        assertEquals("Alicia", state.contacts[0].fn)
    }

    @Test
    fun `clearing the search reloads the full list`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 1, fn = "Alice")))
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = "bob") } returns
            Result.success(page())

        val viewModel = ContactListViewModel(contactRepository)
        advanceUntilIdle()
        viewModel.onSearchQueryChange("bob")
        advanceUntilIdle()
        assertEquals(0, viewModel.uiState.value.contacts.size)

        viewModel.onSearchQueryChange("")
        advanceUntilIdle()
        assertEquals(1, viewModel.uiState.value.contacts.size)
    }

    @Test
    fun `clicking a contact emits navigation event`() = runTest(mainDispatcherRule.testDispatcher) {
        coEvery { contactRepository.listContacts(cursor = null, limit = 50, search = null) } returns
            Result.success(page(ContactSummary(id = 42, fn = "Alice")))

        val viewModel = ContactListViewModel(contactRepository)
        viewModel.events.test {
            advanceUntilIdle()
            viewModel.onContactClick(42)
            val event = awaitItem()
            assertTrue(event is ContactListEvent.NavigateToContact)
            assertEquals(42, (event as ContactListEvent.NavigateToContact).contactId)
        }
    }
}
