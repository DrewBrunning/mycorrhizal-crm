package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedLifeEvent
import com.mycorrhizal.crm.model.network.ConversationAgenda
import com.mycorrhizal.crm.model.network.ConversationAgendaInput
import com.mycorrhizal.crm.model.network.ConversationAgendaPage
import com.mycorrhizal.crm.model.network.Gift
import com.mycorrhizal.crm.model.network.GiftInput
import com.mycorrhizal.crm.model.network.GiftsPage
import com.mycorrhizal.crm.model.network.LifeEvent
import com.mycorrhizal.crm.model.network.LifeEventInput
import com.mycorrhizal.crm.model.network.LifeEventsPage
import com.mycorrhizal.crm.model.network.Preference
import com.mycorrhizal.crm.model.network.PreferenceInput
import com.mycorrhizal.crm.model.network.PreferencesPage
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.mockk
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

/**
 * All four repositories in TimelineEntityRepositories.kt are structurally
 * identical (listForContact/create/update/delete, full-resync-on-list,
 * filterNot { it.deleted == true }). The full pattern is exercised once
 * against LifeEventRepositoryImpl; the other three get one deleted-row-
 * filtering test and one create/update/delete test each rather than
 * repeating every case four times.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class TimelineEntityRepositoriesTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
    }

    @After
    fun teardown() {
        db.close()
    }

    // -- LifeEventRepositoryImpl: full pattern --

    @Test
    fun `lifeEvents listForContact resyncs the cache and drops deleted rows`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        db.cachedLifeEventDao().upsertAll(
            listOf(CachedLifeEvent(id = "stale", entityId = "c1")),
        )
        coEvery { apiClient.listLifeEvents(entityId = "c1") } returns Result.success(
            LifeEventsPage(
                lifeEvents = listOf(
                    LifeEvent(id = "le1", entityId = "c1", type = "graduation"),
                    LifeEvent(id = "le2", entityId = "c1", type = "wedding", deleted = true),
                ),
            ),
        )

        val result = repository.listForContact("c1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("le1", "le2"), result.getOrThrow().map { it.id }.sorted())
        val cached = db.cachedLifeEventDao().getForContact("c1")
        assertEquals(listOf("le1"), cached.map { it.id })
    }

    @Test
    fun `lifeEvents listForContact failure propagates without touching the cache`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        coEvery { apiClient.listLifeEvents(entityId = "c1") } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.listForContact("c1")

        assertTrue(result.isFailure)
        assertTrue(db.cachedLifeEventDao().getForContact("c1").isEmpty())
    }

    @Test
    fun `lifeEvents create mirrors into the cache`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        val input = LifeEventInput(entityId = "c1", type = "graduation")
        coEvery { apiClient.createLifeEvent(input) } returns Result.success(
            LifeEvent(id = "le1", entityId = "c1", type = "graduation"),
        )

        val result = repository.create(input)

        assertTrue(result.isSuccess)
        assertEquals("graduation", db.cachedLifeEventDao().getForContact("c1").single().type)
    }

    @Test
    fun `lifeEvents update mirrors into the cache`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        val input = LifeEventInput(entityId = "c1", type = "graduation", description = "Class of 2026")
        coEvery { apiClient.updateLifeEvent("le1", input) } returns Result.success(
            LifeEvent(id = "le1", entityId = "c1", type = "graduation", description = "Class of 2026"),
        )

        val result = repository.update("le1", input)

        assertTrue(result.isSuccess)
        assertEquals("Class of 2026", db.cachedLifeEventDao().getForContact("c1").single().description)
    }

    @Test
    fun `lifeEvents delete removes the cached row`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        db.cachedLifeEventDao().upsert(CachedLifeEvent(id = "le1", entityId = "c1"))
        coEvery { apiClient.deleteLifeEvent("le1") } returns Result.success(Unit)

        val result = repository.delete("le1")

        assertTrue(result.isSuccess)
        assertTrue(db.cachedLifeEventDao().getForContact("c1").isEmpty())
    }

    @Test
    fun `lifeEvents delete failure leaves the cache untouched`() = runTest {
        val repository = LifeEventRepositoryImpl(apiClient, db.cachedLifeEventDao())
        db.cachedLifeEventDao().upsert(CachedLifeEvent(id = "le1", entityId = "c1"))
        coEvery { apiClient.deleteLifeEvent("le1") } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.delete("le1")

        assertTrue(result.isFailure)
        assertEquals(1, db.cachedLifeEventDao().getForContact("c1").size)
    }

    // -- GiftRepositoryImpl --

    @Test
    fun `gifts listForContact resyncs the cache and drops deleted rows`() = runTest {
        val repository = GiftRepositoryImpl(apiClient, db.cachedGiftDao())
        coEvery { apiClient.listGifts(entityId = "c1") } returns Result.success(
            GiftsPage(
                gifts = listOf(
                    Gift(id = "g1", entityId = "c1", occasion = "birthday"),
                    Gift(id = "g2", entityId = "c1", occasion = "holiday", deleted = true),
                ),
            ),
        )

        val result = repository.listForContact("c1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("g1"), db.cachedGiftDao().getForContact("c1").map { it.id })
    }

    @Test
    fun `gifts create update delete round-trip through the cache`() = runTest {
        val repository = GiftRepositoryImpl(apiClient, db.cachedGiftDao())
        val createInput = GiftInput(entityId = "c1", occasion = "birthday")
        coEvery { apiClient.createGift(createInput) } returns Result.success(
            Gift(id = "g1", entityId = "c1", occasion = "birthday"),
        )
        val result = repository.create(createInput)
        assertTrue(result.isSuccess)
        assertEquals("birthday", db.cachedGiftDao().getForContact("c1").single().occasion)

        val updateInput = GiftInput(entityId = "c1", occasion = "birthday", description = "Bought")
        coEvery { apiClient.updateGift("g1", updateInput) } returns Result.success(
            Gift(id = "g1", entityId = "c1", occasion = "birthday", description = "Bought"),
        )
        assertTrue(repository.update("g1", updateInput).isSuccess)
        assertEquals("Bought", db.cachedGiftDao().getForContact("c1").single().description)

        coEvery { apiClient.deleteGift("g1") } returns Result.success(Unit)
        assertTrue(repository.delete("g1").isSuccess)
        assertTrue(db.cachedGiftDao().getForContact("c1").isEmpty())
    }

    // -- PreferenceRepositoryImpl --

    @Test
    fun `preferences listForContact resyncs the cache and drops deleted rows`() = runTest {
        val repository = PreferenceRepositoryImpl(apiClient, db.cachedPreferenceDao())
        coEvery { apiClient.listPreferences(entityId = "c1") } returns Result.success(
            PreferencesPage(
                preferences = listOf(
                    Preference(id = "p1", entityId = "c1", category = "food"),
                    Preference(id = "p2", entityId = "c1", category = "music", deleted = true),
                ),
            ),
        )

        val result = repository.listForContact("c1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("p1"), db.cachedPreferenceDao().getForContact("c1").map { it.id })
    }

    @Test
    fun `preferences create update delete round-trip through the cache`() = runTest {
        val repository = PreferenceRepositoryImpl(apiClient, db.cachedPreferenceDao())
        val createInput = PreferenceInput(entityId = "c1", category = "food", value = "Italian")
        coEvery { apiClient.createPreference(createInput) } returns Result.success(
            Preference(id = "p1", entityId = "c1", category = "food", value = "Italian"),
        )
        assertTrue(repository.create(createInput).isSuccess)
        assertEquals("Italian", db.cachedPreferenceDao().getForContact("c1").single().value)

        val updateInput = PreferenceInput(entityId = "c1", category = "food", value = "Thai")
        coEvery { apiClient.updatePreference("p1", updateInput) } returns Result.success(
            Preference(id = "p1", entityId = "c1", category = "food", value = "Thai"),
        )
        assertTrue(repository.update("p1", updateInput).isSuccess)
        assertEquals("Thai", db.cachedPreferenceDao().getForContact("c1").single().value)

        coEvery { apiClient.deletePreference("p1") } returns Result.success(Unit)
        assertTrue(repository.delete("p1").isSuccess)
        assertTrue(db.cachedPreferenceDao().getForContact("c1").isEmpty())
    }

    // -- ConversationAgendaRepositoryImpl (also has a `discuss` action the others don't) --

    @Test
    fun `conversationAgenda listForContact resyncs the cache and drops deleted rows`() = runTest {
        val repository = ConversationAgendaRepositoryImpl(apiClient, db.cachedConversationAgendaDao())
        coEvery { apiClient.listConversationAgenda(entityId = "c1") } returns Result.success(
            ConversationAgendaPage(
                conversationAgenda = listOf(
                    ConversationAgenda(id = "a1", entityId = "c1", content = "Ask about trip"),
                    ConversationAgenda(id = "a2", entityId = "c1", content = "Old topic", deleted = true),
                ),
            ),
        )

        val result = repository.listForContact("c1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("a1"), db.cachedConversationAgendaDao().getForContact("c1").map { it.id })
    }

    @Test
    fun `conversationAgenda create update delete round-trip through the cache`() = runTest {
        val repository = ConversationAgendaRepositoryImpl(apiClient, db.cachedConversationAgendaDao())
        val createInput = ConversationAgendaInput(entityId = "c1", content = "Ask about trip")
        coEvery { apiClient.createConversationAgenda(createInput) } returns Result.success(
            ConversationAgenda(id = "a1", entityId = "c1", content = "Ask about trip"),
        )
        assertTrue(repository.create(createInput).isSuccess)
        assertEquals("Ask about trip", db.cachedConversationAgendaDao().getForContact("c1").single().content)

        val updateInput = ConversationAgendaInput(entityId = "c1", content = "Ask about the trip")
        coEvery { apiClient.updateConversationAgenda("a1", updateInput) } returns Result.success(
            ConversationAgenda(id = "a1", entityId = "c1", content = "Ask about the trip"),
        )
        assertTrue(repository.update("a1", updateInput).isSuccess)
        assertEquals("Ask about the trip", db.cachedConversationAgendaDao().getForContact("c1").single().content)

        coEvery { apiClient.deleteConversationAgenda("a1") } returns Result.success(Unit)
        assertTrue(repository.delete("a1").isSuccess)
        assertTrue(db.cachedConversationAgendaDao().getForContact("c1").isEmpty())
    }

    @Test
    fun `conversationAgenda discuss mirrors the linked activity into the cache`() = runTest {
        val repository = ConversationAgendaRepositoryImpl(apiClient, db.cachedConversationAgendaDao())
        coEvery { apiClient.discussConversationAgenda("a1", 42) } returns Result.success(
            ConversationAgenda(id = "a1", entityId = "c1", content = "Ask about trip", discussedAt = "2026-08-21"),
        )

        val result = repository.discuss("a1", 42)

        assertTrue(result.isSuccess)
        assertEquals("2026-08-21", db.cachedConversationAgendaDao().getForContact("c1").single().discussedAt)
    }

    @Test
    fun `conversationAgenda discuss failure propagates`() = runTest {
        val repository = ConversationAgendaRepositoryImpl(apiClient, db.cachedConversationAgendaDao())
        coEvery { apiClient.discussConversationAgenda("a1", null) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.discuss("a1", null)

        assertTrue(result.isFailure)
        assertNull(db.cachedConversationAgendaDao().getForContact("c1").firstOrNull())
    }
}
