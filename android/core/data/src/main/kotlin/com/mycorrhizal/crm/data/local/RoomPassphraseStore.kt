package com.mycorrhizal.crm.data.local

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import java.security.SecureRandom

/**
 * Issue #385: the SQLCipher passphrase for the Room mirror
 * (`mycorrhizal-cache.db`), kept out of the plaintext prefs file entirely.
 *
 * A random 32-byte passphrase is generated on first use and stored in
 * [EncryptedSharedPreferences] behind a Keystore [MasterKey] — the same
 * AES-256-GCM/SIV scheme [EncryptedTokenStorage] uses for the JWT — so the
 * database key is bound to the device Keystore. Hex-encoded (64 ASCII chars)
 * so it is safe to splice into SQL as a `KEY '<hex>'` literal during the
 * plaintext→encrypted transition without quoting concerns.
 *
 * Security posture mirrors the token store: if the Keystore key is lost (e.g.
 * factory reset / OS reinstall), the encrypted DB is unreadable — "lost key =
 * lost cache" — which is acceptable for a rebuildable mirror but means the
 * [RoomCacheEncryption] transition must never run against a passphrase that
 * was not just persisted here (a wrong key would make the migrated DB
 * permanently unreadable).
 *
 * Issue #998: [getOrCreate] therefore writes with [SharedPreferences.Editor.commit]
 * (synchronous, blocks until the value is durably on disk) rather than `apply()`
 * (queues an async write and returns immediately), and reads the value back to
 * confirm it landed, before a freshly generated passphrase is ever handed to
 * the encrypt-in-place step. `apply()` left a window where a process death
 * before the queued write flushed meant the next launch generated a *different*
 * passphrase and used it to open a database that was actually encrypted with
 * the lost one — permanently undecryptable, with nothing here to recover it.
 * [RoomDatabaseRecovery] is the backstop for a database already left in that
 * state (by a pre-fix build, or any other cause) by the time this runs.
 */
class RoomPassphraseStore(context: Context) {

    private val prefs: SharedPreferences = run {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            FILE_NAME,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    /**
     * Returns the persisted passphrase, generating + persisting one on first
     * use. The persist is synchronous (`commit()`, not `apply()`) and
     * verified with a read-back before returning, so a caller that goes on to
     * encrypt the database with this value is never handed a passphrase that
     * only exists in memory (issue #998).
     */
    fun getOrCreate(): String {
        prefs.getString(KEY_PASSPHRASE, null)?.let { return it }
        val passphrase = generatePassphrase()
        val committed = prefs.edit().putString(KEY_PASSPHRASE, passphrase).commit() // # pragma: no cover — needs a real Android Keystore (see RoomEncryptionGuardTest)
        check(committed && prefs.getString(KEY_PASSPHRASE, null) == passphrase) { // # pragma: no cover
            "Failed to durably persist the Room passphrase; refusing to encrypt the cache " + // # pragma: no cover
                "with a value that might not survive a process death" // # pragma: no cover
        } // # pragma: no cover
        return passphrase
    }

    /** Drops the stored passphrase (only safe alongside deleting the DB file). */
    fun clear() {
        prefs.edit().remove(KEY_PASSPHRASE).apply()
    }

    private fun generatePassphrase(): String = randomHexPassphrase()

    companion object {
        private const val FILE_NAME = "secure_room"
        private const val KEY_PASSPHRASE = "room_passphrase"
    }
}

/**
 * Returns [byteCount] random bytes hex-encoded as a lowercase string with no
 * separators and no `0x` prefix — safe to splice into SQL as a `KEY '<hex>'`
 * literal (see [RoomPassphraseStore]).
 *
 * Pure and internal so it is unit-testable without a Context/Keystore (issue
 * #812): [SecureRandom] and hex-encoding run fine on a plain JVM, unlike the
 * EncryptedSharedPreferences-backed store that normally calls this.
 */
internal fun randomHexPassphrase(
    random: SecureRandom = SecureRandom(),
    byteCount: Int = 32,
): String {
    val bytes = ByteArray(byteCount)
    random.nextBytes(bytes)
    return bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
}
