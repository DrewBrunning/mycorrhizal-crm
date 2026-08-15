package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.model.network.CirclesPage
import com.mycorrhizal.crm.model.network.GraphConnectionsResponse
import com.mycorrhizal.crm.model.network.UserProfile
import com.mycorrhizal.crm.network.ApiClient
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class GraphRepositoryImplTest {

    private val apiClient = mockk<ApiClient>()
    private val repository = GraphRepositoryImpl(apiClient)

    @Test
    fun `getConnections passes through to the api client`() = runTest {
        coEvery { apiClient.getConnections(from = "u1", depth = 2, relation = "brother") } returns
            Result.success(GraphConnectionsResponse(fromVCardUid = "u1"))

        val result = repository.getConnections(from = "u1", depth = 2, relation = "brother")

        assertTrue(result.isSuccess)
        coVerify { apiClient.getConnections(from = "u1", depth = 2, relation = "brother") }
    }

    @Test
    fun `selfContactVCardUid reads the profile's pointer`() = runTest {
        coEvery { apiClient.currentUser() } returns Result.success(
            UserProfile(id = 1, selfContactVCardUid = "self-uid"),
        )

        val result = repository.selfContactVCardUid()

        assertTrue(result.isSuccess)
        assertEquals("self-uid", result.getOrThrow())
    }

    @Test
    fun `selfContactVCardUid is null when no self contact is set`() = runTest {
        coEvery { apiClient.currentUser() } returns Result.success(UserProfile(id = 1))

        val result = repository.selfContactVCardUid()

        assertTrue(result.isSuccess)
        assertEquals(null, result.getOrThrow())
    }

    @Test
    fun `circlesWithMembers maps the page into per-circle member uid sets`() = runTest {
        coEvery { apiClient.listCircles(includeMembers = true, limit = 100) } returns Result.success(
            CirclesPage(
                circles = listOf(
                    com.mycorrhizal.crm.model.network.Circle(id = "c1", name = "Family"),
                    com.mycorrhizal.crm.model.network.Circle(id = "c2", name = "Work"),
                    com.mycorrhizal.crm.model.network.Circle(id = "c3", name = "Empty"),
                ),
                members = listOf(
                    com.mycorrhizal.crm.model.network.CircleMember(circleId = "c1", memberVCardUid = "u1"),
                    com.mycorrhizal.crm.model.network.CircleMember(circleId = "c1", memberVCardUid = "u2"),
                    com.mycorrhizal.crm.model.network.CircleMember(circleId = "c2", memberVCardUid = "u2"),
                ),
            ),
        )

        val result = repository.circlesWithMembers()

        assertTrue(result.isSuccess)
        val byId = result.getOrThrow().associateBy { it.id }
        assertEquals(setOf("u1", "u2"), byId.getValue("c1").memberVCardUids)
        assertEquals(setOf("u2"), byId.getValue("c2").memberVCardUids)
        assertTrue(byId.getValue("c3").memberVCardUids.isEmpty())
        assertEquals("Family", byId.getValue("c1").name)
    }

    @Test
    fun `circlesWithMembers propagates a failed fetch`() = runTest {
        coEvery { apiClient.listCircles(includeMembers = true, limit = 100) } returns
            Result.failure(com.mycorrhizal.crm.network.ApiError.Client(500, "boom"))

        val result = repository.circlesWithMembers()

        assertTrue(result.isFailure)
    }
}
