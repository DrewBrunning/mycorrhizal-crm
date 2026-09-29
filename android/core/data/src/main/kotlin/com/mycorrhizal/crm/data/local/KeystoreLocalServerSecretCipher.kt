package com.mycorrhizal.crm.data.local

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * The production [LocalServerSecretCipher]: AES-256-GCM under a non-exportable
 * Android Keystore key, one key per purpose (MASVS CRYPTO, P8). The IV is
 * prepended to the ciphertext. Every call goes through the real Keystore, which
 * a JVM test cannot exercise, so this file is outside the coverage model and is
 * pinned by [LocalServerSecurityGuardTest] instead; the codec it plugs into is
 * fully covered by [LocalServerSecretStoreTest].
 */
class KeystoreLocalServerSecretCipher(private val alias: String) : LocalServerSecretCipher {

    override fun wrap(plaintext: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val iv = cipher.iv
        return iv + cipher.doFinal(plaintext)
    }

    override fun unwrap(blob: ByteArray): ByteArray {
        require(blob.size > GCM_IV_BYTES) { "wrapped secret too short" }
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(GCM_TAG_BITS, blob, 0, GCM_IV_BYTES))
        return cipher.doFinal(blob, GCM_IV_BYTES, blob.size - GCM_IV_BYTES)
    }

    private fun key(): SecretKey {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        (keyStore.getEntry(alias, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private companion object {
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val GCM_IV_BYTES = 12
        const val GCM_TAG_BITS = 128
    }
}

/** Production provider: one Keystore cipher per purpose, created lazily and reused. */
class KeystoreLocalServerSecretCipherProvider : LocalServerSecretCipherProvider {
    private val ciphers = mutableMapOf<LocalServerSecretPurpose, LocalServerSecretCipher>()

    override fun forPurpose(purpose: LocalServerSecretPurpose): LocalServerSecretCipher =
        synchronized(ciphers) {
            ciphers.getOrPut(purpose) { KeystoreLocalServerSecretCipher(purpose.keystoreAlias) }
        }
}
