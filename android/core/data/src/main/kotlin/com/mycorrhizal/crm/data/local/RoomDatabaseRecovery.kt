package com.mycorrhizal.crm.data.local

import android.util.Log
import java.io.File

/**
 * Issue #998: recovers from a Room mirror that fails to open under its
 * current passphrase — most notably a database left encrypted with a
 * passphrase that was since lost (e.g. a pre-fix build hit the
 * [RoomPassphraseStore] async-persist race: the passphrase used to encrypt
 * the file never survived a process death, so no key this process can ever
 * produce again will open it).
 *
 * The mirror is a rebuildable cache for every table except
 * `pending_interactions` (see [AppDatabase]'s doc comment) — but a file this
 * process cannot open at all is cryptographically inaccessible regardless,
 * so whatever it held, including any queued `pending_interactions` rows, is
 * already unrecoverable. Wiping it and rebuilding fresh trades that
 * already-lost data for a working app on this and every later launch,
 * instead of a permanent "wrong key" boot loop.
 */
object RoomDatabaseRecovery {

    private const val TAG = "RoomDatabaseRecovery"

    /**
     * Builds a database via [build] and eagerly forces its underlying store
     * open — Room's own open is otherwise lazy, deferred to the first real
     * query — so a passphrase mismatch fails here, during DI wiring, instead
     * of as a mysterious crash the first time some screen queries a DAO. On
     * failure, [dbFile] and its `-journal`/`-wal`/`-shm` sidecars are deleted
     * and [build] is invoked again to construct a fresh, empty database.
     *
     * Merely opening a SQLCipher file handle with the wrong key does not
     * itself fail — decryption is only exercised once a page is actually
     * read — so this runs the query SQLCipher's own docs recommend for
     * testing a key (`SELECT count(*) FROM sqlite_master`) rather than
     * relying on `writableDatabase` alone happening to read one internally.
     */
    // detekt(TooGenericExceptionCaught): opening a SQLCipher database surfaces
    // a wrong/lost key as a plain SQLiteException from native code; any
    // failure here funnels into the same wipe-and-rebuild recovery.
    @Suppress("TooGenericExceptionCaught")
    fun openOrRebuild(dbFile: File, build: () -> AppDatabase): AppDatabase { // # pragma: no cover — needs a real Room/SQLCipher database; see RoomDatabaseRecoveryTest (app/androidTest)
        val db = build() // # pragma: no cover
        try { // # pragma: no cover
            db.openHelper.writableDatabase.query("SELECT count(*) FROM sqlite_master", emptyArray()).use { // # pragma: no cover
                it.moveToFirst() // # pragma: no cover
            } // # pragma: no cover
            return db // # pragma: no cover
        } catch (e: Exception) { // # pragma: no cover
            Log.e(TAG, "Room mirror at ${dbFile.name} failed to open; wiping and rebuilding it", e) // # pragma: no cover
            db.close() // # pragma: no cover
            deleteDatabaseFiles(dbFile) // # pragma: no cover
            return build() // # pragma: no cover
        } // # pragma: no cover
    }

    /**
     * The pure file-I/O half of [openOrRebuild]'s recovery — split out so it
     * has real JVM unit coverage ([RoomDatabaseRecoveryTest] in
     * `core/data/src/test`) even though the SQLCipher-dependent caller around
     * it does not.
     */
    internal fun deleteDatabaseFiles(dbFile: File) {
        dbFile.delete()
        listOf("-journal", "-wal", "-shm").forEach { suffix ->
            File(dbFile.parentFile, dbFile.name + suffix).delete()
        }
    }
}
