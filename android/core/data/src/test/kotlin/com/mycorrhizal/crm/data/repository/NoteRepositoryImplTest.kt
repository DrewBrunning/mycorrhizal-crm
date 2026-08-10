package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.model.network.ContactNotesResponse
import com.mycorrhizal.crm.model.network.Note
import com.mycorrhizal.crm.model.network.NoteInput
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
class NoteRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var apiClient: ApiClient
    private lateinit var repository: NoteRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        apiClient = mockk()
        repository = NoteRepositoryImpl(apiClient, db.cachedNoteDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    @Test
    fun `listForContact caches notes and returns them`() = runTest {
        coEvery { apiClient.listContactNotes(5) } returns Result.success(
            ContactNotesResponse(
                notes = listOf(
                    Note(id = 3, content = "Loves climbing"),
                    Note(id = 4, content = "Met at conference"),
                ),
            ),
        )

        val result = repository.listForContact(5)

        assertTrue(result.isSuccess)
        assertEquals(2, result.getOrThrow().size)

        val cached = db.cachedNoteDao().getAll()
        assertEquals(2, cached.size)
        assertEquals("Loves climbing", cached[0].content)
    }

    @Test
    fun `listForContact propagates a 404`() = runTest {
        coEvery { apiClient.listContactNotes(999) } returns Result.failure(
            ApiError.Client(404, "Contact not found"),
        )

        val result = repository.listForContact(999)

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(404, (error as ApiError.Client).code)
    }

    @Test
    fun `create caches the created note`() = runTest {
        coEvery { apiClient.createNote(5, any()) } returns Result.success(
            Note(id = 3, content = "Loves climbing"),
        )

        val result = repository.create(5, NoteInput(content = "Loves climbing"))

        assertTrue(result.isSuccess)
        assertEquals("Loves climbing", db.cachedNoteDao().getById(3)?.content)
    }

    @Test
    fun `update caches the updated note`() = runTest {
        coEvery { apiClient.updateNote(3, any()) } returns Result.success(
            Note(id = 3, content = "Loves rock climbing"),
        )

        val result = repository.update(3, NoteInput(content = "Loves rock climbing"))

        assertTrue(result.isSuccess)
        assertEquals("Loves rock climbing", db.cachedNoteDao().getById(3)?.content)
    }

    @Test
    fun `create propagates a validation failure`() = runTest {
        coEvery { apiClient.createNote(5, any()) } returns Result.failure(
            ApiError.Client(400, "Note content is required"),
        )

        val result = repository.create(5, NoteInput(content = ""))

        assertTrue(result.isFailure)
        val error = result.exceptionOrNull() as ApiError
        assertEquals(400, (error as ApiError.Client).code)
    }
}
