package com.mycorrhizal.crm.domain.repository

import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.model.network.WebAuthnRegisterResponse

/**
 * Issue #1293 / ADR 0034: passkey enrollment and management for the signed-in
 * user (web parity: `frontend/src/api/webauthn.ts`). The login ceremony lives
 * on [AuthRepository] because it mints the session. Option blobs and
 * credential JSON are raw strings, passed between the server and Credential
 * Manager verbatim — this layer never models their shape.
 */
interface PasskeyRepository {
    /** GET /webauthn/credentials — id, name and timestamps only (no key material). */
    suspend fun listPasskeys(): Result<List<WebAuthnCredential>>

    /**
     * POST /webauthn/register/begin — the raw creation options. A blank [name]
     * lets the server pick a default label. 403 = OIDC-provisioned account
     * (cannot enroll), 409 = the server has no valid RP configuration.
     */
    suspend fun beginRegistration(name: String?): Result<String>

    /**
     * POST /webauthn/register/finish with the attestation JSON. The response
     * carries the recovery codes (shown exactly once) when this passkey was the
     * account's first second factor; in that case the server re-issued the
     * session and the new token is stored here.
     */
    suspend fun finishRegistration(attestationJson: String): Result<WebAuthnRegisterResponse>

    /**
     * POST /webauthn/assert/begin — options for a proof-of-possession made with
     * a passkey OTHER than [excludeId] (the one being removed, #1317). 409 = no
     * other passkey remains, 404 = unknown / foreign id.
     */
    suspend fun beginProof(excludeId: String): Result<String>

    /** DELETE /webauthn/credentials/{id} proven by a live TOTP / recovery [code]. 400 = bad proof, 404 = unknown id. */
    suspend fun removeWithCode(id: String, code: String): Result<Unit>

    /** DELETE /webauthn/credentials/{id} proven by an assertion from ANOTHER passkey ([beginProof]'s options). */
    suspend fun removeWithAssertion(id: String, assertionJson: String): Result<Unit>
}
