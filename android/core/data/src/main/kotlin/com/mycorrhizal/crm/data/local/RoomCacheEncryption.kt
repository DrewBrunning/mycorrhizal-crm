package com.mycorrhizal.crm.data.local

import android.util.Log
import java.io.File
import java.io.FileInputStream
import java.io.FileOutputStream
import java.security.SecureRandom
import kotlin.math.min
import net.zetetic.database.sqlcipher.SQLiteDatabase

/**
 * Issue #385: whole-DB encryption of the Room mirror via SQLCipher, including
 * the one-time transition of a pre-existing plaintext database.
 *
 * A plaintext SQLite file begins with the 16-byte `SQLite format 3` header;
 * a SQLCipher file begins with random bytes (a fresh salt), so the header
 * cleanly distinguishes the two. When an old plaintext DB is detected it is
 * re-encrypted in place via `sqlcipher_export` — the officially recommended
 * transition — into a temp encrypted file that is then swapped in and the
 * plaintext file securely overwritten before deletion.
 *
 * `sqlcipher_export` duplicates every object (schema, triggers, **virtual
 * tables**, data), so the FTS4 `cached_contacts_fts` mirror and — critically —
 * the non-rebuildable `pending_interactions` outbox and device-local
 * `custom_link_actions` all survive the transition. The FTS virtual table's
 * index does not survive (see [rebuildFtsMirrors]) and is rebuilt from the
 * surviving base content. No table is dropped or rebuilt from the server.
 *
 * Requires the SQLCipher native library; callers must invoke [ensureEncrypted]
 * (or otherwise `System.loadLibrary("sqlcipher")`) before opening the DB.
 */
object RoomCacheEncryption {

    private const val TAG = "RoomCacheEncryption"
    private const val PLAINTEXT_MAGIC = "SQLite format 3\u0000"

    /**
     * Runs before Room opens [dbFile]: loads the SQLCipher native library and,
     * if [dbFile] is still a plaintext database from a pre-#385 install,
     * re-encrypts it with [passphrase] in place. No-op on a fresh or already
     * encrypted database.
     */
    fun ensureEncrypted(dbFile: File, passphrase: String) {
        System.loadLibrary("sqlcipher")
        if (!dbFile.exists()) return
        if (isEncrypted(dbFile)) return
        reencryptInPlace(dbFile, passphrase)
    }

    /** True when [dbFile] is not a plaintext SQLite file (fresh/encrypted/corrupt). */
    fun isEncrypted(dbFile: File): Boolean {
        if (!dbFile.exists() || dbFile.length() < PLAINTEXT_MAGIC.length) return true
        val header = FileInputStream(dbFile).use { stream ->
            val bytes = ByteArray(PLAINTEXT_MAGIC.length)
            val read = stream.read(bytes)
            if (read < bytes.size) return true
            String(bytes, Charsets.ISO_8859_1)
        }
        return header != PLAINTEXT_MAGIC
    }

    private fun reencryptInPlace(dbFile: File, passphrase: String) {
        runReencryption(
            dbFile = dbFile,
            export = { encrypted -> exportToEncrypted(dbFile, encrypted, passphrase) },
            rebuildFts = { rebuildFtsMirrors(dbFile, passphrase) },
        )
    }

    /**
     * The crash-recovery state machine behind [reencryptInPlace], with its two
     * SQLCipher-touching steps injected as [export] (copy every object — schema,
     * triggers, virtual tables, rows — into a fresh encrypted file) and
     * [rebuildFts] (rebuild the FTS index afterward), plus [postSwap] (defaults
     * to the real [deleteSidecars]) so a failure strictly after the swap has
     * completed can be simulated too. Everything else here — the secure-erase
     * overwrite, the rename/copy swap, sidecar cleanup, and above all the
     * three-way recovery decision (export failed / swap failed / a post-swap
     * step failed) — is plain `java.io.File` control flow with no native
     * dependency, so it is `internal` rather than `private`: it lets
     * [RoomCacheEncryptionTest] (JVM, same module) drive every recovery branch
     * directly with fakes. The real [export]/[rebuildFts] lambdas call into
     * `net.zetetic.database.sqlcipher.SQLiteDatabase`, which needs
     * `libsqlcipher.so` — an Android-native (Bionic) binary that cannot load
     * even under Robolectric (see that test's class doc, and
     * `Migration13To14Test`'s), so only the instrumented
     * `RoomCacheEncryptionTest` in `app/src/androidTest` can exercise the real
     * SQLCipher calls end to end.
     *
     * Bug fix (found while adding the crash-recovery tests: `swapped` used to
     * flip to `true` only *after* [deleteSidecars] returned, i.e. as the very
     * last statement of the try block — so a failure there could never take
     * the `if (swapped)` branch below; it fell through to `encrypted.exists()`
     * instead, which is false once the rename above already moved the file,
     * so it took the *last* branch and threw claiming "cache left untouched"
     * even though the swap had already fully succeeded. `swapped` now flips
     * right after the swap itself, before the (best-effort) [postSwap] call,
     * so a sidecar-cleanup failure is correctly treated as non-fatal.
     *
     * detekt(TooGenericExceptionCaught): the transition funnels every failure
     * (SQLCipher errors, IO, SQL) into one of two deliberate outcomes — rethrow
     * as a fatal boot error when the plaintext is intact, or log-and-continue
     * when the swap already succeeded.
     */
    @Suppress("TooGenericExceptionCaught")
    internal fun runReencryption(
        dbFile: File,
        export: (encrypted: File) -> Unit,
        rebuildFts: () -> Unit,
        postSwap: (File) -> Unit = ::deleteSidecars,
    ) {
        val encrypted = File(dbFile.parentFile, dbFile.name + ".encrypted")
        encrypted.delete()
        var swapped = false
        try {
            export(encrypted)

            // Overwrite the plaintext pages with random bytes before unlinking so
            // they are not recoverable from freed blocks (the "stolen device"
            // story the encryption exists for), then swap in the encrypted copy.
            overwriteFile(dbFile)
            dbFile.delete()
            if (!encrypted.renameTo(dbFile)) {
                encrypted.copyTo(dbFile, overwrite = true)
                encrypted.delete()
            }
            swapped = true
            // The old plaintext WAL/journal sidecars may hold uncheckpointed
            // plaintext pages; the encrypted file is self-contained, so drop them.
            // Best-effort: dbFile is already the live encrypted database at this
            // point, so a failure here must not be reported as encryption having
            // failed (see the bug-fix note above).
            postSwap(dbFile)
        } catch (e: Exception) {
            if (swapped) {
                // A post-swap step failed (sidecar deletion); the encrypted DB is live.
                Log.e(TAG, "Encryption completed but a post-swap step failed", e)
            } else if (encrypted.exists()) {
                // The export succeeded but the swap didn't (a secure-erase IO
                // failure can leave the plaintext corrupt mid-overwrite).
                // Recover by completing the swap — the encrypted copy is ready.
                try {
                    dbFile.delete()
                    if (!encrypted.renameTo(dbFile)) {
                        encrypted.copyTo(dbFile, overwrite = true)
                        encrypted.delete()
                    }
                    swapped = true
                    Log.e(TAG, "Recovered a failed swap by completing it", e)
                } catch (recovery: Exception) {
                    Log.e(TAG, "Recovery swap also failed after export succeeded", recovery)
                    encrypted.delete()
                    throw IllegalStateException("Failed to encrypt ${dbFile.name}; cache left untouched", e)
                }
            } else {
                // The export itself failed; the plaintext DB is untouched.
                throw IllegalStateException("Failed to encrypt ${dbFile.name}; cache left untouched", e)
            }
        }
        // FTS rebuild is best-effort: the base rows survived the export, so a
        // rebuild failure degrades search only — it must not block boot.
        // detekt(TooGenericExceptionCaught): the rebuild touches SQLCipher and
        // SQLite virtual-table internals; any failure is intentionally swallowed.
        @Suppress("TooGenericExceptionCaught")
        try {
            rebuildFts()
        } catch (e: Exception) {
            Log.w(TAG, "FTS mirror rebuild failed after encryption transition", e)
        }
        Log.i(TAG, "Re-encrypted ${dbFile.name} in place (${dbFile.length()} bytes encrypted)")
    }

    /** Opens [dbFile] as plaintext and copies every object into a fresh encrypted DB. */
    private fun exportToEncrypted(dbFile: File, encrypted: File, passphrase: String) {
        // Empty passphrase = open the plaintext DB as-is (SQLCipher's documented
        // way to migrate a standard database). ATTACH a brand-new encrypted
        // database and copy every object (tables, triggers, virtual tables incl.
        // FTS4, and all rows) into it.
        val plain = SQLiteDatabase.openOrCreateDatabase(dbFile, "", null, null, null)
        try {
            plain.rawExecSQL("ATTACH DATABASE '${encrypted.absolutePath}' AS encrypted KEY '$passphrase'")
            plain.rawExecSQL("SELECT sqlcipher_export('encrypted')")
            plain.rawExecSQL("DETACH DATABASE encrypted")
        } finally {
            plain.close()
        }
    }

    /**
     * `sqlcipher_export` copies an external-content FTS4 virtual table's shadow
     * tables as inert data — the table is present but its index does not work
     * against the encrypted DB, even though the base table's rows survive.
     * Drop each FTS virtual table, recreate it from its own stored DDL (the
     * `CREATE VIRTUAL TABLE` statement in `sqlite_master`, copied verbatim by
     * the export), then rebuild the index from the surviving base content —
     * the same 'rebuild' command Room's own migration pipeline uses.
     */
    private fun rebuildFtsMirrors(dbFile: File, passphrase: String) {
        val db = SQLiteDatabase.openOrCreateDatabase(dbFile, passphrase, null, null, null)
        try {
            val ftsTables = db.rawQuery(
                "SELECT name FROM sqlite_master WHERE type = 'table' AND sql LIKE 'CREATE VIRTUAL TABLE%USING FTS%'",
                null,
            ).use { cursor ->
                val names = mutableListOf<String>()
                while (cursor.moveToNext()) names += cursor.getString(0)
                names
            }
            ftsTables.forEach { name ->
                val ddl = db.rawQuery("SELECT sql FROM sqlite_master WHERE name = '$name'", null)
                    .use { if (it.moveToFirst()) it.getString(0) else null }
                if (ddl == null) return@forEach
                db.rawExecSQL("DROP TABLE `$name`")
                db.rawExecSQL(ddl)
                db.rawExecSQL("INSERT INTO `$name`(`$name`) VALUES('rebuild')")
            }
        } finally {
            db.close()
        }
    }

    /**
     * Secure-erase: overwrite every byte of [file] with fresh random data
     * before it is unlinked, so the plaintext pages are not recoverable from
     * freed disk blocks. `internal` (rather than `private`) only so
     * [RoomCacheEncryptionTest] can assert on it directly — plain
     * `java.io.File`/`SecureRandom`, no native dependency.
     *
     * Bug fix (found via the new JVM tests): the original length was read
     * from [file] *after* `FileOutputStream(file)` had already opened (and
     * therefore truncated) it, so `remaining` was always `0` and the write
     * loop below never ran a single iteration — the file silently went
     * straight from its real content to empty, with the actual disk blocks
     * that held the plaintext never overwritten at all. That is exactly the
     * "stolen device, data recoverable from freed blocks" scenario this
     * function exists to prevent. The length must be captured before the
     * truncating stream is opened.
     */
    internal fun overwriteFile(file: File) {
        val length = file.length()
        val buffer = ByteArray(64 * 1024)
        SecureRandom().nextBytes(buffer)
        FileOutputStream(file).use { out ->
            var remaining = length
            while (remaining > 0) {
                val chunk = min(remaining, buffer.size.toLong()).toInt()
                out.write(buffer, 0, chunk)
                remaining -= chunk
            }
            out.fd.sync()
        }
    }

    private fun deleteSidecars(dbFile: File) {
        listOf("-journal", "-wal", "-shm").forEach { suffix ->
            File(dbFile.parentFile, dbFile.name + suffix).delete()
        }
    }
}
