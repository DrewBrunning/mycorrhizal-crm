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
    fun `searchFts is accent- and case-insensitive for Latin under unicode61`() = runBlocking {
        // I18N-02 (issue #485): the mirror tokenizer is unicode61 (schema v18),
        // so offline search agrees with the server's FTS5 — "garcia" finds
        // "García", matching the server's documented Latin accent/case fold.
        dao.upsertAll(
            listOf(
                testContact(1, "García Ruiz"),
                testContact(2, "José"),
                testContact(3, "Müller"),
            ),
        )

        assertEquals(1, dao.searchFts("garcia").size)
        assertEquals(1, dao.searchFts("GARCÍA").size)
        assertEquals(1, dao.searchFts("jose").size)
        assertEquals(1, dao.searchFts("MÜLLER").size)
        assertEquals("García Ruiz", dao.searchFts("garcia")[0].fn)
    }

    @Test
    fun `searchFts inherits the documented non-folds of unicode61`() = runBlocking {
        // The server's deliberate non-folds (docs/development/unicode-search.md)
        // must hold offline too: German ß is not ss, Turkish dotless ı is not i.
        dao.upsertAll(
            listOf(
                testContact(1, "Straße"),
                testContact(2, "Istanbul"),
                testContact(3, "İstanbul"),
            ),
        )

        assertEquals(1, dao.searchFts("straße").size)
        assertEquals(0, dao.searchFts("strasse").size)
        assertEquals(2, dao.searchFts("istanbul").size) // ASCII I row + dotted-İ row both fold to i
        assertEquals(0, dao.searchFts("\u0131stanbul").size) // dotless ı stays distinct
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
    fun `searchFtsMatch finds a punctuated phone number by its bare digits`() = runBlocking {
        // Regression for T76: FTS4's default tokenizer splits "(800) 555-1234" on the
        // punctuation into three tokens ("800", "555", "1234"); a query of "8005551234" must
        // still find it via the normalized phonesNormalized column, not the raw one.
        dao.upsertAll(
            listOf(
                testContact(1, "Dana White").copy(
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = PhoneKey.flatten(listOf("(800) 555-1234")),
                ),
                testContact(2, "Bob Jones"),
            ),
        )

        val result = dao.searchFtsMatch("phonesNormalized:8005551234*")

        assertEquals(1, result.size)
        assertEquals("Dana White", result[0].fn)
    }

    @Test
    fun `searchFtsMatch finds a non-primary phone number`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "Dana White").copy(
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = PhoneKey.flatten(listOf("(800) 555-1234", "555-0100")),
                ),
            ),
        )

        val result = dao.searchFtsMatch("phonesNormalized:5550100*")

        assertEquals(1, result.size)
        assertEquals("Dana White", result[0].fn)
    }

    @Test
    fun `findByPhoneKey matches a full-digit token`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "Dana White").copy(
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = PhoneKey.flatten(listOf("(800) 555-1234")),
                ),
                testContact(2, "Bob Jones"),
            ),
        )

        val result = dao.findByPhoneKey(PhoneKey.key("(800) 555-1234"))

        assertEquals("Dana White", result?.fn)
    }

    @Test
    fun `findByPhoneKey matches a contact by its non-primary number`() = runBlocking {
        // Issue #963: matching must consider every stored number, not just the
        // primary one — a secondary cell number was previously unmatchable.
        dao.upsertAll(
            listOf(
                testContact(1, "Dana White").copy(
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = PhoneKey.flatten(listOf("(800) 555-1234", "555-0100")),
                ),
            ),
        )

        assertEquals("Dana White", dao.findByPhoneKey(PhoneKey.key("555-0100"))?.fn)
    }

    @Test
    fun `findByPhoneKey reconciles an international vs local form via the key token`() = runBlocking {
        // Issue #963's case: a CardDAV contact stored as "+49 (0) 151 12345678"
        // must match a call-log sender shown as "+4915112345678" or "0151 12345678".
        dao.upsertAll(
            listOf(
                testContact(1, "Klara Beispiel").copy(
                    primaryPhone = "+49 (0) 151 12345678",
                    phonesNormalized = PhoneKey.flatten(listOf("+49 (0) 151 12345678")),
                ),
            ),
        )

        assertEquals("Klara Beispiel", dao.findByPhoneKey(PhoneKey.key("+4915112345678"))?.fn)
        assertEquals("Klara Beispiel", dao.findByPhoneKey(PhoneKey.key("0151 12345678"))?.fn)
    }

    @Test
    fun `findByPhoneKey is token-exact and never matches a longer number's suffix`() = runBlocking {
        // Boundary safety: a key that is a suffix of a longer stored number must
        // not match it (5551234 must not match 15551234).
        dao.upsertAll(
            listOf(
                testContact(1, "Short").copy(
                    primaryPhone = "5551234",
                    phonesNormalized = PhoneKey.flatten(listOf("5551234")),
                ),
                testContact(2, "Long").copy(
                    primaryPhone = "15551234",
                    phonesNormalized = PhoneKey.flatten(listOf("15551234")),
                ),
            ),
        )

        val result = dao.findByPhoneKey("5551234")
        assertEquals(1, result?.id)
    }

    @Test
    fun `findByPhoneKey returns null when nothing matches`() = runBlocking {
        dao.upsertAll(listOf(testContact(1, "Dana White").copy(primaryPhone = "555-0100")))

        assertNull(dao.findByPhoneKey("8005551234"))
    }

    @Test
    fun `findByPhoneKey excludes soft-deleted contacts`() = runBlocking {
        dao.upsertAll(
            listOf(
                testContact(1, "Dana White").copy(
                    primaryPhone = "(800) 555-1234",
                    phonesNormalized = PhoneKey.flatten(listOf("(800) 555-1234")),
                    deleted = true,
                ),
            ),
        )

        assertNull(dao.findByPhoneKey("8005551234"))
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
    fun `getIdsMissingPhoneIndex returns only rows never detail-fetched`() = runBlocking {
        // Issue #1122: a row with a card (a prior full detail fetch already
        // populated the full multi-phone index) is not "missing" even though
        // some other row hasn't been fetched yet.
        val card = Card(name = com.mycorrhizal.crm.model.network.Name(full = "Dana"))
        dao.upsertAll(
            listOf(
                testContact(1, "Alice").copy(primaryPhone = "555-0100"),
                testContact(2, "Bob").copy(primaryPhone = "555-0200", card = card),
                testContact(3, "No Phone"),
                testContact(4, "Deleted").copy(primaryPhone = "555-0400", deleted = true),
            ),
        )

        val result = dao.getIdsMissingPhoneIndex(limit = 10)

        assertEquals(listOf(1), result)
    }

    @Test
    fun `getIdsMissingPhoneIndex is bounded by limit and ordered by id`() = runBlocking {
        dao.upsertAll(
            (1..5).map { id -> testContact(id, "Contact $id").copy(primaryPhone = "555-000$id") },
        )

        val result = dao.getIdsMissingPhoneIndex(limit = 3)

        assertEquals(listOf(1, 2, 3), result)
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
