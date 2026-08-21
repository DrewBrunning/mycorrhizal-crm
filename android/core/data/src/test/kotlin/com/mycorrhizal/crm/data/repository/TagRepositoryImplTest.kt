package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.data.local.CachedContactTag
import com.mycorrhizal.crm.data.local.CachedTag
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.ContactTagInput
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.network.TagDetailResponse
import com.mycorrhizal.crm.model.network.TagInput
import com.mycorrhizal.crm.model.network.TagsPage
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
class TagRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: TagRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = TagRepositoryImpl(apiClient, db.cachedTagDao(), db.cachedContactTagDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `list mirrors the page into the cache`() = runTest {
        coEvery { apiClient.listTags(cursor = null, limit = 20) } returns Result.success(
            TagsPage(tags = listOf(Tag(id = "t1", name = "vip"))),
        )

        val result = repository.list(cursor = null, limit = 20)

        assertTrue(result.isSuccess)
        assertEquals(listOf("t1"), result.getOrThrow().map { it.id })
        assertEquals("vip", db.cachedTagDao().getById("t1")?.name)
    }

    @Test
    fun `list failure propagates without touching the cache`() = runTest {
        coEvery { apiClient.listTags(cursor = null, limit = 20) } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.list(cursor = null, limit = 20)

        assertTrue(result.isFailure)
        assertNull(db.cachedTagDao().getById("t1"))
    }

    @Test
    fun `tagsForContact replaces the tagging mirror wholesale`() = runTest {
        db.cachedContactTagDao().upsertAll(
            listOf(CachedContactTag(id = 99, tagId = "stale", contactVCardUid = "u1")),
        )
        coEvery { apiClient.listTags(includeContacts = true, limit = 100) } returns Result.success(
            TagsPage(
                tags = listOf(Tag(id = "t1", name = "vip"), Tag(id = "t2", name = "family")),
                contacts = listOf(
                    ContactTag(id = 1, tagId = "t1", contactVCardUid = "u1"),
                    ContactTag(id = 2, tagId = "t2", contactVCardUid = "u2"),
                ),
            ),
        )

        val result = repository.tagsForContact("u1")

        assertTrue(result.isSuccess)
        assertEquals(listOf("t1"), result.getOrThrow().map { it.id })
        assertTrue(db.cachedContactTagDao().getByTagId("stale").isEmpty())
        assertEquals(1, db.cachedContactTagDao().getByTagId("t1").size)
    }

    @Test
    fun `tagsForContact with a blank uid is a no-op`() = runTest {
        val result = repository.tagsForContact(" ")

        assertTrue(result.isSuccess)
        assertEquals(emptyList<Tag>(), result.getOrThrow())
    }

    @Test
    fun `getWithContacts replaces just that tag's contacts`() = runTest {
        coEvery { apiClient.getTag("t1") } returns Result.success(
            TagDetailResponse(
                tag = Tag(id = "t1", name = "vip"),
                contacts = listOf(ContactTag(id = 1, tagId = "t1", contactVCardUid = "u1")),
            ),
        )

        val result = repository.getWithContacts("t1")

        assertTrue(result.isSuccess)
        assertEquals(1, result.getOrThrow().contacts.size)
        assertEquals("vip", db.cachedTagDao().getById("t1")?.name)
    }

    @Test
    fun `create mirrors the created tag into the cache`() = runTest {
        coEvery { apiClient.createTag(TagInput(name = "vip")) } returns Result.success(Tag(id = "t1", name = "vip"))

        val result = repository.create("vip")

        assertTrue(result.isSuccess)
        assertEquals("vip", db.cachedTagDao().getById("t1")?.name)
    }

    @Test
    fun `delete cascades to the cached tag and its taggings`() = runTest {
        db.cachedTagDao().upsert(CachedTag(id = "t1", name = "vip"))
        db.cachedContactTagDao().upsertAll(listOf(CachedContactTag(id = 1, tagId = "t1", contactVCardUid = "u1")))
        coEvery { apiClient.deleteTag("t1") } returns Result.success(Unit)

        val result = repository.delete("t1")

        assertTrue(result.isSuccess)
        assertNull(db.cachedTagDao().getById("t1"))
        assertTrue(db.cachedContactTagDao().getByTagId("t1").isEmpty())
    }

    @Test
    fun `delete failure leaves the cache untouched`() = runTest {
        db.cachedTagDao().upsert(CachedTag(id = "t1", name = "vip"))
        coEvery { apiClient.deleteTag("t1") } returns Result.failure(ApiError.Server(500, "boom"))

        val result = repository.delete("t1")

        assertTrue(result.isFailure)
        assertEquals("vip", db.cachedTagDao().getById("t1")?.name)
    }

    @Test
    fun `addContact upserts the new tagging`() = runTest {
        coEvery { apiClient.addContactTag("t1", ContactTagInput(contactVCardUid = "u1")) } returns Result.success(
            ContactTag(id = 1, tagId = "t1", contactVCardUid = "u1"),
        )

        val result = repository.addContact("t1", "u1")

        assertTrue(result.isSuccess)
        assertEquals(1, db.cachedContactTagDao().getByTagId("t1").size)
    }

    @Test
    fun `removeContact deletes the cached tagging`() = runTest {
        db.cachedContactTagDao().upsertAll(listOf(CachedContactTag(id = 1, tagId = "t1", contactVCardUid = "u1")))
        coEvery { apiClient.removeContactTag("t1", "u1") } returns Result.success(Unit)

        val result = repository.removeContact("t1", "u1")

        assertTrue(result.isSuccess)
        assertTrue(db.cachedContactTagDao().getByTagId("t1").isEmpty())
    }
}
