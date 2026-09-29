package com.mycorrhizal.crm.data.passkey

import android.content.Context
import com.squareup.moshi.JsonReader
import okio.Buffer

/**
 * Issue #1293 / ADR 0034: the seam between the passkey logic (ViewModels,
 * repositories) and the platform Credential Manager. Everything above this
 * interface is plain JVM and tested with a fake; a real ceremony needs a
 * device, a credential provider and a publicly reachable HTTPS server, so it is
 * never attempted in a unit test.
 *
 * The private key never crosses this seam: it lives in the platform
 * authenticator / credential provider. Only the WebAuthn *public* response JSON
 * (attestation or assertion) comes back.
 */
interface PasskeyCredentialClient {
    /**
     * True when this device can plausibly run a ceremony: the platform ships
     * Credential Manager (Android 14+) or, below that, the Play services
     * provider is bundled in this build. It says nothing about whether a
     * provider is actually configured — that surfaces at ceremony time as
     * [PasskeyResult.NoProvider].
     */
    fun isSupported(): Boolean

    /**
     * Registration ceremony. [optionsJson] is the server's
     * `/webauthn/register/begin` body verbatim (the go-webauthn
     * `{"publicKey":{...}}` envelope); [context] should be an Activity so the
     * provider UI can launch. Success carries the RegistrationResponseJSON to
     * POST to `/webauthn/register/finish`.
     */
    suspend fun createPasskey(context: Context, optionsJson: String): PasskeyResult

    /**
     * Assertion ceremony for `/webauthn/login/begin` or `/webauthn/assert/begin`
     * options. Success carries the AuthenticationResponseJSON.
     */
    suspend fun getPasskey(context: Context, optionsJson: String): PasskeyResult
}

/** Typed outcome of one ceremony; every non-[Success] case is a distinct UI state, not an exception. */
sealed interface PasskeyResult {
    /** [json] is the WebAuthn response JSON, sent to the server verbatim. */
    data class Success(val json: String) : PasskeyResult

    /** The user dismissed the provider UI (or it timed out). Silent — never an error banner. */
    data object Cancelled : PasskeyResult

    /** No credential provider / Credential Manager unusable here: the degraded state. */
    data object NoProvider : PasskeyResult

    /**
     * The platform refused because this app is not associated with the RP ID
     * (Digital Asset Links missing or wrong): "this server isn't set up for
     * Android passkeys" (ADR 0034 Decision 4).
     */
    data object NotAssociated : PasskeyResult

    /** Get only: the provider holds no passkey on this device for the server's allow-list. */
    data object NoMatchingPasskey : PasskeyResult

    /** Create only: the provider already holds a passkey the server listed in `excludeCredentials`. */
    data object AlreadyRegistered : PasskeyResult

    /** Anything else. [message] is diagnostic only (not localized). */
    data class Failed(val message: String? = null) : PasskeyResult
}

private const val PUBLIC_KEY_MEMBER = "publicKey"

/**
 * go-webauthn returns `{"publicKey":{...}}` (the browser `navigator.credentials`
 * envelope); Credential Manager wants the bare `PublicKeyCredential*Options`
 * JSON. Extracts the `publicKey` member verbatim (token-for-token, so numbers
 * such as `timeout` are not re-encoded); a body that has no such member — or is
 * not an object at all — is returned unchanged.
 */
@Suppress("TooGenericExceptionCaught", "SwallowedException")
fun unwrapPublicKeyOptions(envelope: String): String {
    return try {
        val reader = JsonReader.of(Buffer().writeUtf8(envelope))
        reader.beginObject()
        var inner: String? = null
        while (reader.hasNext()) {
            val name = reader.nextName()
            if (inner == null && name == PUBLIC_KEY_MEMBER) {
                inner = reader.nextSource().readUtf8()
            } else {
                reader.skipValue()
            }
        }
        inner ?: envelope
    } catch (e: Exception) {
        envelope
    }
}
