package com.mycorrhizal.crm.data.passkey

import android.content.Context
import android.os.Build
import androidx.credentials.CreatePublicKeyCredentialRequest
import androidx.credentials.CreatePublicKeyCredentialResponse
import androidx.credentials.CredentialManager
import androidx.credentials.GetCredentialRequest
import androidx.credentials.GetPublicKeyCredentialOption
import androidx.credentials.PublicKeyCredential
import androidx.credentials.exceptions.CreateCredentialCancellationException
import androidx.credentials.exceptions.CreateCredentialException
import androidx.credentials.exceptions.CreateCredentialNoCreateOptionException
import androidx.credentials.exceptions.CreateCredentialProviderConfigurationException
import androidx.credentials.exceptions.CreateCredentialUnsupportedException
import androidx.credentials.exceptions.GetCredentialCancellationException
import androidx.credentials.exceptions.GetCredentialException
import androidx.credentials.exceptions.GetCredentialProviderConfigurationException
import androidx.credentials.exceptions.GetCredentialUnsupportedException
import androidx.credentials.exceptions.NoCredentialException
import androidx.credentials.exceptions.domerrors.AbortError
import androidx.credentials.exceptions.domerrors.DomError
import androidx.credentials.exceptions.domerrors.InvalidStateError
import androidx.credentials.exceptions.domerrors.NotAllowedError
import androidx.credentials.exceptions.domerrors.SecurityError
import androidx.credentials.exceptions.publickeycredential.CreatePublicKeyCredentialDomException
import androidx.credentials.exceptions.publickeycredential.GetPublicKeyCredentialDomException
import kotlinx.coroutines.CancellationException
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Production [PasskeyCredentialClient]: the platform Credential Manager
 * (`androidx.credentials`), ADR 0034 Decision 1.
 *
 * Artifact choice: only the core `androidx.credentials:credentials` artifact is
 * a dependency of this module (GMS-free, so the F-Droid `foss` flavor keeps its
 * "no Firebase/GMS" guarantee). Below Android 14 the platform has no
 * Credential Manager; the `obtainium`/`play` flavors add
 * `credentials-play-services-auth` at runtime to back-fill it, and `foss` does
 * not — see [passkeysSupportedOn].
 *
 * This class is a thin adapter over the Jetpack API; the untestable part is the
 * two `CredentialManager` calls. The decisions (support check, envelope unwrap,
 * failure classification) are pure functions covered by JVM tests.
 */
@Singleton
class CredentialManagerPasskeyClient @Inject constructor() : PasskeyCredentialClient {

    override fun isSupported(): Boolean =
        passkeysSupportedOn(Build.VERSION.SDK_INT, playProviderBundled)

    @Suppress("TooGenericExceptionCaught")
    override suspend fun createPasskey(context: Context, optionsJson: String): PasskeyResult = try {
        val response = CredentialManager.create(context).createCredential(
            context,
            CreatePublicKeyCredentialRequest(unwrapPublicKeyOptions(optionsJson)),
        )
        (response as? CreatePublicKeyCredentialResponse)
            ?.registrationResponseJson
            ?.let { PasskeyResult.Success(it) }
            ?: PasskeyResult.Failed("Unexpected credential response type")
    } catch (e: CancellationException) {
        throw e
    } catch (e: CreateCredentialException) {
        classifyCreateFailure(e)
    } catch (e: Exception) {
        PasskeyResult.Failed(e.javaClass.simpleName)
    }

    @Suppress("TooGenericExceptionCaught")
    override suspend fun getPasskey(context: Context, optionsJson: String): PasskeyResult = try {
        val result = CredentialManager.create(context).getCredential(
            context,
            GetCredentialRequest(listOf(GetPublicKeyCredentialOption(unwrapPublicKeyOptions(optionsJson)))),
        )
        (result.credential as? PublicKeyCredential)
            ?.authenticationResponseJson
            ?.let { PasskeyResult.Success(it) }
            ?: PasskeyResult.Failed("Unexpected credential response type")
    } catch (e: CancellationException) {
        throw e
    } catch (e: GetCredentialException) {
        classifyGetFailure(e)
    } catch (e: Exception) {
        PasskeyResult.Failed(e.javaClass.simpleName)
    }

    private companion object {
        /** The reflective entry point androidx.credentials uses to find the Play services provider. */
        const val PLAY_PROVIDER_CLASS = "androidx.credentials.playservices.CredentialProviderPlayServicesImpl"

        val playProviderBundled: Boolean by lazy {
            runCatching { Class.forName(PLAY_PROVIDER_CLASS) }.isSuccess
        }
    }
}

/** First Android release whose platform ships Credential Manager (API 34). */
internal const val CREDENTIAL_MANAGER_PLATFORM_SDK = 34

/**
 * Pure device-support rule: the platform provides Credential Manager from API
 * 34; below that only a bundled Play services provider can (obtainium/play
 * builds). The F-Droid `foss` build bundles none, so it is supported on
 * Android 14+ only and otherwise sits behind the closed gate.
 */
internal fun passkeysSupportedOn(sdkInt: Int, playProviderBundled: Boolean): Boolean =
    sdkInt >= CREDENTIAL_MANAGER_PLATFORM_SDK || playProviderBundled

/**
 * The DOM-exception class the troubleshooting guide attributes to a missing or
 * wrong `/.well-known/assetlinks.json`: a `SecurityError`, or any error whose
 * message says the request could not be validated / the app is not associated.
 */
private fun isAssociationFailure(domError: DomError, message: CharSequence?): Boolean {
    if (domError is SecurityError) return true
    val text = message?.toString()?.lowercase().orEmpty()
    return ASSOCIATION_MARKERS.any { it in text }
}

private val ASSOCIATION_MARKERS = listOf(
    "cannot be validated",
    "not associated",
    "asset link",
    "assetlinks",
    "digital asset",
)

internal fun classifyCreateFailure(e: CreateCredentialException): PasskeyResult = when (e) {
    is CreateCredentialCancellationException -> PasskeyResult.Cancelled
    is CreateCredentialProviderConfigurationException,
    is CreateCredentialUnsupportedException,
    is CreateCredentialNoCreateOptionException,
    -> PasskeyResult.NoProvider
    is CreatePublicKeyCredentialDomException -> when {
        isAssociationFailure(e.domError, e.errorMessage) -> PasskeyResult.NotAssociated
        e.domError is InvalidStateError -> PasskeyResult.AlreadyRegistered
        // NotAllowedError / AbortError is the spec's catch-all for "user declined or it timed out".
        e.domError is NotAllowedError || e.domError is AbortError -> PasskeyResult.Cancelled
        else -> PasskeyResult.Failed(e.javaClass.simpleName)
    }
    else -> PasskeyResult.Failed(e.javaClass.simpleName)
}

internal fun classifyGetFailure(e: GetCredentialException): PasskeyResult = when (e) {
    is GetCredentialCancellationException -> PasskeyResult.Cancelled
    is NoCredentialException -> PasskeyResult.NoMatchingPasskey
    is GetCredentialProviderConfigurationException,
    is GetCredentialUnsupportedException,
    -> PasskeyResult.NoProvider
    is GetPublicKeyCredentialDomException -> when {
        isAssociationFailure(e.domError, e.errorMessage) -> PasskeyResult.NotAssociated
        e.domError is NotAllowedError || e.domError is AbortError -> PasskeyResult.Cancelled
        else -> PasskeyResult.Failed(e.javaClass.simpleName)
    }
    else -> PasskeyResult.Failed(e.javaClass.simpleName)
}
