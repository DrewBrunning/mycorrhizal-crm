package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.room.Room
import androidx.test.core.app.ApplicationProvider
import com.mycorrhizal.crm.data.local.AppDatabase
import com.mycorrhizal.crm.domain.repository.CustomLinkAction
import kotlinx.coroutines.runBlocking
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
class CustomLinkActionRepositoryImplTest {

    private lateinit var db: AppDatabase
    private lateinit var repository: CustomLinkActionRepositoryImpl

    @Before
    fun setup() {
        val context = ApplicationProvider.getApplicationContext<Context>()
        db = Room.inMemoryDatabaseBuilder(context, AppDatabase::class.java).build()
        repository = CustomLinkActionRepositoryImpl(db.customLinkActionDao())
    }

    @After
    fun teardown() {
        db.close()
    }

    private fun action(protocol: String, label: String = "Open") = CustomLinkAction(
        protocol = protocol,
        label = label,
        kind = "APP_OPEN",
        mimeType = "text/plain",
        intentUriTemplate = "$protocol://open?id={value}",
    )

    @Test
    fun `upsert then getAll returns the stored action`() = runBlocking {
        repository.upsert(action("matrix"))

        val all = repository.getAll()

        assertEquals(1, all.size)
        assertEquals("matrix", all.single().protocol)
    }

    @Test
    fun `getForProtocol filters to just that protocol`() = runBlocking {
        repository.upsert(action("matrix"))
        repository.upsert(action("signal"))

        val matrixActions = repository.getForProtocol("matrix")

        assertEquals(listOf("matrix"), matrixActions.map { it.protocol })
    }

    @Test
    fun `upsert replaces an existing row with the same id`() = runBlocking {
        repository.upsert(action("matrix", label = "Open"))
        val stored = repository.getAll().single()

        repository.upsert(stored.copy(label = "Open in Matrix"))

        assertEquals("Open in Matrix", repository.getAll().single().label)
    }

    @Test
    fun `delete removes the stored action`() = runBlocking {
        repository.upsert(action("matrix"))
        val stored = repository.getAll().single()

        repository.delete(stored)

        assertTrue(repository.getAll().isEmpty())
    }
}
