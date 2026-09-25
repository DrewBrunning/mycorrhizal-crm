package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.InviteeSuggestion
import com.mycorrhizal.crm.model.network.InviteeSuggestionsResponse
import com.mycorrhizal.crm.model.network.OccasionEvent
import com.mycorrhizal.crm.model.network.OccasionEventAttendee
import com.mycorrhizal.crm.model.network.OccasionEventAttendeeInput
import com.mycorrhizal.crm.model.network.OccasionEventDetail
import com.mycorrhizal.crm.model.network.OccasionEventInput
import com.mycorrhizal.crm.model.network.OccasionEventsResponse
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
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
class OccasionEventRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: OccasionEventRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = OccasionEventRepositoryImpl(apiClient, db.cachedOccasionEventDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun event(id: String = "e1", title: String = "Summer BBQ", deleted: Boolean = false) =
        OccasionEvent(id = id, title = title, startsAt = "2026-07-04T15:00:00Z", deleted = deleted)

    @Test
    fun `list replaces the cache and drops tombstones`() = runTest {
        coEvery { apiClient.listOccasionEvents() } returns Result.success(
            OccasionEventsResponse(occasionEvents = listOf(event("e1"), event("e2"), event("e3", deleted = true))),
        )

        val result = repository.list()

        assertTrue(result.isSuccess)
        // The list response is returned verbatim; only the offline cache drops tombstones.
        assertEquals(3, result.getOrNull()?.size)
        val cached = db.cachedOccasionEventDao().getAll()
        assertEquals(2, cached.size)
        assertTrue(cached.none { it.deleted })
    }

    @Test
    fun `list on failure leaves the cache untouched`() = runTest {
        db.cachedOccasionEventDao().upsert(com.mycorrhizal.crm.data.local.CachedOccasionEvent(id = "e1", title = "old"))
        coEvery { apiClient.listOccasionEvents() } returns Result.failure(RuntimeException("offline"))

        val result = repository.list()

        assertTrue(result.isFailure)
        assertEquals(1, db.cachedOccasionEventDao().getAll().size)
    }

    @Test
    fun `get passes through the detail`() = runTest {
        val detail = OccasionEventDetail(occasionEvent = event(), attendees = emptyList())
        coEvery { apiClient.getOccasionEvent("e1") } returns Result.success(detail)

        val result = repository.get("e1")

        assertEquals(detail, result.getOrNull())
    }

    @Test
    fun `create and update mirror the returned event`() = runTest {
        val input = OccasionEventInput(title = "Party", startsAt = "2026-07-04T15:00:00Z")
        coEvery { apiClient.createOccasionEvent(input) } returns Result.success(event("e1", "Party"))

        repository.create(input)
        assertEquals(1, db.cachedOccasionEventDao().getAll().size)

        coEvery { apiClient.updateOccasionEvent("e1", input) } returns Result.success(event("e1", "Renamed"))
        repository.update("e1", input)
        assertEquals("Renamed", db.cachedOccasionEventDao().getAll().first().title)
    }

    @Test
    fun `delete removes the cached row`() = runTest {
        db.cachedOccasionEventDao().upsert(com.mycorrhizal.crm.data.local.CachedOccasionEvent(id = "e1", title = "x"))
        coEvery { apiClient.deleteOccasionEvent("e1") } returns Result.success(Unit)

        repository.delete("e1")

        assertEquals(0, db.cachedOccasionEventDao().getAll().size)
    }

    @Test
    fun `failed delete keeps the cached row`() = runTest {
        db.cachedOccasionEventDao().upsert(com.mycorrhizal.crm.data.local.CachedOccasionEvent(id = "e1", title = "x"))
        coEvery { apiClient.deleteOccasionEvent("e1") } returns Result.failure(RuntimeException("boom"))

        repository.delete("e1")

        assertEquals(1, db.cachedOccasionEventDao().getAll().size)
    }

    @Test
    fun `attendee operations pass straight through`() = runTest {
        val attendee = OccasionEventAttendee(id = "a1", eventId = "e1", entityId = "uid-1")
        coEvery { apiClient.addOccasionEventAttendee("e1", OccasionEventAttendeeInput("uid-1")) } returns Result.success(attendee)
        coEvery { apiClient.updateOccasionEventAttendee("e1", "uid-1", "accepted") } returns Result.success(attendee)
        coEvery { apiClient.removeOccasionEventAttendee("e1", "uid-1") } returns Result.success(Unit)

        assertEquals(attendee, repository.addAttendee("e1", OccasionEventAttendeeInput("uid-1")).getOrNull())
        assertEquals(attendee, repository.updateAttendee("e1", "uid-1", "accepted").getOrNull())
        assertTrue(repository.removeAttendee("e1", "uid-1").isSuccess)
        coVerify { apiClient.removeOccasionEventAttendee("e1", "uid-1") }
    }

    @Test
    fun `suggestInvitees unwraps the suggestions`() = runTest {
        val suggestions = listOf(InviteeSuggestion(contactId = 1, contactName = "Alice", entityId = "uid-1"))
        coEvery { apiClient.suggestInvitees(listOf("c1"), "e1") } returns Result.success(
            InviteeSuggestionsResponse(suggestions = suggestions),
        )

        assertEquals(suggestions, repository.suggestInvitees(listOf("c1"), "e1").getOrNull())
    }
}
