package com.mycorrhizal.crm.data.local

import java.io.File
import org.junit.After
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Issue #998: JVM unit coverage for [RoomDatabaseRecovery.deleteDatabaseFiles]'s
 * pure `java.io.File` I/O. [RoomDatabaseRecovery.openOrRebuild] itself needs a
 * real Room/SQLCipher database to open (or fail to open), so it stays
 * instrumented-only — see `RoomDatabaseRecoveryTest` in `app/src/androidTest`,
 * which exercises the whole recovery flow end to end. This mirrors
 * [RoomCacheEncryptionTest]'s split between a pure-I/O JVM test and an
 * instrumented one for the same reason.
 */
class RoomDatabaseRecoveryTest {

    private lateinit var dir: File
    private val files = mutableListOf<File>()

    @Before
    fun setUp() {
        dir = File.createTempFile("room-recovery-", "").apply {
            delete()
            mkdirs()
        }
    }

    @After
    fun tearDown() {
        files.forEach { it.delete() }
        dir.deleteRecursively()
    }

    private fun file(name: String): File = File(dir, name).also { files += it }

    @Test
    fun `deletes the main database file`() {
        val db = file("cache.db").apply { writeText("data") }

        RoomDatabaseRecovery.deleteDatabaseFiles(db)

        assertFalse("The main db file must be gone", db.exists())
    }

    @Test
    fun `deletes the journal, wal and shm sidecars alongside the main file`() {
        val db = file("cache.db").apply { writeText("data") }
        val journal = file("cache.db-journal").apply { writeText("j") }
        val wal = file("cache.db-wal").apply { writeText("w") }
        val shm = file("cache.db-shm").apply { writeText("s") }

        RoomDatabaseRecovery.deleteDatabaseFiles(db)

        assertFalse(db.exists())
        assertFalse("The -journal sidecar must be deleted", journal.exists())
        assertFalse("The -wal sidecar must be deleted", wal.exists())
        assertFalse("The -shm sidecar must be deleted", shm.exists())
    }

    @Test
    fun `is a no-op, not a crash, when nothing exists yet`() {
        val db = file("never-created.db")

        RoomDatabaseRecovery.deleteDatabaseFiles(db)

        assertFalse(db.exists())
    }

    @Test
    fun `only deletes this database's own sidecars, not an unrelated file with a similar name`() {
        val db = file("cache.db").apply { writeText("data") }
        val unrelated = file("cache.db.bak").apply { writeText("keep me") }

        RoomDatabaseRecovery.deleteDatabaseFiles(db)

        assertFalse(db.exists())
        assertTrue("A file that merely starts with the db name must survive", unrelated.exists())
    }
}
