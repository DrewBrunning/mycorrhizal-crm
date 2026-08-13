package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.Circle
import com.mycorrhizal.crm.model.network.CircleMember
import com.mycorrhizal.crm.model.network.CirclesPage
import com.mycorrhizal.crm.model.network.ContactTag
import com.mycorrhizal.crm.model.network.Tag
import com.mycorrhizal.crm.model.network.TagsPage
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

/**
 * M24: the contact-detail inline editors derive a contact's circles/tags from the join-row
 * snapshot (`include_members`/`include_contacts`), since the contact payload carries neither.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [35])
@GraphicsMode(GraphicsMode.Mode.NATIVE)
class MembershipDerivationRepositoryTest {

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

    @Test
    fun `circlesForContact filters the member snapshot by the contact's uid`() = runTest {
        val repo = CircleRepositoryImpl(apiClient, db.cachedCircleDao(), db.cachedCircleMemberDao())
        coEvery { apiClient.listCircles(cursor = any(), limit = any(), includeMembers = true) } returns Result.success(
            CirclesPage(
                circles = listOf(Circle(id = "c1", name = "friends"), Circle(id = "c2", name = "family")),
                members = listOf(
                    CircleMember(id = 1, circleId = "c1", memberVCardUid = "u5"),
                    CircleMember(id = 2, circleId = "c2", memberVCardUid = "u9"),
                ),
            ),
        )

        val result = repo.circlesForContact("u5")

        assertTrue(result.isSuccess)
        assertEquals(listOf("friends"), result.getOrThrow().map { it.name })
        // The mirror is refreshed wholesale.
        val cachedMembers = db.cachedCircleMemberDao().getByCircleId("c1")
        assertEquals(1, cachedMembers.size)
    }

    @Test
    fun `circlesForContact with a blank uid short-circuits without a network call`() = runTest {
        val repo = CircleRepositoryImpl(apiClient, db.cachedCircleDao(), db.cachedCircleMemberDao())

        val result = repo.circlesForContact("")

        assertTrue(result.isSuccess)
        assertTrue(result.getOrThrow().isEmpty())
        io.mockk.coVerify(exactly = 0) { apiClient.listCircles(any(), any(), any()) }
    }

    @Test
    fun `tagsForContact filters the tagging snapshot by the contact's uid`() = runTest {
        val repo = TagRepositoryImpl(apiClient, db.cachedTagDao(), db.cachedContactTagDao())
        coEvery { apiClient.listTags(cursor = any(), limit = any(), includeContacts = true) } returns Result.success(
            TagsPage(
                tags = listOf(Tag(id = "t1", name = "close"), Tag(id = "t2", name = "work")),
                contacts = listOf(
                    ContactTag(id = 1, tagId = "t1", contactVCardUid = "u5"),
                    ContactTag(id = 2, tagId = "t2", contactVCardUid = "u9"),
                ),
            ),
        )

        val result = repo.tagsForContact("u5")

        assertTrue(result.isSuccess)
        assertEquals(listOf("close"), result.getOrThrow().map { it.name })
    }

    @Test
    fun `tagsForContact propagates a failure rather than returning a partial list`() = runTest {
        val repo = TagRepositoryImpl(apiClient, db.cachedTagDao(), db.cachedContactTagDao())
        coEvery { apiClient.listTags(cursor = any(), limit = any(), includeContacts = true) } returns Result.failure(
            ApiError.Network(java.io.IOException("offline")),
        )

        val result = repo.tagsForContact("u5")

        assertTrue(result.isFailure)
    }
}
