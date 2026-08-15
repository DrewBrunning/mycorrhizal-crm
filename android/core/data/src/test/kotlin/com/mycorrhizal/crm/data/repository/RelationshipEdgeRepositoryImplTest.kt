package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeInput
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
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
class RelationshipEdgeRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: RelationshipEdgeRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = RelationshipEdgeRepositoryImpl(apiClient, db.cachedRelationshipEdgeDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `update caches the updated edge`() = runTest {
        val updated = RelationshipEdge(
            id = "e1", sourceId = "u1", targetId = "u2",
            type = RelationshipEdgeTypes.SPOUSE_OF, sensitivity = "private",
        )
        coEvery { apiClient.updateRelationshipEdge("e1", any()) } returns Result.success(updated)

        val result = repository.update(
            "e1",
            RelationshipEdgeInput(sourceId = "u1", targetId = "u2", type = RelationshipEdgeTypes.SPOUSE_OF, sensitivity = "private"),
        )

        assertTrue(result.isSuccess)
        val cached = db.cachedRelationshipEdgeDao().getById("e1")
        assertEquals(RelationshipEdgeTypes.SPOUSE_OF, cached?.type)
        assertEquals("private", cached?.sensitivity)
    }

    @Test
    fun `update failure leaves the cache untouched`() = runTest {
        coEvery { apiClient.updateRelationshipEdge("e1", any()) } returns Result.failure(
            ApiError.Client(404, "Relationship edge not found"),
        )

        val result = repository.update(
            "e1",
            RelationshipEdgeInput(sourceId = "u1", targetId = "u2", type = RelationshipEdgeTypes.SPOUSE_OF),
        )

        assertTrue(result.isFailure)
        assertNull(db.cachedRelationshipEdgeDao().getById("e1"))
    }

    // --- T104 ---

    @Test
    fun `suggest maps the response to its suggested edges`() = runTest {
        val edges = listOf(
            RelationshipEdge(
                id = "e1", sourceId = "u1", targetId = "u2",
                type = RelationshipEdgeTypes.PARENT_OF, status = "suggested", source = "graph-inferred",
            ),
        )
        coEvery { apiClient.suggestRelationshipEdges() } returns
            Result.success(com.mycorrhizal.crm.model.network.RelationshipSuggestionsResponse(suggestedEdges = edges, total = 1))

        val result = repository.suggest()

        assertTrue(result.isSuccess)
        assertEquals(edges, result.getOrThrow())
    }

    @Test
    fun `suggest failure propagates`() = runTest {
        coEvery { apiClient.suggestRelationshipEdges() } returns Result.failure(
            ApiError.Client(500, "boom"),
        )

        val result = repository.suggest()

        assertTrue(result.isFailure)
    }
}
