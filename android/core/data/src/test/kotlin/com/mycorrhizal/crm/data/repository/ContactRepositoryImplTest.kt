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
import org.junit.Assert.assertNull
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

    /** A wire list/feed page (the repository consumes the network envelope). */
    private fun networkPage(
        contacts: List<ContactSummary>,
        nextCursor: String? = null,
        sync: SyncInfo? = null,
    ) = com.mycorrhizal.crm.model.network.ContactsPage(
        contacts = contacts,
        nextCursor = nextCursor,
        limit = 100,
        sync = sync,
    )

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
    fun `listContacts caches the favorite flag into Room`() = runTest {
        // Issue #212: the star must survive the cache mirror (and thus the
        // offline list) exactly as the server sent it.
        coEvery { apiClient.listContacts(any(), any(), any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice").copy(isFavorite = true), summary(2, "Bob")),
                nextCursor = "",
            ),
        )

        repository.listContacts()

        val cached = db.cachedContactDao().getAll().associateBy { it.id }
        assertEquals(true, cached[1]?.isFavorite)
        assertEquals(false, cached[2]?.isFavorite)
    }

    @Test
    fun `listContacts forwards the circle filter and archived toggle to the client`() = runTest {
        // M23: the list's filter dropdown + archived switch are only as good as the
        // repository's willingness to pass them through — this pins the data layer.
        coEvery { apiClient.listContacts(any(), any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(contacts = emptyList(), nextCursor = ""),
        )

        repository.listContacts(circle = "Friends", includeArchived = true)

        io.mockk.coVerify(exactly = 1) {
            apiClient.listContacts(
                cursor = null,
                limit = 50,
                search = null,
                includeArchived = true,
                circle = "Friends",
            )
        }
    }

    @Test
    fun `listContacts leaves the circle filter and archived toggle null by default`() = runTest {
        // A plain list request must not send include_archived/circle (the backend treats
        // absent as the default) — proving the defaults are null, not false/empty.
        coEvery { apiClient.listContacts(any(), any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(contacts = emptyList(), nextCursor = ""),
        )

        repository.listContacts()

        io.mockk.coVerify(exactly = 1) {
            apiClient.listContacts(
                cursor = null,
                limit = 50,
                search = null,
                includeArchived = null,
                circle = null,
                favorites = null,
            )
        }
    }

    // --- Issue #212: the favorites filter + favorite/unfavorite (web #173) ---

    @Test
    fun `listContacts forwards the favorites filter to the client`() = runTest {
        // MockK records the compiled default for trailing params, so the stub
        // must name the favorites=true it wants matched — a 6-any() stub would
        // only ever answer favorites=null calls.
        coEvery { apiClient.listContacts(any(), any(), any(), any(), any(), any(), true) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(contacts = emptyList(), nextCursor = ""),
        )

        repository.listContacts(favorites = true)

        io.mockk.coVerify(exactly = 1) {
            apiClient.listContacts(
                cursor = null,
                limit = 50,
                search = null,
                includeArchived = null,
                circle = null,
                favorites = true,
            )
        }
    }

    @Test
    fun `listContacts leaves the favorites filter null by default`() = runTest {
        // A plain list request must not send favorites (the backend treats
        // absent as "all contacts", not favorites-only).
        coEvery { apiClient.listContacts(any(), any(), any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(contacts = emptyList(), nextCursor = ""),
        )

        repository.listContacts()

        io.mockk.coVerify(exactly = 1) {
            apiClient.listContacts(
                cursor = null,
                limit = 50,
                search = null,
                includeArchived = null,
                circle = null,
                favorites = null,
            )
        }
    }

    @Test
    fun `favoriteContact flips the cached favorite flag on success`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White", isFavorite = false))
        coEvery { apiClient.favoriteContact(5) } returns Result.success(Unit)

        val result = repository.favoriteContact(5)

        assertTrue(result.isSuccess)
        assertEquals(true, db.cachedContactDao().getById(5)?.isFavorite)
        io.mockk.coVerify(exactly = 1) { apiClient.favoriteContact(5) }
    }

    @Test
    fun `unfavoriteContact flips the cached favorite flag back`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White", isFavorite = true))
        coEvery { apiClient.unfavoriteContact(5) } returns Result.success(Unit)

        val result = repository.unfavoriteContact(5)

        assertTrue(result.isSuccess)
        assertEquals(false, db.cachedContactDao().getById(5)?.isFavorite)
        io.mockk.coVerify(exactly = 1) { apiClient.unfavoriteContact(5) }
    }

    @Test
    fun `favoriteContact failure leaves the cached flag unchanged`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White", isFavorite = false))
        coEvery { apiClient.favoriteContact(5) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.favoriteContact(5)

        assertTrue(result.isFailure)
        assertEquals(false, db.cachedContactDao().getById(5)?.isFavorite)
    }

    // --- Issue #959: the T17 ?since= tombstone feed (offline mirror deletes) ---

    @Test
    fun `listContacts never deletes from the sync incremental collection names`() = runTest {
        // The exact #959 bug: sync.incremental is the static list of COLLECTION
        // NAMES ("contacts", "notes", …), not deleted ids — parsing them as ids
        // always yielded an empty list, and (worse) the mirror never applied a
        // real tombstone. A genuine list response must leave the mirror alone.
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Bob"),
            ),
        )
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice")),
                nextCursor = "",
                sync = SyncInfo(
                    mode = "incremental",
                    incremental = listOf("contacts", "notes", "activities"),
                ),
            ),
        )

        repository.listContacts()

        assertEquals(listOf(1, 2), db.cachedContactDao().getAll().map { it.id }.sorted())
    }

    @Test
    fun `syncContacts bootstrap reconciles a small server snapshot`() = runTest {
        // A live set smaller than one feed page is the server's whole truth, so
        // a cached row missing from it is a server-side delete.
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Deleted Bob"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 3, fn = "Carol"),
            ),
        )
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns Result.success(
            networkPage(listOf(summary(1, "Alice"), summary(3, "Carol")), nextCursor = ""),
        )

        val result = repository.syncContacts()

        assertTrue(result.isSuccess)
        assertEquals(listOf(1, 3), db.cachedContactDao().getAll().map { it.id }.sorted())
    }

    @Test
    fun `syncContacts applies a change-feed tombstone and removes the cached row`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Gone"),
            ),
        )
        // Bootstrap: the page has a next page, so its trailing cursor becomes
        // the feed watermark rather than triggering a reconcile.
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns Result.success(
            networkPage(listOf(summary(1, "Alice"), summary(2, "Gone")), nextCursor = "cursor-1"),
        )
        repository.syncContacts()

        // The change feed returns contact 2 as a deletion tombstone.
        coEvery { apiClient.listContacts(since = "cursor-1", limit = 100) } returns Result.success(
            networkPage(listOf(summary(2, "Gone").copy(deleted = true))),
        )
        val result = repository.syncContacts()

        assertTrue(result.isSuccess)
        assertEquals(listOf(1), db.cachedContactDao().getAll().map { it.id })
    }

    @Test
    fun `syncContacts keeps an archived cached row present in the live snapshot`() = runTest {
        // Bootstrapping with include_archived=true is what stops a reconcile
        // from treating an archived cached row as a deleted one.
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Archived", archived = true),
        )
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns Result.success(
            networkPage(listOf(summary(1, "Archived").copy(archived = true))),
        )

        repository.syncContacts()

        assertTrue(db.cachedContactDao().getById(1) != null)
    }

    @Test
    fun `syncContacts drains every feed page and advances the watermark`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Gone"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 3, fn = "Gone Too"),
            ),
        )
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns Result.success(
            networkPage(listOf(summary(1, "Alice")), nextCursor = "c1"),
        )
        repository.syncContacts()

        coEvery { apiClient.listContacts(since = "c1", limit = 100) } returns Result.success(
            networkPage(listOf(summary(2, "Gone").copy(deleted = true)), nextCursor = "c2"),
        )
        coEvery { apiClient.listContacts(since = "c2", limit = 100) } returns Result.success(
            networkPage(listOf(summary(3, "Gone Too").copy(deleted = true)), nextCursor = ""),
        )

        val result = repository.syncContacts()

        assertTrue(result.isSuccess)
        // Both tombstones (2 and 3) are gone; the bootstrap's live row 1 stays.
        assertEquals(listOf(1), db.cachedContactDao().getAll().map { it.id })
        // Both feed pages were requested — draining doesn't stop at the first.
        io.mockk.coVerify(exactly = 1) { apiClient.listContacts(since = "c1", limit = 100) }
        io.mockk.coVerify(exactly = 1) { apiClient.listContacts(since = "c2", limit = 100) }
    }

    @Test
    fun `syncContacts preserves cached full detail for a live feed row`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Alice",
                card = Card(name = Name(full = "Alice Full Name")),
            ),
        )
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns Result.success(
            networkPage(listOf(summary(1, "Alice")), nextCursor = "c1"),
        )
        repository.syncContacts()
        coEvery { apiClient.listContacts(since = "c1", limit = 100) } returns Result.success(
            networkPage(listOf(summary(1, "Alice"))),
        )

        repository.syncContacts()

        // The feed row is summary-only; the cached card must survive.
        assertEquals("Alice Full Name", db.cachedContactDao().getById(1)?.card?.name?.full)
    }

    @Test
    fun `syncContacts reboots from a 410 Gone feed cursor`() = runTest {
        db.cachedContactDao().upsertAll(
            listOf(
                com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"),
                com.mycorrhizal.crm.data.local.CachedContact(id = 2, fn = "Gone"),
            ),
        )
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returnsMany listOf(
            Result.success(networkPage(listOf(summary(1, "Alice"), summary(2, "Gone")), nextCursor = "old-cursor")),
            // After the 410 the server's live set no longer holds contact 2.
            Result.success(networkPage(listOf(summary(1, "Alice")), nextCursor = "")),
        )
        repository.syncContacts()

        coEvery { apiClient.listContacts(since = "old-cursor", limit = 100) } returns
            Result.failure(ApiError.Client(410, "full resync required"))

        val result = repository.syncContacts()

        assertTrue(result.isSuccess)
        assertEquals(listOf(1), db.cachedContactDao().getAll().map { it.id })
    }

    @Test
    fun `syncContacts propagates a network failure without touching the cache`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Alice"))
        coEvery { apiClient.listContacts(limit = 100, includeArchived = true) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))

        val result = repository.syncContacts()

        assertTrue(result.isFailure)
        assertEquals(1, db.cachedContactDao().getById(1)?.id)
    }

    @Test
    fun `resolveByUid maps the response by VCardUID`() = runTest {
        coEvery {
            apiClient.listContacts(vcardUids = listOf("u1", "u2"), includeArchived = true)
        } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(
                    ContactSummary(id = 1, uid = "u1", fn = "Alice"),
                    ContactSummary(id = 2, uid = "u2", fn = "Bob"),
                ),
                nextCursor = "",
            ),
        )

        val result = repository.resolveByUid(listOf("u1", "u2"))

        assertTrue(result.isSuccess)
        val map = result.getOrThrow()
        assertEquals(2, map.size)
        assertEquals("Alice", map["u1"]?.fn)
        assertEquals("Bob", map["u2"]?.fn)
    }

    @Test
    fun `resolveByUid requests archived contacts so a reference to one still resolves`() = runTest {
        // Web's getContactsByUid sends include_archived=true (an edge/audit
        // reference can point at an archived contact); the Android mirror must
        // too, or an archived contact's uid silently fails to resolve.
        coEvery { apiClient.listContacts(vcardUids = any(), includeArchived = true) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(ContactSummary(id = 3, uid = "u3", fn = "Carol", archived = true)),
                nextCursor = "",
            ),
        )

        val result = repository.resolveByUid(listOf("u3"))

        assertTrue(result.isSuccess)
        assertEquals("Carol", result.getOrThrow()["u3"]?.fn)
        io.mockk.coVerify(exactly = 1) {
            apiClient.listContacts(vcardUids = listOf("u3"), includeArchived = true)
        }
    }

    @Test
    fun `resolveByUid with empty input short-circuits without a network call`() = runTest {
        val result = repository.resolveByUid(emptyList())

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().isEmpty())
        io.mockk.coVerify(exactly = 0) { apiClient.listContacts(any(), any(), any(), any(), any()) }
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
    fun `getContactIdsMissingPhoneIndex returns list-synced rows never detail-fetched`() = runTest {
        // A plain list-page row only ever carries primaryPhone (see toCached()).
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Alice").copy(primaryPhone = "555-0100")),
                nextCursor = "",
            ),
        )

        repository.listContacts()

        assertEquals(listOf(1), repository.getContactIdsMissingPhoneIndex(limit = 10))
    }

    @Test
    fun `issue 1122 a detail fetch hydrates the full phone index and clears it from the backfill queue`() = runTest {
        // Simulates the ContactPhoneIndexBackfillWorker loop end to end: a
        // contact synced only via the list endpoint has a non-primary number
        // that findByPhone can't match until getContact hydrates it.
        coEvery { apiClient.listContacts(any(), any(), any(), any()) } returns Result.success(
            com.mycorrhizal.crm.model.network.ContactsPage(
                contacts = listOf(summary(1, "Dana White").copy(primaryPhone = "555-0100")),
                nextCursor = "",
            ),
        )
        repository.listContacts()
        assertEquals(null, repository.findByPhone("555-0200"))
        assertEquals(listOf(1), repository.getContactIdsMissingPhoneIndex(limit = 10))

        coEvery { apiClient.getContact(1) } returns Result.success(
            ContactRecordResponse(
                id = 1,
                uid = "u1",
                card = Card(
                    name = Name(full = "Dana White"),
                    phones = listOf(
                        com.mycorrhizal.crm.model.network.Phone(number = "555-0100"),
                        com.mycorrhizal.crm.model.network.Phone(number = "555-0200"),
                    ),
                ),
            ),
        )

        repository.getContact(1)

        assertEquals(1, repository.findByPhone("555-0200")?.id)
        assertTrue(repository.getContactIdsMissingPhoneIndex(limit = 10).isEmpty())
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

    // --- Issue #963: call/SMS phone matching via PhoneKey -----------------

    @Test
    fun `findByPhone reconciles formatting and country-code differences`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Klara Beispiel",
                primaryPhone = "+49 (0) 151 12345678",
                phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(listOf("+49 (0) 151 12345678")),
            ),
        )

        assertEquals(1, repository.findByPhone("+4915112345678")?.id)
        assertEquals(1, repository.findByPhone("0151 12345678")?.id)
        assertEquals(1, repository.findByPhone("+49 (0) 151 12345678")?.id)
    }

    @Test
    fun `findByPhone matches a non-primary number`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Dana White",
                primaryPhone = "(800) 555-1234",
                phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(
                    listOf("(800) 555-1234", "555-0100"),
                ),
            ),
        )

        assertEquals(1, repository.findByPhone("5550100")?.id)
    }

    @Test
    fun `findByPhone never matches below seven digits`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Short Code",
                primaryPhone = "5551",
                phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(listOf("5551")),
            ),
        )

        assertNull(repository.findByPhone("5551"))
        assertNull(repository.findByPhone("12345"))
    }

    @Test
    fun `findByPhone ignores soft-deleted contacts`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(
                id = 1,
                fn = "Dana White",
                primaryPhone = "(800) 555-1234",
                phonesNormalized = com.mycorrhizal.crm.data.local.PhoneKey.flatten(listOf("(800) 555-1234")),
                deleted = true,
            ),
        )

        assertNull(repository.findByPhone("8005551234"))
    }

    @Test
    fun `findByPhone returns null when no contact matches`() = runTest {
        db.cachedContactDao().upsert(
            com.mycorrhizal.crm.data.local.CachedContact(id = 1, fn = "Bob Jones"),
        )

        assertNull(repository.findByPhone("8005551234"))
    }

    // --- M24: delete / archive / unarchive / export ---

    @Test
    fun `deleteContact removes the cached row on success`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White"))
        coEvery { apiClient.deleteContact(5) } returns Result.success(Unit)

        val result = repository.deleteContact(5)

        assertTrue(result.isSuccess)
        assertEquals(null, db.cachedContactDao().getById(5))
        io.mockk.coVerify(exactly = 1) { apiClient.deleteContact(5) }
    }

    @Test
    fun `deleteContact failure leaves the cached row in place`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White"))
        coEvery { apiClient.deleteContact(5) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.deleteContact(5)

        assertTrue(result.isFailure)
        assertTrue(db.cachedContactDao().getById(5) != null)
    }

    @Test
    fun `archiveContact flips the cached archived flag on success`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White", archived = false))
        coEvery { apiClient.archiveContact(5) } returns Result.success(Unit)

        val result = repository.archiveContact(5)

        assertTrue(result.isSuccess)
        assertEquals(true, db.cachedContactDao().getById(5)?.archived)
    }

    @Test
    fun `unarchiveContact flips the cached archived flag back`() = runTest {
        db.cachedContactDao().upsert(com.mycorrhizal.crm.data.local.CachedContact(id = 5, fn = "Dana White", archived = true))
        coEvery { apiClient.unarchiveContact(5) } returns Result.success(Unit)

        val result = repository.unarchiveContact(5)

        assertTrue(result.isSuccess)
        assertEquals(false, db.cachedContactDao().getById(5)?.archived)
    }

    @Test
    fun `exportContactVcf forwards the call and returns the raw bytes`() = runTest {
        val bytes = "BEGIN:VCARD\r\nEND:VCARD\r\n".toByteArray()
        coEvery { apiClient.exportContactVcf("u5", null) } returns Result.success(bytes)

        val result = repository.exportContactVcf("u5")

        assertTrue(result.isSuccess)
        assertEquals(bytes.contentToString(), result.getOrThrow().contentToString())
    }

    @Test
    fun `exportContactVcf propagates a backend failure`() = runTest {
        coEvery { apiClient.exportContactVcf("u5", 3) } returns Result.failure(ApiError.Client(400, "bad request"))

        val result = repository.exportContactVcf("u5", version = 3)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(400, (error as ApiError.Client).code)
    }

    // --- Issue #219: profile-photo upload ---

    @Test
    fun `uploadPhoto delegates to the api client with bytes and mime type`() = runTest {
        val bytes = ByteArray(64) { it.toByte() }
        coEvery { apiClient.uploadContactPhoto(5, bytes, "image/png") } returns Result.success(Unit)

        val result = repository.uploadPhoto(5, bytes, "image/png")

        assertTrue(result.isSuccess)
        io.mockk.coVerify(exactly = 1) { apiClient.uploadContactPhoto(5, bytes, "image/png") }
    }

    @Test
    fun `uploadPhoto propagates a backend failure`() = runTest {
        val bytes = ByteArray(64) { it.toByte() }
        coEvery { apiClient.uploadContactPhoto(5, bytes, "image/jpeg") } returns
            Result.failure(ApiError.Client(400, "File too large. Maximum size is 10MB"))

        val result = repository.uploadPhoto(5, bytes, "image/jpeg")

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(400, (error as ApiError.Client).code)
    }

    // --- 167: contact-address suggestions ---

    @Test
    fun `suggestContactAddresses maps the response to its suggestions`() = runTest {
        val suggestions = listOf(
            com.mycorrhizal.crm.model.network.ContactAddressSuggestion(
                contactVCardUid = "alice-uid",
                contactName = "Alice",
                sourceKind = "relationship",
                sourceId = "bob-uid",
                sourceName = "Bob",
                relationType = "spouse_of",
                addressKey = "key1",
            ),
        )
        coEvery { apiClient.suggestContactAddresses() } returns
            Result.success(com.mycorrhizal.crm.model.network.ContactAddressSuggestionsResponse(suggestions = suggestions, total = 1))

        val result = repository.suggestContactAddresses()

        assertTrue(result.isSuccess)
        assertEquals(suggestions, result.getOrThrow())
    }

    @Test
    fun `suggestContactAddresses propagates a backend failure`() = runTest {
        coEvery { apiClient.suggestContactAddresses() } returns Result.failure(ApiError.Client(500, "boom"))

        val result = repository.suggestContactAddresses()

        assertTrue(result.isFailure)
    }

    @Test
    fun `applyContactAddressSuggestion delegates to the api client`() = runTest {
        val input = com.mycorrhizal.crm.model.network.ApplyContactAddressSuggestionInput(
            contactVCardUid = "alice-uid",
            sourceKind = "relationship",
            sourceId = "bob-uid",
            addressKey = "key1",
        )
        coEvery { apiClient.applyContactAddressSuggestion(input) } returns Result.success(Unit)

        val result = repository.applyContactAddressSuggestion(input)

        assertTrue(result.isSuccess)
    }
}
