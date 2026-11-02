package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedActivity
import com.mycorrhizal.crm.model.network.ActivitiesPage
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityImage
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.model.network.AddContactToActivitiesResponse
import com.mycorrhizal.crm.model.network.AddToActivityResult
import com.mycorrhizal.crm.model.network.ContactActivitiesResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
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
class ActivityRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: ActivityRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = ActivityRepositoryImpl(apiClient, db.cachedActivityDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `listForContact caches activities and returns them`() = runTest {
        coEvery {
            apiClient.listContactActivities(
                5,
                any(),
                any(),
                any(),
                any(),
                any()
            )
        } returns Result.success(
            ContactActivitiesResponse(
                activities = listOf(
                    Activity(id = 1, title = "Coffee with Dana", type = "visit"),
                    Activity(id = 2, title = "Phone call", type = "call")
                )
            )
        )

        val result = repository.listForContact(5)

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().activities.size)

        val cached = db.cachedActivityDao().getAll()
        assertEquals(2, cached.size)
        assertEquals("Coffee with Dana", cached[0].title)
    }

    @Test
    fun `listForContact propagates a 404`() = runTest {
        coEvery {
            apiClient.listContactActivities(
                999,
                any(),
                any(),
                any(),
                any(),
                any()
            )
        } returns Result.failure(
            ApiError.Client(404, "Contact not found")
        )

        val result = repository.listForContact(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(404, (error as ApiError.Client).code)
    }

    // M19: the page carries the T17 next_cursor for "load more".
    @Test
    fun `listForContact carries the next cursor`() = runTest {
        coEvery {
            apiClient.listContactActivities(
                5,
                any(),
                any(),
                any(),
                any(),
                any()
            )
        } returns Result.success(
            ContactActivitiesResponse(
                activities = listOf(Activity(id = 1, title = "Coffee")),
                nextCursor = "cursor-2"
            )
        )

        val result = repository.listForContact(5)

        assertTrue(result.isSuccess)
        assertEquals("cursor-2", result.getOrThrow().nextCursor)
    }

    // M19: delete drops the local cache row after a successful server soft delete.
    @Test
    fun `delete removes the cached row on success`() = runTest {
        db.cachedActivityDao().upsertAll(
            listOf(
                CachedActivity(
                    id = 1,
                    title = "Coffee",
                    description = null,
                    location = null,
                    date = null,
                    type = null,
                    deleted = false
                )
            )
        )
        coEvery { apiClient.deleteActivity(1) } returns Result.success(Unit)

        val result = repository.delete(1, null)

        assertTrue(result.isSuccess)
        assertTrue(db.cachedActivityDao().getAll().isEmpty())
    }

    // M19: a failed delete must not clear the local cache (the item stays).
    @Test
    fun `delete failure leaves the cached row`() = runTest {
        db.cachedActivityDao().upsertAll(
            listOf(
                CachedActivity(
                    id = 1,
                    title = "Coffee",
                    description = null,
                    location = null,
                    date = null,
                    type = null,
                    deleted = false
                )
            )
        )
        coEvery { apiClient.deleteActivity(1) } returns Result.failure(ApiError.Client(500, "boom"))

        val result = repository.delete(1, null)

        assertTrue(result.isFailure)
        assertEquals(1, db.cachedActivityDao().getAll().size)
    }

    // M9: the Activities drawer inbox — GET /activities?include=contacts, all contacts' activities.
    @Test
    fun `listAll requests included contacts and caches the page`() = runTest {
        coEvery {
            apiClient.listActivities(
                cursor = null,
                limit = null,
                includeContacts = true
            )
        } returns Result.success(
            ActivitiesPage(
                activitiesRaw = listOf(
                    Activity(id = 1, title = "Coffee with Dana", type = "visit")
                ),
                nextCursor = "cursor-2"
            )
        )

        val result = repository.listAll()

        assertTrue(result.isSuccess)
        assertEquals("cursor-2", result.getOrThrow().nextCursor)
        assertEquals(1, db.cachedActivityDao().getAll().size)
    }

    // Trap-#8 fix: GetActivities marshals a nil Go slice as JSON `null`, not `[]`, when there are
    // zero activities. ActivitiesPage's raw field must be nullable so this doesn't crash Moshi —
    // this is the repository-level proof (ApiClientTest covers the JSON parse itself).
    @Test
    fun `listAll tolerates a null activities list`() = runTest {
        coEvery {
            apiClient.listActivities(
                cursor = null,
                limit = null,
                includeContacts = true
            )
        } returns Result.success(
            ActivitiesPage(activitiesRaw = null)
        )

        val result = repository.listAll()

        assertTrue(result.isSuccess)
        assertTrue(db.cachedActivityDao().getAll().isEmpty())
    }

    @Test
    fun `get returns the single activity`() = runTest {
        coEvery { apiClient.getActivity(7) } returns Result.success(
            Activity(id = 7, title = "Lunch", type = "meal")
        )

        val result = repository.get(7)

        assertTrue(result.isSuccess)
        assertEquals("Lunch", result.getOrThrow().title)
    }

    @Test
    fun `create caches the created activity`() = runTest {
        val created = Activity(id = 7, title = "Lunch", type = "meal")
        coEvery { apiClient.createActivity(any()) } returns Result.success(created)

        val result = repository.create(ActivityInput(title = "Lunch"))

        assertTrue(result.isSuccess)
        assertEquals(7, result.getOrThrow().id)
        assertEquals("Lunch", db.cachedActivityDao().getById(7)?.title)
    }

    @Test
    fun `update caches the updated activity`() = runTest {
        val updated = Activity(id = 7, title = "Lunch and coffee", type = "meal")
        coEvery { apiClient.updateActivity(7, any()) } returns Result.success(updated)

        val result = repository.update(7, ActivityInput(title = "Lunch and coffee"), null)

        assertTrue(result.isSuccess)
        assertEquals("Lunch and coffee", db.cachedActivityDao().getById(7)?.title)
    }

    @Test
    fun `create propagates a validation failure`() = runTest {
        coEvery { apiClient.createActivity(any()) } returns Result.failure(
            ApiError.Client(400, "Title is required")
        )

        val result = repository.create(ActivityInput(title = ""))

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(400, (error as ApiError.Client).code)
    }

    // Issue #1665 / ADR 0008: the revision is forwarded as If-Match.
    @Test
    fun `update and delete forward the revision`() = runTest {
        coEvery {
            apiClient.updateActivity(
                7,
                any(),
                5
            )
        } returns Result.success(Activity(id = 7, title = "x"))
        coEvery { apiClient.deleteActivity(7, 5) } returns Result.success(Unit)

        assertTrue(repository.update(7, ActivityInput(title = "x"), 5).isSuccess)
        assertTrue(repository.delete(7, 5).isSuccess)

        io.mockk.coVerify(exactly = 1) { apiClient.updateActivity(7, any(), 5) }
        io.mockk.coVerify(exactly = 1) { apiClient.deleteActivity(7, 5) }
    }

    private fun cached(id: Int) = CachedActivity(
        id = id,
        title = "A$id",
        description = null,
        location = null,
        date = null,
        type = null,
        deleted = false
    )

    // --- Issue #2136 ------------------------------------------------------------

    @Test
    fun `addContactToActivities drops cached rows only for activities that were added`() = runTest {
        db.cachedActivityDao().upsertAll(
            listOf(1, 2, 3).map { cached(it) }
        )
        coEvery { apiClient.addContactToActivities(5, listOf(1, 2, 3)) } returns Result.success(
            AddContactToActivitiesResponse(
                contactId = 5,
                results = listOf(
                    AddToActivityResult(1, "added"),
                    AddToActivityResult(2, "already_member"),
                    AddToActivityResult(3, "not_found")
                )
            )
        )

        val result = repository.addContactToActivities(5, listOf(1, 2, 3))

        assertTrue(result.isSuccess)
        assertEquals(listOf(2, 3), db.cachedActivityDao().getAll().map { it.id }.sorted())
    }

    @Test
    fun `addContactToActivities leaves the cache alone when the request fails`() = runTest {
        db.cachedActivityDao().upsertAll(listOf(cached(1)))
        coEvery { apiClient.addContactToActivities(5, listOf(1)) } returns
            Result.failure(ApiError.Network(java.io.IOException("offline")))

        val result = repository.addContactToActivities(5, listOf(1))

        assertTrue(result.isFailure)
        assertEquals(1, db.cachedActivityDao().getAll().size)
    }

    @Test
    fun `image methods delegate to the client`() = runTest {
        val image = ActivityImage(
            id = 9,
            uuid = "u",
            activityId = 1,
            mimeType = "image/jpeg",
            width = 10,
            height = 10,
            sizeBytes = 100,
            sensitivity = "normal",
            createdAt = "2026-01-01T00:00:00Z",
            fileUrl = "/f",
            thumbnailUrl = "/t"
        )
        coEvery { apiClient.listActivityImages(1) } returns Result.success(
            com.mycorrhizal.crm.model.network.ActivityImagesResponse(
                imagesRaw = listOf(image),
                total = 1
            )
        )
        coEvery { apiClient.uploadActivityImage(1, "a.jpg", "image/jpeg", byteArrayOf(1)) } returns
            Result.success(image)
        coEvery { apiClient.deleteActivityImage(1, 9) } returns Result.success(Unit)

        assertEquals(listOf(image), repository.listImages(1).getOrThrow())
        assertEquals(
            image,
            repository.uploadImage(1, "a.jpg", "image/jpeg", byteArrayOf(1)).getOrThrow()
        )
        assertTrue(repository.deleteImage(1, 9).isSuccess)
    }
}
