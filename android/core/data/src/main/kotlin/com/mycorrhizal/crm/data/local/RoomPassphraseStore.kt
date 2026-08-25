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

    /** Returns the persisted passphrase, generating + persisting one on first use. */
    fun getOrCreate(): String {
        prefs.getString(KEY_PASSPHRASE, null)?.let { return it }
        val passphrase = generatePassphrase()
        prefs.edit().putString(KEY_PASSPHRASE, passphrase).apply()
        return passphrase
    }

    /** Drops the stored passphrase (only safe alongside deleting the DB file). */
    fun clear() {
        prefs.edit().remove(KEY_PASSPHRASE).apply()
    }

    private fun generatePassphrase(): String {
        val bytes = ByteArray(32)
        SecureRandom().nextBytes(bytes)
        return bytes.joinToString("") { (it.toInt() and 0xff).toString(16).padStart(2, '0') }
    }

    companion object {
        private const val FILE_NAME = "secure_room"
        private const val KEY_PASSPHRASE = "room_passphrase"
    }
}
