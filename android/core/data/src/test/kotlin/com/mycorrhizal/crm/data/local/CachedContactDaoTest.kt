package com.mycorrhizal.crm.data.local

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.model.network.CRMEnvelope
import com.mycorrhizal.crm.model.network.Card
import com.mycorrhizal.crm.model.network.Email
import kotlinx.coroutines.runBlocking
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CachedContactDaoTest {

    private lateinit var db: AppDatabase
    private lateinit var dao: CachedContactDao

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        dao = db.cachedContactDao()
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun testContact(id: Int, fn: String, firstname: String = fn): CachedContact =
        CachedContact(id = id, fn = fn, firstname = firstname)

    @Test
    fun `upsert replaces existing row`() = runBlocking {
        dao.upsert(testContact(1, "Alice"))
        dao.upsert(testContact(1, "Alicia"))

        val result = dao.getById(1)
        assertEquals("Alicia", result?.fn)
    }

    @Test
    fun `getById returns null for missing id`() = runBlocking {
        assertNull(dao.getById(999))
    }

    @Test
    fun `getAll excludes soft-deleted rows`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "Alice"),
                testContact(2, "Bob").copy(deleted = true),
            ),
        )

        val result = dao.getAll()
        assertEquals(1, result.size)
        assertEquals(1, result[0].id)
    }

    @Test
    fun `search matches name and email`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "Alice Smith").copy(primaryEmail = "alice@example.com"),
                testContact(2, "Bob Jones"),
            ),
        )

        val byName = dao.search("smith")
        assertEquals(1, byName.size)
        assertEquals(1, byName[0].id)

        val byEmail = dao.search("alice@example.com")
        assertEquals(1, byEmail.size)
    }

    @Test
    fun `searchFts matches via the FTS mirror with prefix semantics`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "David Smith").copy(primaryEmail = "david@example.com"),
                testContact(2, "Bob Jones"),
            ),
        )

        // FTS MATCH is case-insensitive and prefix-expanded by the '*'.
        val byPrefix = dao.searchFts("dav")
        assertEquals(1, byPrefix.size)
        assertEquals("David Smith", byPrefix[0].fn)

        val byEmail = dao.searchFts("david@example.com")
        assertEquals(1, byEmail.size)
        assertEquals(1, byEmail[0].id)
    }

    @Test
    fun `searchFts excludes soft-deleted rows`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "David Smith"),
                testContact(2, "Dan").copy(deleted = true),
            ),
        )

        val result = dao.searchFts("da")
        assertEquals(1, result.size)
        assertEquals(1, result[0].id)
    }

    @Test
    fun `searchFts stays in sync after a replace`() = runBlocking {
        dao.upsert(testContact(1, "Alice Smith"))
        dao.upsert(testContact(1, "Alice Jones"))

        // The FTS mirror must reflect the updated row, not the old one.
        val result = dao.searchFts("smith")
        assertEquals(0, result.size)
        val jones = dao.searchFts("jones")
        assertEquals(1, jones.size)
        assertEquals("Alice Jones", jones[0].fn)
    }

    @Test
    fun `deleteByIds removes the listed rows`() = runBlocking {
        dao.upsertAll(listOf(testContact(1, "Alice"), testContact(2, "Bob")))
        dao.deleteByIds(listOf(1))

        assertNull(dao.getById(1))
        assertEquals(2, dao.getById(2)?.id)
    }

    @Test
    fun `deleteAll empties the table`() = runBlocking {
        dao.upsertAll(listOf(testContact(1, "Alice"), testContact(2, "Bob")))
        dao.deleteAll()

        assertEquals(0, dao.getAll().size)
    }

    @Test
    fun `card and crm survive the round trip via converters`() = runBlocking {
        val card = Card(name = com.mycorrhizal.crm.model.network.Name(full = "Alice"), emails = listOf(Email(address = "a@x.com")))
        val crm = CRMEnvelope(circles = listOf("friends"))
        val contact = testContact(1, "Alice").copy(card = card, crm = crm, circles = listOf("friends"))
        dao.upsert(contact)

        val result = dao.getById(1)
        assertEquals("Alice", result?.card?.name?.full)
        assertEquals(listOf("friends"), result?.crm?.circles)
        assertEquals(listOf("friends"), result?.circles)
    }
}
