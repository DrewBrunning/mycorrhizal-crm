package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordResponse
import com.mycorrhizal.crm.model.network.ContactSummary
import com.mycorrhizal.crm.model.network.Name
import com.mycorrhizal.crm.model.network.SyncInfo
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.mockk
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.runTest
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class ContactRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: ContactRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = ContactRepositoryImpl(apiClient, db.cachedContactDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun summary(id: Int, fn: String) = ContactSummary(id = id, fn = fn, firstname = fn)

    @Test
    fun `listContacts caches the page into Room on success`() = runTest {
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice"), summary(2, "Bob")),
                nextCursor = "",
            ),
        )

        val result = repository.listContacts()

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().contacts.size)
        // Cache mirrors the fetched page.
        val cached = db.cachedContactDao().getAll()
        assertEquals(2, cached.size)
        assertEquals("Alice", cached[0].fn)
    }

    @Test
    fun `listContacts applies incremental sync deletions to the cache`() = runTest {
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice")),
                nextCursor = "",
                sync = SyncInfo(mode = "incremental", incremental = listOf("2", "3")),
            ),
        )
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Stale Bob"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 3, fn = "Stale Carol"),
            ),
        )

        repository.listContacts()

        val remaining = db.cachedContactDao().getAll()
        assertEquals(1, remaining.size)
        assertEquals(1, remaining[0].id)
    }

    @Test
    fun `getContact caches the full record`() = runTest {
        val record = ContactRecordResponse(
            id = 5,
            uid = "u5",
            card = Card(name = Name(full = "Dana White"), emails = emptyList()),
            crm = CRMEnvelope(circles = listOf("friends")),
        )
        coEvery { apiClient.getContact(5) } returns Result.success(record)

        val result = repository.getContact(5)

        assertTrue(result.isSuccess)
        assertEquals("Dana White", result.getOrThrow().card?.name?.full)
        val cached = db.cachedContactDao().getById(5)
        assertEquals("u5", cached?.uid)
    }

    @Test
    fun `getContact falls back to cache on network failure`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 5,
                uid = "u5",
                card = Card(name = Name(full = "Dana White")),
            ),
        )
        coEvery { apiClient.getContact(5) } returns Result.failure(ApiError.Network(java.io.IOException("no net")))

        val result = repository.getContact(5)

        assertTrue(result.isSuccess)
        assertEquals("Dana White", result.getOrThrow().card?.name?.full)
    }

    @Test
    fun `getContact propagates failure when no cache exists`() = runTest {
        coEvery { apiClient.getContact(999) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val result = repository.getContact(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull()
        assertTrue(error is ApiError.Client)
        assertEquals(404, (error as ApiError.Client).code)
    }

    @Test
    fun `list refresh does not clobber cached detail`() = runTest {
        // Detail cached first with a full Card.
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Alice",
                card = Card(name = Name(full = "Alice Full Name")),
            ),
        )
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice")),
                nextCursor = "",
            ),
        )

        repository.listContacts()

        val cached = db.cachedContactDao().getById(1)
        // The summary page must not have wiped the cached card.
        assertEquals("Alice Full Name", cached?.card?.name?.full)
    }

    @Test
    fun `observeContacts surfaces cached summaries`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice", firstname = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Bob", firstname = "Bob"),
            ),
        )

        val observed = repository.observeContacts().first()

        assertEquals(2, observed.size)
        assertEquals("Alice", observed[0].fn)
    }
}
