package com.mycorrhizal.crm.data.local

import java.io.File
import java.security.SecureRandom
import java.util.Base64

/**
 * ADR 0028 Decision 2 / issue #1262: the two secrets the embedded server needs,
 * generated on-device on first run. They never touch a plaintext file: each is
 * wrapped by its own dedicated Android Keystore AES-GCM key (one key per
 * purpose, per MASVS CRYPTO) and the wrapped blobs are stored together in
 * `filesDir/local-server/keys.bin`. The host unwraps them in memory and hands
 * them to the server process over its private stdin.
 */
data class LocalServerSecrets(
    /** The JWT signing secret, base64url. */
    val jwtSecret: String,
    /** The at-rest master key, base64 (the backend's expected encoding). */
    val dataEncryptionKey: String,
)

/** The two secrets, each with its own Keystore key alias. */
enum class LocalServerSecretPurpose(val keystoreAlias: String) {
    JWT_SECRET("mycorrhizal.local_server.jwt"),
    ATREST_MASTER_KEY("mycorrhizal.local_server.atrest"),
}

/** Wraps a secret for storage; the production implementation is Keystore-backed. */
interface LocalServerSecretCipher {
    fun wrap(plaintext: ByteArray): ByteArray

    fun unwrap(blob: ByteArray): ByteArray
}

/** Supplies the cipher for a purpose, so each secret has its own key. */
fun interface LocalServerSecretCipherProvider {
    fun forPurpose(purpose: LocalServerSecretPurpose): LocalServerSecretCipher
}

/**
 * Generates and stores the embedded server's secrets. The generation and file
 * codec are pure (testable with a fake [LocalServerSecretCipherProvider]); only
 * the default cipher touches Keystore.
 *
 * The file format is a versioned three-line text record:
 * `mycorrhizal-local-server-keys/v1`, base64(wrap(jwtSecret)), base64(wrap(atrest)).
 */
class LocalServerSecretStore(
    private val keyFile: File,
    private val ciphers: LocalServerSecretCipherProvider,
    private val random: SecureRandom = SecureRandom(),
) {

    /**
     * Returns the stored secrets, generating and persisting fresh ones on first
     * run. A present-but-malformed file is an error, not a regeneration: silently
     * minting a new at-rest key would make the existing database's encrypted
     * fields unreadable.
     */
    fun loadOrCreate(): LocalServerSecrets {
        if (keyFile.exists()) {
            return decode(keyFile.readText())
        }
        val jwt = ByteArray(SECRET_BYTES).also(random::nextBytes)
        val atrest = ByteArray(SECRET_BYTES).also(random::nextBytes)
        val record = buildString {
            append(FORMAT_HEADER).append('\n')
            append(encode(ciphers.forPurpose(LocalServerSecretPurpose.JWT_SECRET).wrap(jwt))).append('\n')
            append(encode(ciphers.forPurpose(LocalServerSecretPurpose.ATREST_MASTER_KEY).wrap(atrest))).append('\n')
        }
        keyFile.parentFile?.mkdirs()
        keyFile.writeText(record)
        return LocalServerSecrets(base64Url(jwt), base64(atrest))
    }

    /** Removes the wrapped keys (the "Delete local data" path). */
    fun delete() {
        keyFile.delete()
    }

    private fun decode(text: String): LocalServerSecrets {
        val lines = text.trim().split('\n')
        require(lines.size >= 3 && lines[0] == FORMAT_HEADER) {
            "local-server keys.bin is malformed; refusing to regenerate and orphan the database"
        }
        val jwt = ciphers.forPurpose(LocalServerSecretPurpose.JWT_SECRET).unwrap(dec(lines[1]))
        val atrest = ciphers.forPurpose(LocalServerSecretPurpose.ATREST_MASTER_KEY).unwrap(dec(lines[2]))
        return LocalServerSecrets(base64Url(jwt), base64(atrest))
    }

    private fun encode(bytes: ByteArray): String = Base64.getEncoder().encodeToString(bytes)
    private fun dec(text: String): ByteArray = Base64.getDecoder().decode(text)
    private fun base64(bytes: ByteArray): String = Base64.getEncoder().encodeToString(bytes)
    private fun base64Url(bytes: ByteArray): String = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)

    companion object {
        const val FORMAT_HEADER = "mycorrhizal-local-server-keys/v1"
        const val SECRET_BYTES = 32
    }
}
