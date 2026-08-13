package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.domain.repository.ContactsPage
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.ContactRecordInput
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

    @Test
    fun `createContact caches the created record`() = runTest {
        val created = ContactRecordResponse(id = 9, uid = "u9", card = Card(name = Name(full = "Carol King")))
        coEvery { apiClient.createContact(any()) } returns Result.success(created)
        val input = ContactRecordInput(card = Card(name = Name(full = "Carol King")))

        val result = repository.createContact(input)

        assertTrue(result.isSuccess)
        assertEquals(9, result.getOrThrow().id)
        val cached = db.cachedContactDao().getById(9)
        assertEquals("u9", cached?.uid)
    }

    @Test
    fun `updateContact caches the updated record`() = runTest {
        val updated = ContactRecordResponse(id = 9, uid = "u9", card = Card(name = Name(full = "Carol King Renamed")))
        coEvery { apiClient.updateContact(9, any()) } returns Result.success(updated)
        val input = ContactRecordInput(card = Card(name = Name(full = "Carol King Renamed")))

        val result = repository.updateContact(9, input)

        assertTrue(result.isSuccess)
        val cached = db.cachedContactDao().getById(9)
        assertEquals("Carol King Renamed", cached?.card?.name?.full)
    }

    @Test
    fun `createContact propagates a validation failure`() = runTest {
        coEvery { apiClient.createContact(any()) } returns Result.failure(
            ApiError.Client(400, "at least one name component (kind=given) or name.full is required"),
        )

        val result = repository.createContact(ContactRecordInput(card = Card()))

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(400, (error as ApiError.Client).code)
    }

    @Test
    fun `searchLocal matches cached rows via FTS`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "David Smith"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Bob Jones"),
            ),
        )

        val result = repository.searchLocal("dav")

        assertEquals(1, result.size)
        assertEquals("David Smith", result[0].fn)
    }

    @Test
    fun `searchLocal finds a punctuated phone number by its bare digits`() = runTest {
        // T76: offline search must find a contact by phone number regardless of the stored
        // punctuation — the exact reported bug (offline search can't find a contact by phone).
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(
                    id = 1,
                    fn = "Dana White",
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(listOf("(800) 555-1234")),
                ),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Bob Jones"),
            ),
        )

        val result = repository.searchLocal("8005551234")

        assertEquals(1, result.size)
        assertEquals("Dana White", result[0].fn)
    }

    @Test
    fun `searchLocal finds a contact by querying with a country code the stored number lacks`() = runTest {
        // The specific thing PhoneKey's digits-vs-key duality buys over a bare/unrestricted FTS
        // prefix match: "18005551234" is not a prefix of the stored "8005551234" token or vice
        // versa, so only the OR'd key match (via phoneMatchExpr) finds it. This is the part a
        // weaker "does *any* phone query work" test wouldn't catch regressing.
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(
                    id = 1,
                    fn = "Dana White",
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(listOf("(800) 555-1234")),
                ),
            ),
        )

        val result = repository.searchLocal("+1 (800) 555-1234")

        assertEquals(1, result.size)
        assertEquals("Dana White", result[0].fn)
    }

    @Test
    fun `searchLocal with a blank query returns the whole cache`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Bob"),
            ),
        )

        val result = repository.searchLocal("")

        assertEquals(2, result.size)
    }

    @Test
    fun `searchLocal strips FTS operator characters from the query`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "David Smith")),
        )

        // A quote would otherwise break the FTS MATCH expression.
        val result = repository.searchLocal("\"smith\"")

        assertEquals(1, result.size)
        assertEquals("David Smith", result[0].fn)
    }

    @Test
    fun `searchLocal tolerates unbalanced parens and NEAR without crashing`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "David Smith")),
        )

        // Unbalanced parens or a bare NEAR throw "malformed MATCH expression"
        // in FTS4; searchLocal must sanitize (or fall back to LIKE), never crash.
        val parens = repository.searchLocal("Wilson (office")
        val near = repository.searchLocal("NEAR")
        val smith = repository.searchLocal("smith")

        // No exception; results are the sanitized MATCH (multi-token returns 0
        // for a single-token row) or the full cache for a fully-stripped query.
        assertEquals(0, parens.size)
        assertEquals(1, near.size)
        assertEquals("David Smith", smith[0].fn)
    }
}
