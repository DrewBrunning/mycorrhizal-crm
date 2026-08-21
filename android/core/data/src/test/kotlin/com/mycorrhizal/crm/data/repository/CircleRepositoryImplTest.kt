package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedCircle
import com.mycorrhizal.crm.data.local.CachedCircleMember
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleDetailResponse
import com.mycorrhizal.crm.model.network.CircleInput
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CircleMemberInput
import com.mycorrhizal.crm.model.network.CirclesPage
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

@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class CircleRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: CircleRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = CircleRepositoryImpl(apiClient, db.cachedCircleDao(), db.cachedCircleMemberDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `list mirrors the page into the cache`() = runTest {
        coEvery { apiClient.listCircles(cursor = null, limit = 20) } returns Result.success(
            CirclesPage(circles = listOf(Circle(id = "c1", name = "Friends"))),
        )

        val result = repository.list(cursor = null, limit = 20)

        assertTrue(result.isSuccess)
        assertEquals(listOf("c1"), result.getOrThrow().map { it.id })
        assertEquals("Friends", db.cachedCircleDao().getById("c1")?.name)
    }

    @Test
    fun `list failure propagates without touching the cache`() = runTest {
        coEvery { apiClient.listCircles(cursor = null, limit = 20) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.list(cursor = null, limit = 20)

        assertTrue(result.isFailure)
        assertNull(db.cachedCircleDao().getById("c1"))
    }

    @Test
    fun `circlesForContact replaces the member mirror wholesale`() = runTest {
        // Seed a stale membership for a different circle that should be wiped by the full resync.
        db.cachedCircleMemberDao().upsertAll(
            listOf(CachedCircleMember(id = 99, circleId = "stale", memberVCardUid = "u1")),
        )
        coEvery { apiClient.listCircles(includeMembers = true, limit = 100) } returns Result.success(
            CirclesPage(
                circles = listOf(Circle(id = "c1", name = "Friends"), Circle(id = "c2", name = "Family")),
                members = listOf(
                    CircleMember(id = 1, circleId = "c1", memberVCardUid = "u1"),
                    CircleMember(id = 2, circleId = "c2", memberVCardUid = "u2"),
                ),
            ),
        )

        val result = repository.circlesForContact("u1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("c1"), result.getOrThrow().map { it.id })
        assertTrue(db.cachedCircleMemberDao().getByCircleId("stale").isEmpty())
        assertEquals(1, db.cachedCircleMemberDao().getByCircleId("c1").size)
    }

    @Test
    fun `circlesForContact with a blank uid is a no-op`() = runTest {
        val result = repository.circlesForContact(" ")

        assertTrue(result.isSuccess)
        assertEquals(emptyList<Circle>(), result.getOrThrow())
    }

    @Test
    fun `getWithMembers replaces just that circle's members`() = runTest {
        coEvery { apiClient.getCircle("c1") } returns Result.success(
            CircleDetailResponse(
                circle = Circle(id = "c1", name = "Friends"),
                members = listOf(CircleMember(id = 1, circleId = "c1", memberVCardUid = "u1")),
            ),
        )

        val result = repository.getWithMembers("c1")

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().members.size)
        assertEquals("Friends", db.cachedCircleDao().getById("c1")?.name)
    }

    @Test
    fun `create mirrors the created circle into the cache`() = runTest {
        coEvery { apiClient.createCircle(CircleInput(name = "Friends")) } returns Result.success(
            Circle(id = "c1", name = "Friends"),
        )

        val result = repository.create("Friends")

        assertTrue(result.isSuccess)
        assertEquals("Friends", db.cachedCircleDao().getById("c1")?.name)
    }

    @Test
    fun `delete cascades to the cached circle and its members`() = runTest {
        db.cachedCircleDao().upsert(CachedCircle(id = "c1", name = "Friends"))
        db.cachedCircleMemberDao().upsertAll(
            listOf(CachedCircleMember(id = 1, circleId = "c1", memberVCardUid = "u1")),
        )
        coEvery { apiClient.deleteCircle("c1") } returns Result.success(Unit)

        val result = repository.delete("c1")

        assertTrue(result.isSuccess)
        assertNull(db.cachedCircleDao().getById("c1"))
        assertTrue(db.cachedCircleMemberDao().getByCircleId("c1").isEmpty())
    }

    @Test
    fun `delete failure leaves the cache untouched`() = runTest {
        db.cachedCircleDao().upsert(CachedCircle(id = "c1", name = "Friends"))
        coEvery { apiClient.deleteCircle("c1") } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.delete("c1")

        assertTrue(result.isFailure)
        assertEquals("Friends", db.cachedCircleDao().getById("c1")?.name)
    }

    @Test
    fun `addMember upserts the new membership`() = runTest {
        coEvery { apiClient.addCircleMember("c1", CircleMemberInput(memberVCardUid = "u1")) } returns Result.success(
            CircleMember(id = 1, circleId = "c1", memberVCardUid = "u1"),
        )

        val result = repository.addMember("c1", "u1")

        assertTrue(result.isSuccess)
        assertEquals(1, db.cachedCircleMemberDao().getByCircleId("c1").size)
    }

    @Test
    fun `removeMember deletes the cached membership`() = runTest {
        db.cachedCircleMemberDao().upsertAll(
            listOf(CachedCircleMember(id = 1, circleId = "c1", memberVCardUid = "u1")),
        )
        coEvery { apiClient.removeCircleMember("c1", "u1") } returns Result.success(Unit)

        val result = repository.removeMember("c1", "u1")

        assertTrue(result.isSuccess)
        assertTrue(db.cachedCircleMemberDao().getByCircleId("c1").isEmpty())
    }
}
