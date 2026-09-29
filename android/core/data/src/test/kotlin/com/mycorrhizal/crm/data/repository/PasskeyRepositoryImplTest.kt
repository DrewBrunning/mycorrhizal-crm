package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.DefaultSessionManager
import com.mycorrhizal.crm.data.session.FakeSessionPrefsStorage
import com.mycorrhizal.crm.data.session.FakeTokenStorage
import com.mycorrhizal.crm.model.network.MessageResponse
import com.mycorrhizal.crm.model.network.WebAuthnCredential
import com.mycorrhizal.crm.model.network.WebAuthnCredentialListResponse
import com.mycorrhizal.crm.model.network.WebAuthnRegisterResponse
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import com.mycorrhizal.crm.network.ReissuedTokenResult
import io.mockk.coEvery
import io.mockk.coVerify
import io.mockk.mockk
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** Issue #1293: enrollment/management calls, error normalization and the re-issued session token. */
class PasskeyRepositoryImplTest {

    private class Harness {
        val apiClient = mockk<ApiClient>()
        val tokenStorage = FakeTokenStorage()
        val sessionManager = DefaultSessionManager(tokenStorage, FakeSessionPrefsStorage())
        val repository = PasskeyRepositoryImpl(apiClient, sessionManager)
    }

    @Test
    fun `list unwraps the credentials`() = runTest {
        val h = Harness()
        val list = listOf(WebAuthnCredential("a", "Phone", "2026-09-01T10:00:00Z", null))
        coEvery { h.apiClient.listWebAuthnCredentials() } returns Result.success(WebAuthnCredentialListResponse(list))

        assertEquals(list, h.repository.listPasskeys().getOrThrow())
    }

    @Test
    fun `failures are normalized to ApiError`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.listWebAuthnCredentials() } returns Result.failure(ApiError.Server(500, "boom"))
        coEvery { h.apiClient.webauthnRegisterBegin(any()) } returns Result.failure(ApiError.Client(403, "OIDC"))
        coEvery { h.apiClient.webauthnProofBegin(any()) } returns Result.failure(ApiError.Client(409, "No other passkey"))

        assertTrue(h.repository.listPasskeys().exceptionOrNull() is ApiError.Server)
        assertEquals(403, (h.repository.beginRegistration("x").exceptionOrNull() as ApiError.Client).code)
        assertEquals(409, (h.repository.beginProof("a").exceptionOrNull() as ApiError.Client).code)
    }

    @Test
    fun `begin registration trims the label and passes null through`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.webauthnRegisterBegin(any()) } returns Result.success("{}")

        h.repository.beginRegistration("  Laptop ")
        h.repository.beginRegistration(null)

        coVerify { h.apiClient.webauthnRegisterBegin("Laptop") }
        coVerify { h.apiClient.webauthnRegisterBegin(null) }
    }

    @Test
    fun `finishing the first passkey stores the re-issued session token`() = runTest {
        val h = Harness()
        h.sessionManager.setServerUrl("https://crm.example.com")
        val body = WebAuthnRegisterResponse("id", "Phone", "2026-09-01T10:00:00Z", listOf("AAAAA-BBBBB-CCCCC"))
        coEvery { h.apiClient.webauthnRegisterFinish("{att}") } returns Result.success(ReissuedTokenResult(body, "jwt-new"))

        val result = h.repository.finishRegistration("{att}")

        assertEquals(body, result.getOrThrow())
        assertEquals("jwt-new", h.sessionManager.bearerToken())
    }

    @Test
    fun `finishing a later passkey leaves the session token alone`() = runTest {
        val h = Harness()
        h.sessionManager.setServerUrl("https://crm.example.com")
        h.sessionManager.setToken("jwt-old")
        val body = WebAuthnRegisterResponse("id", "Phone", "2026-09-01T10:00:00Z", emptyList())
        coEvery { h.apiClient.webauthnRegisterFinish(any()) } returns Result.success(ReissuedTokenResult(body, null))

        assertTrue(h.repository.finishRegistration("{att}").getOrThrow().recoveryCodes.isEmpty())
        assertEquals("jwt-old", h.sessionManager.bearerToken())
    }

    @Test
    fun `a rejected attestation does not touch the session`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.webauthnRegisterFinish(any()) } returns Result.failure(ApiError.Client(400, "bad"))

        assertEquals(400, (h.repository.finishRegistration("{att}").exceptionOrNull() as ApiError.Client).code)
        assertNull(h.sessionManager.bearerToken())
    }

    @Test
    fun `removal by code trims it and by assertion passes the id and blob`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.deleteWebAuthnCredential(any(), any()) } returns Result.success(MessageResponse("ok"))
        coEvery { h.apiClient.deleteWebAuthnCredentialWithAssertion(any(), any()) } returns Result.success(MessageResponse("ok"))

        assertTrue(h.repository.removeWithCode("a", " 123456 ").isSuccess)
        assertTrue(h.repository.removeWithAssertion("b", """{"id":"other"}""").isSuccess)

        coVerify { h.apiClient.deleteWebAuthnCredential("a", "123456") }
        coVerify { h.apiClient.deleteWebAuthnCredentialWithAssertion("b", """{"id":"other"}""") }
    }

    @Test
    fun `removal failures keep their status`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.deleteWebAuthnCredential(any(), any()) } returns Result.failure(ApiError.Client(404, "Not found"))
        coEvery { h.apiClient.deleteWebAuthnCredentialWithAssertion(any(), any()) } returns Result.failure(ApiError.Client(400, "bad proof"))

        assertEquals(404, (h.repository.removeWithCode("a", "1").exceptionOrNull() as ApiError.Client).code)
        assertEquals(400, (h.repository.removeWithAssertion("a", "{}").exceptionOrNull() as ApiError.Client).code)
    }

    @Test
    fun `proof begin forwards the id to exclude`() = runTest {
        val h = Harness()
        coEvery { h.apiClient.webauthnProofBegin("target") } returns Result.success("""{"publicKey":{}}""")

        assertEquals("""{"publicKey":{}}""", h.repository.beginProof("target").getOrThrow())
        coVerify { h.apiClient.webauthnProofBegin("target") }
    }
}
