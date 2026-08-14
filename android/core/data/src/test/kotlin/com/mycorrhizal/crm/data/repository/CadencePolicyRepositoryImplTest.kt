package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedCadencePolicy
import com.mycorrhizal.crm.model.network.CadenceHealth
import com.mycorrhizal.crm.model.network.CadencePoliciesResponse
import com.mycorrhizal.crm.model.network.CadencePolicy
import com.mycorrhizal.crm.model.network.CadencePolicyInput
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
class CadencePolicyRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: CadencePolicyRepositoryImpl

    private val uid = "11111111-1111-1111-1111-111111111111"

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = CadencePolicyRepositoryImpl(apiClient, db.cachedCadencePolicyDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun policy(
        id: String = "p1",
        interval: Int = 30,
        qualifyingTypes: List<String> = emptyList(),
        deleted: Boolean = false,
    ) = CadencePolicy(
        id = id,
        entityId = uid,
        targetIntervalDays = interval,
        qualifyingTypes = qualifyingTypes,
        deleted = deleted,
        health = CadenceHealth(hasQualifyingInteraction = true, overdueBy = 0),
    )

    @Test
    fun `listForContact mirrors the fetched policies into the cache`() = runTest {
        coEvery { apiClient.listCadencePolicies(uid) } returns Result.success(
            CadencePoliciesResponse(cadencePolicies = listOf(policy("p1", 30), policy("p2", 45))),
        )

        val result = repository.listForContact(uid)

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().size)
        val cached = db.cachedCadencePolicyDao().getForContact(uid)
        assertEquals(listOf("p1", "p2"), cached.map { it.id })
    }

    @Test
    fun `listForContact full-resyncs the cache for the contact`() = runTest {
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "stale", entityId = uid, targetIntervalDays = 30),
        )
        coEvery { apiClient.listCadencePolicies(uid) } returns Result.success(
            CadencePoliciesResponse(cadencePolicies = listOf(policy("p1", 30))),
        )

        val result = repository.listForContact(uid)

        assertTrue(result.isSuccess)
        val cached = db.cachedCadencePolicyDao().getForContact(uid)
        assertEquals(listOf("p1"), cached.map { it.id })
    }

    @Test
    fun `listForContact drops tombstones from the cache but returns the raw list`() = runTest {
        coEvery { apiClient.listCadencePolicies(uid) } returns Result.success(
            CadencePoliciesResponse(cadencePolicies = listOf(policy("p1", 30, deleted = true))),
        )

        val result = repository.listForContact(uid)

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().size)
        assertTrue(db.cachedCadencePolicyDao().getForContact(uid).isEmpty())
    }

    @Test
    fun `create caches the created policy`() = runTest {
        val created = policy("p1", 14, listOf("call"))
        coEvery { apiClient.createCadencePolicy(any()) } returns Result.success(created)

        val result = repository.create(CadencePolicyInput(uid, 14, listOf("call")))

        assertTrue(result.isSuccess)
        val cached = db.cachedCadencePolicyDao().getForContact(uid)
        assertEquals(listOf("p1"), cached.map { it.id })
        assertEquals(listOf("call"), cached[0].qualifyingTypes)
    }

    @Test
    fun `update replaces the cached policy`() = runTest {
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "p1", entityId = uid, targetIntervalDays = 30),
        )
        val updated = policy("p1", 90)
        coEvery { apiClient.updateCadencePolicy("p1", any()) } returns Result.success(updated)

        val result = repository.update("p1", CadencePolicyInput(uid, 90, emptyList()))

        assertTrue(result.isSuccess)
        assertEquals(90, db.cachedCadencePolicyDao().getForContact(uid)[0].targetIntervalDays)
    }

    @Test
    fun `delete removes the cached policy`() = runTest {
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "p1", entityId = uid, targetIntervalDays = 30),
        )
        coEvery { apiClient.deleteCadencePolicy("p1") } returns Result.success(Unit)

        val result = repository.delete("p1")

        assertTrue(result.isSuccess)
        assertTrue(db.cachedCadencePolicyDao().getForContact(uid).isEmpty())
    }

    @Test
    fun `a failed delete keeps the cached policy`() = runTest {
        db.cachedCadencePolicyDao().upsert(
            CachedCadencePolicy(id = "p1", entityId = uid, targetIntervalDays = 30),
        )
        coEvery { apiClient.deleteCadencePolicy("p1") } returns Result.failure(
            ApiError.Client(404, "Not found"),
        )

        val result = repository.delete("p1")

        assertTrue(result.isFailure)
        assertEquals(30, db.cachedCadencePolicyDao().getForContact(uid)[0].targetIntervalDays)
    }
}
