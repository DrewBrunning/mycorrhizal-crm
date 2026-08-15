package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.AcceptHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.AddressHouseholdSuggestion
import com.mycorrhizal.crm.model.network.AddressSuggestionsResponse
import com.mycorrhizal.crm.model.network.DismissHouseholdSuggestionInput
import com.mycorrhizal.crm.model.network.Household
import com.mycorrhizal.crm.model.network.HouseholdMember
import com.mycorrhizal.crm.model.network.HouseholdMemberInput
import com.mycorrhizal.crm.model.network.HouseholdTypes
import com.mycorrhizal.crm.model.network.RelationshipEdge
import com.mycorrhizal.crm.model.network.RelationshipEdgeTypes
import com.mycorrhizal.crm.model.network.SuggestRelationshipsResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import io.mockk.coEvery
import io.mockk.mockk
import io.mockk.slot
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
class HouseholdRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: HouseholdRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = HouseholdRepositoryImpl(apiClient, db.cachedHouseholdDao(), db.cachedHouseholdMemberDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `suggestRelationships maps the suggested edges from the response`() = runTest {
        val edge = RelationshipEdge(
            id = "e1",
            sourceId = "u1",
            targetId = "u2",
            type = RelationshipEdgeTypes.SPOUSE_OF,
            status = "suggested",
        )
        coEvery { apiClient.suggestHouseholdRelationships("h1") } returns Result.success(
            SuggestRelationshipsResponse(
                message = "Relationship suggestions generated",
                householdId = "h1",
                suggestedEdges = listOf(edge),
                total = 1,
            ),
        )

        val result = repository.suggestRelationships("h1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("e1"), result.getOrThrow().map { it.id })
    }

    @Test
    fun `suggestRelationships failure propagates`() = runTest {
        coEvery { apiClient.suggestHouseholdRelationships("h1") } returns Result.failure(
            ApiError.Client(404, "Household not found"),
        )

        val result = repository.suggestRelationships("h1")

        assertTrue(result.isFailure)
    }

    @Test
    fun `suggestAddressHouseholds maps the suggestions from the response`() = runTest {
        coEvery { apiClient.suggestAddressHouseholds() } returns Result.success(
            AddressSuggestionsResponse(
                suggestions = listOf(
                    AddressHouseholdSuggestion(
                        addressHash = "ah1",
                        memberHash = "mh1",
                        memberVCardUids = listOf("u1", "u2"),
                    ),
                ),
                total = 1,
            ),
        )

        val result = repository.suggestAddressHouseholds()

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().size)
        assertEquals(listOf("u1", "u2"), result.getOrThrow()[0].memberVCardUids)
    }

    @Test
    fun `acceptAddressSuggestion upserts the household into the cache and returns it`() = runTest {
        val household = Household(id = "h9", name = "Alice & Bob", type = HouseholdTypes.FAMILY_UNIT)
        coEvery { apiClient.acceptHouseholdSuggestion(any()) } returns Result.success(household)

        val result = repository.acceptAddressSuggestion(
            AcceptHouseholdSuggestionInput(memberVCardUids = listOf("u1", "u2")),
        )

        assertTrue(result.isSuccess)
        assertEquals("h9", result.getOrThrow().id)
        assertEquals("Alice & Bob", db.cachedHouseholdDao().getById("h9")?.name)
    }

    @Test
    fun `acceptAddressSuggestion failure leaves the cache untouched`() = runTest {
        coEvery { apiClient.acceptHouseholdSuggestion(any()) } returns Result.failure(
            ApiError.Client(409, "already in a household"),
        )

        val result = repository.acceptAddressSuggestion(
            AcceptHouseholdSuggestionInput(memberVCardUids = listOf("u1", "u2")),
        )

        assertTrue(result.isFailure)
        assertNull(db.cachedHouseholdDao().getById("h9"))
    }

    @Test
    fun `dismissAddressSuggestion delegates to the api client`() = runTest {
        coEvery { apiClient.dismissHouseholdSuggestion(any()) } returns Result.success(Unit)

        val result = repository.dismissAddressSuggestion(
            DismissHouseholdSuggestionInput(memberVCardUids = listOf("u1", "u2")),
        )

        assertTrue(result.isSuccess)
    }

    @Test
    fun `addMember normalizes a null role to an empty string on the wire`() = runTest {
        val member = HouseholdMember(id = 1, householdId = "h1", memberVCardUid = "u1")
        coEvery { apiClient.addHouseholdMember("h1", any()) } returns Result.success(member)

        repository.addMember("h1", "u1", null)

        val inputSlot = slot<HouseholdMemberInput>()
        io.mockk.coVerify { apiClient.addHouseholdMember("h1", capture(inputSlot)) }
        assertEquals("", inputSlot.captured.role)
    }

    @Test
    fun `updateMember normalizes a null role to an empty string on the wire`() = runTest {
        coEvery { apiClient.updateHouseholdMember("h1", "u1", any()) } returns Result.success(Unit)
        coEvery { apiClient.getHousehold("h1") } returns Result.success(
            com.mycorrhizal.crm.model.network.HouseholdDetailResponse(
                household = Household(id = "h1", name = "Home", type = HouseholdTypes.FAMILY_UNIT),
                members = emptyList(),
            ),
        )

        repository.updateMember("h1", "u1", null)

        val inputSlot = slot<HouseholdMemberInput>()
        io.mockk.coVerify { apiClient.updateHouseholdMember("h1", "u1", capture(inputSlot)) }
        assertEquals("", inputSlot.captured.role)
    }
}
