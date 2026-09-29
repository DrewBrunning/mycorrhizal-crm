package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.SessionManager
import com.mycorrhizal.crm.domain.repository.PasskeyRepository
import com.mycorrhizal.crm.domain.repository.SecondFactorProof
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.model.network.WebAuthnRegisterResponse
import com.mycorrhizal.crm.model.network.MessageResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ReissuedTokenResult
import com.mycorrhizal.crm.network.toApiError
import javax.inject.Inject

/** Issue #1293: thin, error-normalizing adapter over the [ApiClient] WebAuthn calls. */
class PasskeyRepositoryImpl @Inject constructor(
    private val apiClient: ApiClient,
    private val sessionManager: SessionManager,
) : PasskeyRepository {

    override suspend fun listPasskeys(): Result<List<WebAuthnCredential>> =
        apiClient.listWebAuthnCredentials().fold(
            onSuccess = { Result.success(it.credentials) },
            onFailure = { Result.failure(it.toApiError()) },
        )

    override suspend fun beginRegistration(name: String?, proof: SecondFactorProof?): Result<String> =
        apiClient.webauthnRegisterBegin(
            name = name?.trim(),
            code = (proof as? SecondFactorProof.Code)?.code?.trim(),
            assertionJson = (proof as? SecondFactorProof.Assertion)?.json,
        ).fold(
            onSuccess = { Result.success(it) },
            onFailure = { Result.failure(it.toApiError()) },
        )

    override suspend fun finishRegistration(attestationJson: String): Result<WebAuthnRegisterResponse> {
        val body = apiClient.webauthnRegisterFinish(attestationJson)
            .getOrElse { return Result.failure(it.toApiError()) }
        // The first second factor bumps token_version server-side: swap in the
        // re-issued session token so this session isn't invalidated by the very
        // mutation that just succeeded (same as confirmTwoFactor).
        body.reissuedToken?.let { sessionManager.setToken(it) }
        return Result.success(body.value)
    }

    override suspend fun beginProof(excludeId: String?): Result<String> =
        apiClient.webauthnProofBegin(excludeId).fold(
            onSuccess = { Result.success(it) },
            onFailure = { Result.failure(it.toApiError()) },
        )

    override suspend fun removeWithCode(id: String, code: String): Result<Unit> =
        adoptReissuedToken(apiClient.deleteWebAuthnCredential(id, code.trim()))

    override suspend fun removeWithAssertion(id: String, assertionJson: String): Result<Unit> =
        adoptReissuedToken(apiClient.deleteWebAuthnCredentialWithAssertion(id, assertionJson))

    /**
     * Removing the account's last second factor bumps token_version and revokes
     * every session (#1338), re-issuing the caller's as a fresh cookie: swap it
     * in so the next request doesn't 401 and sign the user out (as disableTwoFactor does).
     */
    private suspend fun adoptReissuedToken(result: Result<ReissuedTokenResult<MessageResponse>>): Result<Unit> {
        val body = result.getOrElse { return Result.failure(it.toApiError()) }
        body.reissuedToken?.let { sessionManager.setToken(it) }
        return Result.success(Unit)
    }
}
